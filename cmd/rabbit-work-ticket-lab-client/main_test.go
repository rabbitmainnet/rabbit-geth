package main

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestBuildLaboratoryTicket(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := buildLaboratoryTicket(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := lqc.ValidateWorkTicketCryptography(big.NewInt(928), ticket); err != nil {
		t.Fatalf("generated laboratory ticket is invalid: %v", err)
	}
	args := ticketArgs(ticket)
	if uint64(args.Version) != uint64(ticket.Version) || args.Proof != ticket.Proof || len(args.Signature) != 65 {
		t.Fatalf("RPC args lost ticket fields: %+v", args)
	}
}
