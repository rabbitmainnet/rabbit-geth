package lqc

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func signedHeaderWorkTicketV3(
	t *testing.T,
	epoch uint64,
	participantSeed byte,
	nonce uint64,
) SignedRandomXWorkTicketV1 {
	t.Helper()

	material := make([]byte, 32)
	material[31] = participantSeed
	if participantSeed == 0 {
		material[31] = 1
	}
	key, err := crypto.ToECDSA(material)
	if err != nil {
		t.Fatal(err)
	}

	participant := crypto.PubkeyToAddress(key.PublicKey)
	ticket := RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       epoch,
		Participant: participant,
		Nonce:       nonce,
	}

	digest := crypto.Keccak256Hash(
		[]byte("RABBIT-LQC-HEADER-V3-TEST"),
		participant.Bytes(),
		newUint64BytesV3(epoch),
		newUint64BytesV3(nonce),
	)
	signature, err := crypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}

	return SignedRandomXWorkTicketV1{
		Ticket:    ticket,
		Signature: signature,
	}
}

func newUint64BytesV3(value uint64) []byte {
	if value == 0 {
		return []byte{0}
	}
	out := make([]byte, 0, 8)
	for shift := 56; shift >= 0; shift -= 8 {
		b := byte(value >> uint(shift))
		if len(out) == 0 && b == 0 {
			continue
		}
		out = append(out, b)
	}
	return out
}

func TestLQCHeaderV3RoundTripCanonicalWorkOrder(t *testing.T) {
	registryRoot := common.HexToHash("0x1111")
	workRoot := common.HexToHash("0x2222")

	a := signedHeaderWorkTicketV3(t, 7, 1, 3)
	b := signedHeaderWorkTicketV3(t, 7, 2, 1)
	c := signedHeaderWorkTicketV3(t, 7, 1, 1)

	left, err := EncodeLQCHeaderExtraV3(
		300,
		registryRoot,
		workRoot,
		nil,
		[]SignedRandomXWorkTicketV1{a, b, c},
		16,
	)
	if err != nil {
		t.Fatal(err)
	}

	right, err := EncodeLQCHeaderExtraV3(
		300,
		registryRoot,
		workRoot,
		nil,
		[]SignedRandomXWorkTicketV1{c, a, b},
		16,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(left, right) {
		t.Fatal("arrival order changed canonical V3 encoding")
	}

	decoded, err := ValidateLQCHeaderExtraV3(
		300,
		16,
		left,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Version != LQCHeaderEnvelopeVersionV3 ||
		decoded.RegistryRoot != registryRoot ||
		decoded.WorkStateRoot != workRoot ||
		len(decoded.WorkTickets) != 3 {
		t.Fatalf("unexpected V3 envelope: %+v", decoded)
	}
}

func TestLQCHeaderV3RejectsSemanticDuplicateTicket(t *testing.T) {
	registryRoot := common.HexToHash("0x1111")
	workRoot := common.HexToHash("0x2222")

	a := signedHeaderWorkTicketV3(t, 7, 1, 3)
	duplicate := cloneSignedRandomXWorkTicketV1(a)

	_, err := EncodeLQCHeaderExtraV3(
		300,
		registryRoot,
		workRoot,
		nil,
		[]SignedRandomXWorkTicketV1{a, duplicate},
		16,
	)
	if err != ErrDuplicateWorkTicketV3 {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateWorkTicketV3)
	}
}

func TestLQCHeaderV3RejectsTooManyWorkTickets(t *testing.T) {
	registryRoot := common.HexToHash("0x1111")
	workRoot := common.HexToHash("0x2222")

	tickets := []SignedRandomXWorkTicketV1{
		signedHeaderWorkTicketV3(t, 7, 1, 1),
		signedHeaderWorkTicketV3(t, 7, 2, 1),
	}

	_, err := EncodeLQCHeaderExtraV3(
		300,
		registryRoot,
		workRoot,
		nil,
		tickets,
		1,
	)
	if err != ErrTooManyWorkTicketsV3 {
		t.Fatalf("error = %v, want %v", err, ErrTooManyWorkTicketsV3)
	}
}

func TestLQCHeaderV3PreservesProducerSealSuffixContract(t *testing.T) {
	extra, err := EncodeLQCHeaderExtraV3(
		300,
		common.HexToHash("0x1111"),
		common.HexToHash("0x2222"),
		nil,
		[]SignedRandomXWorkTicketV1{
			signedHeaderWorkTicketV3(t, 7, 1, 1),
		},
		16,
	)
	if err != nil {
		t.Fatal(err)
	}

	payload, seal, err := splitProducerSeal(extra)
	if err != nil {
		t.Fatal(err)
	}
	if len(seal) != ProducerSealLength {
		t.Fatalf("seal len = %d, want %d", len(seal), ProducerSealLength)
	}
	for _, value := range seal {
		if value != 0 {
			t.Fatal("new V3 envelope did not reserve an empty producer seal")
		}
	}

	withSyntheticSeal := append([]byte(nil), payload...)
	synthetic := make([]byte, ProducerSealLength)
	synthetic[0] = 1
	synthetic[ProducerSealLength-1] = 1
	withSyntheticSeal = append(withSyntheticSeal, synthetic...)

	decoded, err := DecodeLQCHeaderExtraV3(
		withSyntheticSeal,
		16,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.BlockNumber != 300 {
		t.Fatalf("block = %d, want 300", decoded.BlockNumber)
	}
}

func TestLQCHeaderV3RejectsZeroRootsAndBlockMismatch(t *testing.T) {
	if _, err := EncodeLQCHeaderExtraV3(
		300,
		common.Hash{},
		common.HexToHash("0x2222"),
		nil,
		nil,
		16,
	); err != ErrInvalidLQCHeaderExtraV3 {
		t.Fatalf("zero registry root error = %v", err)
	}

	if _, err := EncodeLQCHeaderExtraV3(
		300,
		common.HexToHash("0x1111"),
		common.Hash{},
		nil,
		nil,
		16,
	); err != ErrInvalidLQCHeaderExtraV3 {
		t.Fatalf("zero work root error = %v", err)
	}

	extra, err := EncodeLQCHeaderExtraV3(
		300,
		common.HexToHash("0x1111"),
		common.HexToHash("0x2222"),
		nil,
		nil,
		16,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateLQCHeaderExtraV3(
		301,
		16,
		extra,
	); !errors.Is(err, ErrLQCHeaderBlockMismatchV3) {
		t.Fatalf("block mismatch error = %v", err)
	}
}

func TestLQCHeaderV3RespectsExisting16KiBExtraBound(t *testing.T) {
	tickets := make([]SignedRandomXWorkTicketV1, 0, 256)
	for i := 0; i < 256; i++ {
		tickets = append(
			tickets,
			signedHeaderWorkTicketV3(
				t,
				7,
				byte((i%250)+1),
				uint64(i+1),
			),
		)
	}

	_, err := EncodeLQCHeaderExtraV3(
		300,
		common.HexToHash("0x1111"),
		common.HexToHash("0x2222"),
		nil,
		tickets,
		256,
	)
	if !errors.Is(err, ErrInvalidLQCHeaderExtraV3) {
		t.Fatalf("oversized V3 error = %v", err)
	}
}
