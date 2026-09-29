package lqc

import (
	"math/big"
	"testing"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFCertificateReplayShareV1(t *testing.T, id uint64, scalar byte) *rabbitvrf.SecretShare {
	t.Helper()
	encoded := make([]byte, rabbitvrf.SecretKeySize)
	encoded[len(encoded)-1] = scalar
	share, err := rabbitvrf.SecretShareFromBytes(id, encoded)
	if err != nil {
		t.Fatal(err)
	}
	return share
}

func TestRabbitVRFKeysetCertificateVerificationSharesReplayV1(t *testing.T) {
	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-certificate-replay-committee")),
		4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if context.Threshold != 3 {
		t.Fatalf("threshold=%d want=3", context.Threshold)
	}

	scalars := []byte{15, 33, 61, 99}
	allShares := make([]RabbitVRFVerificationShareV1, 4)
	for index, scalar := range scalars {
		share := rabbitVRFCertificateReplayShareV1(t, uint64(index+1), scalar)
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		allShares[index] = RabbitVRFVerificationShareV1{
			ShareID:   uint64(index + 1),
			PublicKey: publicKey,
		}
	}

	master := rabbitVRFCertificateReplayShareV1(t, 99, 7)
	thresholdPublicKey, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	transcriptRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-certificate-replay-transcript"))
	keysetRoot, canonicalShares, err := RabbitVRFKeysetRootV1(
		context.ChainID,
		context.TargetVRFEpoch,
		context.CommitteeRoot,
		context.CommitteeSize,
		context.Threshold,
		thresholdPublicKey,
		transcriptRoot,
		allShares,
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
	}

	certificate := RabbitVRFKeysetCertificateV1{
		Version:            RabbitVRFKeysetCertificateVersionV1,
		SessionID:          sessionID,
		KeysetRoot:         keysetRoot,
		ThresholdPublicKey: thresholdPublicKey,
		TranscriptRoot:     transcriptRoot,
		VerificationShareSamples: append(
			[]RabbitVRFVerificationShareV1(nil),
			canonicalShares[:int(context.Threshold)]...,
		),
		Signatures: [][]byte{make([]byte, 65), make([]byte, 65), make([]byte, 65), nil},
	}

	reconstructed, err := RabbitVRFKeysetCertificateVerificationSharesV1(context, certificate)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconstructed) != len(canonicalShares) {
		t.Fatalf("reconstructed=%d want=%d", len(reconstructed), len(canonicalShares))
	}
	for index := range reconstructed {
		if reconstructed[index] != canonicalShares[index] {
			t.Fatalf("verification share %d changed during replay reconstruction", index+1)
		}
	}

	tampered := certificate
	tampered.VerificationShareSamples = append(
		[]RabbitVRFVerificationShareV1(nil),
		certificate.VerificationShareSamples...,
	)
	tampered.VerificationShareSamples[1].PublicKey = canonicalShares[3].PublicKey
	if _, err := RabbitVRFKeysetCertificateVerificationSharesV1(context, tampered); err == nil {
		t.Fatal("tampered verification sample accepted")
	}

	short := certificate
	short.VerificationShareSamples = append(
		[]RabbitVRFVerificationShareV1(nil),
		certificate.VerificationShareSamples[:2]...,
	)
	if _, err := RabbitVRFKeysetCertificateVerificationSharesV1(context, short); err == nil {
		t.Fatal("insufficient verification samples accepted")
	}
}

func TestRabbitVRFKeysetCertificateMaxCommitteeHeaderSizeV1(t *testing.T) {
	const (
		committeeSize = 128
		threshold     = 86
	)

	samples := make([]RabbitVRFVerificationShareV1, threshold)
	for index := range samples {
		share := rabbitVRFCertificateReplayShareV1(t, uint64(index+1), byte(index+1))
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		samples[index] = RabbitVRFVerificationShareV1{
			ShareID:   uint64(index + 1),
			PublicKey: publicKey,
		}
	}

	thresholdKey, err := rabbitVRFCertificateReplayShareV1(t, 99, 1).PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	signatures := make([][]byte, committeeSize)
	for index := 0; index < threshold; index++ {
		signatures[index] = make([]byte, 65)
		signatures[index][0] = 1
	}

	certificate := RabbitVRFKeysetCertificateV1{
		Version:                  RabbitVRFKeysetCertificateVersionV1,
		SessionID:                gethcrypto.Keccak256Hash([]byte("rabbit-vrf-size-session")),
		KeysetRoot:               gethcrypto.Keccak256Hash([]byte("rabbit-vrf-size-keyset")),
		ThresholdPublicKey:       thresholdKey,
		TranscriptRoot:           gethcrypto.Keccak256Hash([]byte("rabbit-vrf-size-transcript")),
		VerificationShareSamples: samples,
		Signatures:               signatures,
	}

	extra, err := encodeLQCHeaderExtraV5(
		100,
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-size-registry")),
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-size-work")),
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-size-claims")),
		nil,
		nil,
		nil,
		[]RabbitVRFKeysetCertificateV1{certificate},
		nil,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra) > MaxRegistryHeaderExtraSize {
		t.Fatalf("max committee keyset certificate exceeds header: %d > %d", len(extra), MaxRegistryHeaderExtraSize)
	}
	t.Logf(
		"max committee keyset certificate header size=%d limit=%d remaining=%d",
		len(extra),
		MaxRegistryHeaderExtraSize,
		MaxRegistryHeaderExtraSize-len(extra),
	)
}
