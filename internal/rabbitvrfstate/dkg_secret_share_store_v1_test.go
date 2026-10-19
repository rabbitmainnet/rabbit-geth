package rabbitvrfstate

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestDKGSecretShareStoreV1RestartDuplicateConflict(t *testing.T) {
	context, err := lqc.NewRabbitVRFDKGSessionContextV1(big.NewInt(9280), 11, crypto.Keccak256Hash([]byte("secret-share-store-committee-v1")), 32)
	if err != nil {
		t.Fatal(err)
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID:     7,
		TicketHash:  crypto.Keccak256Hash([]byte("secret-share-store-ticket-v1")),
		Participant: crypto.PubkeyToAddress(key.PublicKey),
	}

	makeShare := func(value byte) *rabbitvrf.SecretShare {
		encoded := make([]byte, rabbitvrf.SecretKeySize)
		encoded[len(encoded)-1] = value
		share, err := rabbitvrf.SecretShareFromBytes(member.ShareID, encoded)
		if err != nil {
			t.Fatal(err)
		}
		return share
	}

	first := makeShare(5)
	second := makeShare(6)

	dir := t.TempDir()
	store, err := NewDKGSecretShareStoreV1(dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}

	const password = "rabbit-vrf-secret-share-test"

	if err := store.Store(context, member, first, password); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewDKGSecretShareStoreV1(dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := restarted.Load(context, member, password)
	if err != nil {
		t.Fatal(err)
	}

	firstBytes, err := first.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	loadedBytes, err := loaded.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if firstBytes != loadedBytes {
		t.Fatal("restart changed secret share")
	}

	if err := restarted.Store(context, member, first, password); err != nil {
		t.Fatalf("duplicate rejected: %v", err)
	}

	if err := restarted.Store(context, member, second, password); !errors.Is(err, ErrDKGSecretShareStoreConflictV1) {
		t.Fatalf("conflict error=%v", err)
	}
}
