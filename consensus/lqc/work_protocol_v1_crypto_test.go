package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestWorkProtocolV1EpochKeyIsGlobalAcrossParticipants(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x123456")
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	keyA, err := RandomXWorkEpochKeyV1(chainID, 7, anchor)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := RandomXWorkEpochKeyV1(chainID, 7, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if keyA != keyB {
		t.Fatal("epoch key changed without epoch/anchor change")
	}

	inputA, err := RandomXWorkInputV1(chainID, anchor, RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       7,
		Participant: a,
		Nonce:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputB, err := RandomXWorkInputV1(chainID, anchor, RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       7,
		Participant: b,
		Nonce:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(inputA) == string(inputB) {
		t.Fatal("participant did not bind RandomX input")
	}
}

func TestWorkProtocolV1OnlyParticipantCanAuthorizeSuccessfulWork(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0xabcdef")
	proofHash := common.HexToHash("0x01")

	participantKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	attackerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	participant := crypto.PubkeyToAddress(participantKey.PublicKey)

	ticket := RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       9,
		Participant: participant,
		Nonce:       42,
	}

	signHash, err := RandomXWorkSigningHashV1(
		chainID,
		anchor,
		ticket,
		proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}

	goodSig, err := crypto.Sign(signHash[:], participantKey)
	if err != nil {
		t.Fatal(err)
	}
	badSig, err := crypto.Sign(signHash[:], attackerKey)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		SignedRandomXWorkTicketV1{
			Ticket:    ticket,
			Signature: goodSig,
		},
		proofHash,
	); err != nil {
		t.Fatalf("participant signature rejected: %v", err)
	}

	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		SignedRandomXWorkTicketV1{
			Ticket:    ticket,
			Signature: badSig,
		},
		proofHash,
	); err != ErrInvalidRandomXWorkSignature {
		t.Fatalf("attacker signature error = %v, want %v", err, ErrInvalidRandomXWorkSignature)
	}
}

func TestWorkProtocolV1SignatureBindsProofAndNonce(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x998877")
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(key.PublicKey)

	ticket := RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       11,
		Participant: participant,
		Nonce:       7,
	}
	proofHash := common.HexToHash("0x1234")

	signHash, err := RandomXWorkSigningHashV1(
		chainID,
		anchor,
		ticket,
		proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(signHash[:], key)
	if err != nil {
		t.Fatal(err)
	}

	signed := SignedRandomXWorkTicketV1{
		Ticket:    ticket,
		Signature: sig,
	}

	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		signed,
		proofHash,
	); err != nil {
		t.Fatal(err)
	}

	tamperedNonce := signed
	tamperedNonce.Ticket.Nonce++
	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		tamperedNonce,
		proofHash,
	); err != ErrInvalidRandomXWorkSignature {
		t.Fatalf("tampered nonce error = %v", err)
	}

	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		signed,
		common.HexToHash("0x1235"),
	); err != ErrInvalidRandomXWorkSignature {
		t.Fatalf("tampered proof error = %v", err)
	}
}

func TestWorkProtocolV1TargetValidation(t *testing.T) {
	// difficulty 1 accepts every 256-bit hash.
	ok, err := RandomXWorkHashMeetsTargetV1(
		common.HexToHash("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		big.NewInt(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("difficulty 1 rejected max hash")
	}

	// At difficulty 2, max hash is above the target.
	ok, err = RandomXWorkHashMeetsTargetV1(
		common.HexToHash("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		big.NewInt(2),
	)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("difficulty 2 accepted max hash")
	}
}

func TestWorkProtocolV1ValidatedWorkBecomesOneSeat(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x5555")
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(key.PublicKey)

	ticket := RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       12,
		Participant: participant,
		Nonce:       99,
	}
	proofHash := common.HexToHash("0x01")

	signHash, err := RandomXWorkSigningHashV1(
		chainID,
		anchor,
		ticket,
		proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(signHash[:], key)
	if err != nil {
		t.Fatal(err)
	}

	verified, err := ValidateRecomputedRandomXWorkV1(
		chainID,
		anchor,
		big.NewInt(1),
		SignedRandomXWorkTicketV1{
			Ticket:    ticket,
			Signature: sig,
		},
		proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}

	seats, err := CanonicalWorkSeatsV1(
		[]VerifiedRandomXWorkTicketV1{verified},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 1 {
		t.Fatalf("seats = %d, want 1", len(seats))
	}
	if seats[0].Participant != participant || seats[0].TicketHash != proofHash {
		t.Fatalf("unexpected seat: %+v", seats[0])
	}
}
