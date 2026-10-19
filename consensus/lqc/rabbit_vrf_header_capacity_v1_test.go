package lqc

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFHeaderCapacityCertificateV1(t *testing.T) RabbitVRFKeysetCertificateV1 {
	t.Helper()

	const (
		committeeSize = 128
		threshold     = 86
	)

	samples := make([]RabbitVRFVerificationShareV1, threshold)
	for index := range samples {
		share := rabbitVRFCertificateReplayShareV1(
			t,
			uint64(index+1),
			byte(index+1),
		)
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

	return RabbitVRFKeysetCertificateV1{
		Version:                  RabbitVRFKeysetCertificateVersionV1,
		SessionID:                gethcrypto.Keccak256Hash([]byte("rabbit-vrf-capacity-session")),
		KeysetRoot:               gethcrypto.Keccak256Hash([]byte("rabbit-vrf-capacity-keyset")),
		ThresholdPublicKey:       thresholdKey,
		TranscriptRoot:           gethcrypto.Keccak256Hash([]byte("rabbit-vrf-capacity-transcript")),
		VerificationShareSamples: samples,
		Signatures:               signatures,
	}
}

func rabbitVRFHeaderCapacityFinalizationsV1() []RabbitVRFFinalizationV1 {
	values := make([]RabbitVRFFinalizationV1, MaxRabbitVRFFinalizationsPerBlockV1)

	for index := range values {
		requestID := gethcrypto.Keccak256Hash(
			[]byte{byte(index + 1), 0x52, 0x56, 0x46},
		)
		values[index] = RabbitVRFFinalizationV1{
			Version:                         RabbitVRFFinalizationVersionV1,
			RequestID:                       requestID,
			KeysetRoot:                      common.HexToHash("0x1001"),
			Epoch:                           11,
			Round:                           uint64(index + 1),
			Randomness:                      gethcrypto.Keccak256Hash(requestID[:], []byte("randomness")),
			ProofHash:                       gethcrypto.Keccak256Hash(requestID[:], []byte("proof")),
			Signature:                       rabbitvrf.Signature{1},
			ParticipationBitmap:             [RabbitVRFCompactParticipationBitmapBytesV1]byte{1},
			ParticipationAggregateSignature: rabbitvrf.Signature{2},
		}
	}

	return values
}

func TestRabbitVRFHeaderV5CompactFinalizationCapacityV1(t *testing.T) {
	registryRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-capacity-registry"))
	workRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-capacity-work"))
	claimRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-capacity-claims"))
	finalizations := rabbitVRFHeaderCapacityFinalizationsV1()
	certificate := rabbitVRFHeaderCapacityCertificateV1(t)

	withoutCertificate, err := encodeLQCHeaderExtraV5(
		100,
		registryRoot,
		workRoot,
		claimRoot,
		nil,
		nil,
		nil,
		nil,
		finalizations,
		1,
	)
	if err != nil {
		t.Fatalf("32 compact finalizations without certificate must fit: %v", err)
	}
	if len(withoutCertificate) > MaxRegistryHeaderExtraSize {
		t.Fatalf(
			"32 compact finalizations exceed header without certificate: %d > %d",
			len(withoutCertificate),
			MaxRegistryHeaderExtraSize,
		)
	}

	if _, err := encodeLQCHeaderExtraV5(
		100,
		registryRoot,
		workRoot,
		claimRoot,
		nil,
		nil,
		nil,
		[]RabbitVRFKeysetCertificateV1{certificate},
		finalizations,
		1,
	); !errors.Is(err, ErrInvalidLQCHeaderExtraV5) {
		t.Fatalf("max certificate + 32 finalizations unexpectedly accepted: %v", err)
	}

	fitted, err := encodeLQCHeaderExtraV5FittingFinalizationsV1(
		100,
		registryRoot,
		workRoot,
		claimRoot,
		nil,
		nil,
		nil,
		[]RabbitVRFKeysetCertificateV1{certificate},
		finalizations,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err := DecodeLQCHeaderExtraV5(fitted, 1)
	if err != nil {
		t.Fatal(err)
	}

	included := len(envelope.RabbitVRFFinalizations)
	if included <= 0 || included >= len(finalizations) {
		t.Fatalf(
			"capacity packer included=%d want between 1 and %d",
			included,
			len(finalizations)-1,
		)
	}
	if len(fitted) > MaxRegistryHeaderExtraSize {
		t.Fatalf(
			"fitted header exceeds limit: %d > %d",
			len(fitted),
			MaxRegistryHeaderExtraSize,
		)
	}

	canonical, err := CanonicalRabbitVRFFinalizationsV1(finalizations)
	if err != nil {
		t.Fatal(err)
	}
	for index := range envelope.RabbitVRFFinalizations {
		if envelope.RabbitVRFFinalizations[index] != canonical[index] {
			t.Fatalf("capacity packer changed canonical prefix at index %d", index)
		}
	}

	t.Logf(
		"compact VRF capacity: 32-without-cert=%d max-cert-fitted=%d included=%d/%d remaining=%d",
		len(withoutCertificate),
		len(fitted),
		included,
		len(finalizations),
		MaxRegistryHeaderExtraSize-len(fitted),
	)
}

func TestRabbitVRFHeaderV5CapacityDoesNotHideInvalidSuffixV1(t *testing.T) {
	finalizations := rabbitVRFHeaderCapacityFinalizationsV1()
	finalizations[len(finalizations)-1].ProofHash = common.Hash{}

	_, err := encodeLQCHeaderExtraV5FittingFinalizationsV1(
		100,
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-invalid-registry")),
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-invalid-work")),
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-invalid-claims")),
		nil,
		nil,
		nil,
		[]RabbitVRFKeysetCertificateV1{
			rabbitVRFHeaderCapacityCertificateV1(t),
		},
		finalizations,
		1,
	)
	if !errors.Is(err, ErrInvalidRabbitVRFFinalizationV1) {
		t.Fatalf(
			"invalid finalization suffix was hidden by capacity trimming: %v",
			err,
		)
	}
}
