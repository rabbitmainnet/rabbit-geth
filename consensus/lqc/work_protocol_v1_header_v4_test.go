package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestLQCHeaderV4RoundTripCanonicalClaims(t *testing.T) {
	claims := []CommitteeParticipationClaimGroupV1{
		{TargetBlock: 99, Participations: []CompactCommitteeParticipationV1{compactClaimV1(7), compactClaimV1(1)}},
		{TargetBlock: 93, Participations: []CompactCommitteeParticipationV1{compactClaimV1(3)}},
	}
	extra, err := EncodeLQCHeaderExtraV4(
		101,
		common.HexToHash("0x1111"),
		common.HexToHash("0x2222"),
		common.HexToHash("0x3333"),
		nil,
		nil,
		claims,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ValidateLQCHeaderExtraV4(101, MaxWorkTicketsPerBlockV1, extra)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Version != LQCHeaderEnvelopeVersionV4 ||
		decoded.CommitteeClaimRoot != common.HexToHash("0x3333") ||
		len(decoded.CommitteeParticipationClaims) != 2 ||
		decoded.CommitteeParticipationClaims[0].TargetBlock != 93 ||
		decoded.CommitteeParticipationClaims[1].Participations[0].Position != 1 {
		t.Fatalf("unexpected V4 envelope: %+v", decoded)
	}
	_, seal, err := splitProducerSeal(extra)
	if err != nil || len(seal) != ProducerSealLength || !bytes.Equal(seal, make([]byte, ProducerSealLength)) {
		t.Fatal("V4 producer seal suffix contract changed")
	}
}

func TestLQCHeaderV4RejectsNonCanonicalAndCapacity(t *testing.T) {
	claims := []CommitteeParticipationClaimGroupV1{{
		TargetBlock:    100,
		Participations: []CompactCommitteeParticipationV1{compactClaimV1(1), compactClaimV1(0)},
	}}
	envelope := LQCHeaderEnvelopeV4{
		Version:                      LQCHeaderEnvelopeVersionV4,
		BlockNumber:                  101,
		RegistryRoot:                 common.HexToHash("0x1111"),
		WorkStateRoot:                common.HexToHash("0x2222"),
		CommitteeClaimRoot:           common.HexToHash("0x3333"),
		CommitteeParticipationClaims: claims,
	}
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		t.Fatal(err)
	}
	extra := appendEmptyProducerSeal(append(append([]byte(nil), registryHeaderMagic...), payload...))
	if _, err := DecodeLQCHeaderExtraV4(extra, MaxWorkTicketsPerBlockV1); !errors.Is(err, ErrNonCanonicalCommitteeClaimsV1) {
		t.Fatalf("non-canonical error=%v", err)
	}

	operations := make([]RegistryOperation, MaxRegistryOperationsWithCommitteeClaimsV1+1)
	for index := range operations {
		operations[index] = RegistryOperation{
			Version:    RegistryProtocolVersion,
			Action:     RegistryActionHeartbeat,
			Address:    common.BigToAddress(newUint64BigV4(uint64(index + 1))),
			Sequence:   1,
			ValidUntil: 200,
			Signature:  make([]byte, 65),
		}
	}
	_, err = EncodeLQCHeaderExtraV4(
		101,
		common.HexToHash("0x1111"),
		common.HexToHash("0x2222"),
		common.HexToHash("0x3333"),
		operations,
		nil,
		[]CommitteeParticipationClaimGroupV1{{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(0)}}},
		MaxWorkTicketsPerBlockV1,
	)
	if !errors.Is(err, ErrTooManyClaimRegistryOperationsV4) {
		t.Fatalf("claim capacity error=%v", err)
	}
}

func newUint64BigV4(value uint64) *big.Int {
	return new(big.Int).SetUint64(value)
}
