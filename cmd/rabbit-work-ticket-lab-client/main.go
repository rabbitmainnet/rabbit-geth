package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

type workTicketArgs struct {
	Version     hexutil.Uint64 `json:"version"`
	Epoch       hexutil.Uint64 `json:"epoch"`
	Anchor      common.Hash    `json:"anchor"`
	Participant common.Address `json:"participant"`
	Sequence    hexutil.Uint64 `json:"sequence"`
	Previous    common.Hash    `json:"previous"`
	Proof       common.Hash    `json:"proof"`
	Signature   hexutil.Bytes  `json:"signature"`
}

type submissionResult struct {
	Status      string         `json:"status"`
	Hash        common.Hash    `json:"hash"`
	Participant common.Address `json:"participant"`
	RPC         string         `json:"rpc"`
}

func buildLaboratoryTicket(key *ecdsa.PrivateKey) (lqc.WorkTicket, error) {
	chainID := big.NewInt(928)
	participant := crypto.PubkeyToAddress(key.PublicKey)
	anchor := crypto.Keccak256Hash([]byte("RABBIT-LQC-WORK-TICKET-LIVE-LAB-V1"))
	ticket := lqc.WorkTicket{
		Version:     lqc.WorkTicketProtocolVersion,
		Epoch:       1,
		Anchor:      anchor,
		Participant: participant,
		Sequence:    1,
		Previous:    lqc.InitialWorkTicketPrevious(chainID, anchor, 1, participant),
	}
	proof, err := lqc.GenerateWorkTicketProof(chainID, ticket)
	if err != nil {
		return lqc.WorkTicket{}, err
	}
	ticket.Proof = proof
	hash := lqc.WorkTicketSigningHash(chainID, ticket)
	ticket.Signature, err = crypto.Sign(hash[:], key)
	if err != nil {
		return lqc.WorkTicket{}, err
	}
	return ticket, nil
}

func ticketArgs(ticket lqc.WorkTicket) workTicketArgs {
	return workTicketArgs{
		Version:     hexutil.Uint64(ticket.Version),
		Epoch:       hexutil.Uint64(ticket.Epoch),
		Anchor:      ticket.Anchor,
		Participant: ticket.Participant,
		Sequence:    hexutil.Uint64(ticket.Sequence),
		Previous:    ticket.Previous,
		Proof:       ticket.Proof,
		Signature:   append(hexutil.Bytes(nil), ticket.Signature...),
	}
}

func run(endpoint string) error {
	key, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	ticket, err := buildLaboratoryTicket(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := rpc.DialContext(ctx, endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	var accepted common.Hash
	if err := client.CallContext(ctx, &accepted, "lqc_submitWorkTicket", ticketArgs(ticket)); err != nil {
		return err
	}
	want := lqc.WorkTicketHash(big.NewInt(928), ticket)
	if accepted != want {
		return fmt.Errorf("RPC returned hash %s, want %s", accepted, want)
	}
	return json.NewEncoder(os.Stdout).Encode(submissionResult{
		Status:      "PASS",
		Hash:        accepted,
		Participant: ticket.Participant,
		RPC:         endpoint,
	})
}

func main() {
	endpoint := flag.String("rpc", "http://127.0.0.1:8771", "laboratory node HTTP RPC endpoint")
	flag.Parse()
	if err := run(*endpoint); err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
}
