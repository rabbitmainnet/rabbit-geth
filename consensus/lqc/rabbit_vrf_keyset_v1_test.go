package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFKeysetTestPublicKeyV1(
	t *testing.T,
	scalar byte,
) rabbitvrf.PublicKey {
	t.Helper()

	encoded := make([]byte, rabbitvrf.SecretKeySize)
	encoded[len(encoded)-1] = scalar

	secret, err := rabbitvrf.SecretKeyFromBytes(
		encoded,
	)
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err := secret.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	return publicKey
}

func TestRabbitVRFKeysetRootV1CanonicalizesShareOrder(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	committeeRoot := common.HexToHash("0xaaaa")
	transcriptRoot := common.HexToHash("0xbbbb")
	thresholdPublicKey := rabbitVRFKeysetTestPublicKeyV1(
		t,
		42,
	)

	sharesA := []RabbitVRFVerificationShareV1{
		{
			ShareID:   5,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 5),
		},
		{
			ShareID:   1,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
		},
		{
			ShareID:   3,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 3),
		},
	}

	sharesB := []RabbitVRFVerificationShareV1{
		sharesA[1],
		sharesA[2],
		sharesA[0],
	}

	rootA, canonicalA, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		3,
		thresholdPublicKey,
		transcriptRoot,
		sharesA,
	)
	if err != nil {
		t.Fatal(err)
	}

	rootB, canonicalB, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		3,
		thresholdPublicKey,
		transcriptRoot,
		sharesB,
	)
	if err != nil {
		t.Fatal(err)
	}

	if rootA != rootB {
		t.Fatal("verification share arrival order changed keyset root")
	}

	wantIDs := []uint64{1, 3, 5}

	for index, want := range wantIDs {
		if canonicalA[index].ShareID != want ||
			canonicalB[index].ShareID != want {
			t.Fatalf(
				"canonical share index=%d got=%d,%d want=%d",
				index,
				canonicalA[index].ShareID,
				canonicalB[index].ShareID,
				want,
			)
		}
	}
}

func TestRabbitVRFKeysetRootV1BindsPublicContext(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	committeeRoot := common.HexToHash("0xaaaa")
	transcriptRoot := common.HexToHash("0xbbbb")
	thresholdPublicKey := rabbitVRFKeysetTestPublicKeyV1(
		t,
		42,
	)

	shares := []RabbitVRFVerificationShareV1{
		{
			ShareID:   1,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
		},
		{
			ShareID:   2,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 2),
		},
		{
			ShareID:   3,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 3),
		},
	}

	base, _, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		2,
		thresholdPublicKey,
		transcriptRoot,
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}

	otherEpoch, _, err := RabbitVRFKeysetRootV1(
		chainID,
		10,
		committeeRoot,
		5,
		2,
		thresholdPublicKey,
		transcriptRoot,
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherEpoch == base {
		t.Fatal("VRF epoch did not bind keyset root")
	}

	otherCommittee, _, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		common.HexToHash("0xaaab"),
		5,
		2,
		thresholdPublicKey,
		transcriptRoot,
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherCommittee == base {
		t.Fatal("committee root did not bind keyset root")
	}

	otherThreshold, _, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		3,
		thresholdPublicKey,
		transcriptRoot,
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherThreshold == base {
		t.Fatal("threshold did not bind keyset root")
	}

	otherPublicKey, _, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		2,
		rabbitVRFKeysetTestPublicKeyV1(t, 43),
		transcriptRoot,
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherPublicKey == base {
		t.Fatal("threshold public key did not bind keyset root")
	}

	otherTranscript, _, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		2,
		thresholdPublicKey,
		common.HexToHash("0xbbbc"),
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherTranscript == base {
		t.Fatal("transcript root did not bind keyset root")
	}

	otherShares := append(
		[]RabbitVRFVerificationShareV1(nil),
		shares...,
	)
	otherShares[2].PublicKey =
		rabbitVRFKeysetTestPublicKeyV1(t, 4)

	otherShareRoot, _, err := RabbitVRFKeysetRootV1(
		chainID,
		9,
		committeeRoot,
		5,
		2,
		thresholdPublicKey,
		transcriptRoot,
		otherShares,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherShareRoot == base {
		t.Fatal("verification shares did not bind keyset root")
	}
}

func TestRabbitVRFKeysetRootV1PreservesQualifiedShareIDs(
	t *testing.T,
) {
	shares := []RabbitVRFVerificationShareV1{
		{
			ShareID:   4,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 4),
		},
		{
			ShareID:   1,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
		},
		{
			ShareID:   3,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 3),
		},
	}

	_, canonical, err := RabbitVRFKeysetRootV1(
		big.NewInt(9280),
		9,
		common.HexToHash("0xaaaa"),
		5,
		2,
		rabbitVRFKeysetTestPublicKeyV1(t, 42),
		common.HexToHash("0xbbbb"),
		shares,
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []uint64{1, 3, 4}

	for index := range want {
		if canonical[index].ShareID != want[index] {
			t.Fatalf(
				"index=%d shareID=%d want=%d",
				index,
				canonical[index].ShareID,
				want[index],
			)
		}
	}
}

func TestRabbitVRFKeysetRootV1RejectsInvalidStructure(
	t *testing.T,
) {
	thresholdPublicKey := rabbitVRFKeysetTestPublicKeyV1(
		t,
		42,
	)
	transcriptRoot := common.HexToHash("0xbbbb")
	committeeRoot := common.HexToHash("0xaaaa")

	valid := []RabbitVRFVerificationShareV1{
		{
			ShareID:   1,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
		},
		{
			ShareID:   3,
			PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 3),
		},
	}

	tests := []struct {
		name          string
		committeeSize uint64
		threshold     uint64
		transcript    common.Hash
		shares        []RabbitVRFVerificationShareV1
	}{
		{
			name:          "zero committee size",
			committeeSize: 0,
			threshold:     1,
			transcript:    transcriptRoot,
			shares:        valid,
		},
		{
			name:          "zero threshold",
			committeeSize: 5,
			threshold:     0,
			transcript:    transcriptRoot,
			shares:        valid,
		},
		{
			name:          "threshold above qualified shares",
			committeeSize: 5,
			threshold:     3,
			transcript:    transcriptRoot,
			shares:        valid,
		},
		{
			name:          "zero transcript",
			committeeSize: 5,
			threshold:     1,
			transcript:    common.Hash{},
			shares:        valid,
		},
		{
			name:          "zero share id",
			committeeSize: 5,
			threshold:     1,
			transcript:    transcriptRoot,
			shares: []RabbitVRFVerificationShareV1{
				{
					ShareID:   0,
					PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
				},
			},
		},
		{
			name:          "share id above committee size",
			committeeSize: 5,
			threshold:     1,
			transcript:    transcriptRoot,
			shares: []RabbitVRFVerificationShareV1{
				{
					ShareID:   6,
					PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
				},
			},
		},
		{
			name:          "duplicate share id",
			committeeSize: 5,
			threshold:     1,
			transcript:    transcriptRoot,
			shares: []RabbitVRFVerificationShareV1{
				{
					ShareID:   1,
					PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 1),
				},
				{
					ShareID:   1,
					PublicKey: rabbitVRFKeysetTestPublicKeyV1(t, 2),
				},
			},
		},
		{
			name:          "zero verification key",
			committeeSize: 5,
			threshold:     1,
			transcript:    transcriptRoot,
			shares: []RabbitVRFVerificationShareV1{
				{
					ShareID: 1,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := RabbitVRFKeysetRootV1(
				big.NewInt(9280),
				9,
				committeeRoot,
				test.committeeSize,
				test.threshold,
				thresholdPublicKey,
				test.transcript,
				test.shares,
			); err == nil {
				t.Fatal("invalid keyset structure accepted")
			}
		})
	}

	var zeroThresholdPublicKey rabbitvrf.PublicKey

	if _, _, err := RabbitVRFKeysetRootV1(
		big.NewInt(9280),
		9,
		committeeRoot,
		5,
		1,
		zeroThresholdPublicKey,
		transcriptRoot,
		valid,
	); err == nil {
		t.Fatal("invalid threshold public key accepted")
	}
}
