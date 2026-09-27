package rabbitvrfstate

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestDKGVerifiedEvaluationStoreV1RestartDuplicateConflict(t *testing.T) {
	context, err := lqc.NewRabbitVRFDKGSessionContextV1(big.NewInt(9280), 11, crypto.Keccak256Hash([]byte("verified-evaluation-store-committee-v1")), 32)
	if err != nil {
		t.Fatal(err)
	}
	recipientKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	dealerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	recipient := lqc.RabbitVRFCommitteeMemberV1{ShareID: 7, TicketHash: crypto.Keccak256Hash([]byte("verified-evaluation-recipient-ticket-v1")), Participant: crypto.PubkeyToAddress(recipientKey.PublicKey)}
	dealer := lqc.RabbitVRFCommitteeMemberV1{ShareID: 9, TicketHash: crypto.Keccak256Hash([]byte("verified-evaluation-dealer-ticket-v1")), Participant: crypto.PubkeyToAddress(dealerKey.PublicKey)}
	var firstBytes [rabbitvrf.DKGPolynomialEvaluationSizeV1]byte
	firstBytes[len(firstBytes)-1] = 1
	first, err := rabbitvrf.DKGPolynomialEvaluationV1FromBytes(firstBytes[:])
	if err != nil {
		t.Fatal(err)
	}
	var secondBytes [rabbitvrf.DKGPolynomialEvaluationSizeV1]byte
	secondBytes[len(secondBytes)-1] = 2
	second, err := rabbitvrf.DKGPolynomialEvaluationV1FromBytes(secondBytes[:])
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store, err := NewDKGVerifiedEvaluationStoreV1(dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	const password = "rabbit-vrf-evaluation-store-test"
	if err := store.Store(context, recipient, dealer, first, password); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewDKGVerifiedEvaluationStoreV1(dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(context, recipient, dealer, password)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != first {
		t.Fatal("restart load changed verified evaluation")
	}
	if err := restarted.Store(context, recipient, dealer, first, password); err != nil {
		t.Fatalf("duplicate rejected: %v", err)
	}
	if err := restarted.Store(context, recipient, dealer, second, password); !errors.Is(err, ErrDKGVerifiedEvaluationStoreConflictV1) {
		t.Fatalf("conflict error=%v", err)
	}
}
