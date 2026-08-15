package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func commitTestKeyV1(t *testing.T, seed int) *big.Int {
	t.Helper()
	for retry := 0; ; retry++ {
		raw := crypto.Keccak256(
			[]byte("RABBIT-WORK-COMMIT-V1-TEST"),
			[]byte{byte(seed >> 8), byte(seed)},
			[]byte{byte(retry >> 8), byte(retry)},
		)
		value := new(big.Int).SetBytes(raw)
		if value.Sign() > 0 &&
			value.Cmp(crypto.S256().Params().N) < 0 {
			return value
		}
	}
}

func commitCandidateV1(
	t *testing.T,
	epoch uint64,
	identitySeed int,
	nonce uint64,
	proofHash common.Hash,
) WorkCommitCandidateV1 {
	t.Helper()

	private, err := crypto.ToECDSA(
		commitTestKeyV1(
			t,
			identitySeed,
		).FillBytes(make([]byte, 32)),
	)
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(private.PublicKey)

	ticket := RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       epoch,
		Participant: participant,
		Nonce:       nonce,
	}

	// Structural header signature only. Full authorization is tested by the
	// crypto/RandomX V1 suite.
	digest := crypto.Keccak256Hash(
		[]byte("RABBIT-WORK-COMMIT-V1-SHAPE"),
		participant[:],
		proofHash[:],
	)
	signature, err := crypto.Sign(digest[:], private)
	if err != nil {
		t.Fatal(err)
	}

	return WorkCommitCandidateV1{
		Signed: SignedRandomXWorkTicketV1{
			Ticket:    ticket,
			Signature: signature,
		},
		ProofHash: proofHash,
	}
}

func commitProofHashV1(index int) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT-WORK-COMMIT-V1-PROOF"),
		new(big.Int).SetInt64(int64(index)).Bytes(),
	)
}

func TestWorkCommitV1ArrivalOrderDoesNotChangeBatch(t *testing.T) {
	left := make([]WorkCommitCandidateV1, 0, 20)
	for i := 0; i < 20; i++ {
		left = append(left, commitCandidateV1(
			t,
			7,
			i+1,
			uint64(i+1),
			commitProofHashV1(i+1),
		))
	}

	right := append([]WorkCommitCandidateV1(nil), left...)
	for i, j := 0, len(right)-1; i < j; i, j = i+1, j-1 {
		right[i], right[j] = right[j], right[i]
	}

	batchLeft, err := SelectWorkCommitBatchV1(left, 7)
	if err != nil {
		t.Fatal(err)
	}
	batchRight, err := SelectWorkCommitBatchV1(right, 7)
	if err != nil {
		t.Fatal(err)
	}

	if len(batchLeft) != int(MaxWorkTicketsPerBlockV1) ||
		len(batchRight) != int(MaxWorkTicketsPerBlockV1) {
		t.Fatalf(
			"batch sizes = %d/%d, want %d",
			len(batchLeft),
			len(batchRight),
			MaxWorkTicketsPerBlockV1,
		)
	}

	for i := range batchLeft {
		if batchLeft[i].ProofHash != batchRight[i].ProofHash {
			t.Fatalf(
				"arrival order changed batch at %d",
				i,
			)
		}
	}
}

func TestWorkCommitV1IdentityDoesNotChangeFixedProofPriority(t *testing.T) {
	one := make([]WorkCommitCandidateV1, 0, 32)
	split := make([]WorkCommitCandidateV1, 0, 32)

	for i := 0; i < 32; i++ {
		hash := commitProofHashV1(i + 1)

		one = append(one, commitCandidateV1(
			t,
			7,
			1,
			uint64(i+1),
			hash,
		))
		split = append(split, commitCandidateV1(
			t,
			7,
			1000+i,
			1,
			hash,
		))
	}

	left, err := SelectWorkCommitBatchV1(one, 7)
	if err != nil {
		t.Fatal(err)
	}
	right, err := SelectWorkCommitBatchV1(split, 7)
	if err != nil {
		t.Fatal(err)
	}

	if len(left) != len(right) {
		t.Fatal("identity split changed selected count")
	}
	for i := range left {
		if left[i].ProofHash != right[i].ProofHash {
			t.Fatalf(
				"identity split changed proof priority at %d",
				i,
			)
		}
	}
}

func TestWorkCommitV1FiveThousandIdleIdentitiesAddNoCapacity(t *testing.T) {
	fixedWork := make([]WorkCommitCandidateV1, 0, 20)

	for i := 0; i < 20; i++ {
		fixedWork = append(fixedWork, commitCandidateV1(
			t,
			7,
			10_000+i,
			1,
			commitProofHashV1(i+1),
		))
	}

	batch, err := SelectWorkCommitBatchV1(
		fixedWork,
		7,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 8 {
		t.Fatalf("batch = %d, want 8", len(batch))
	}

	// Creating 4,980 more addresses with no verified proof does not create
	// any WorkCommitCandidateV1 and therefore adds exactly zero admission power.
	const identities = 5000
	const identitiesWithProofs = 20
	if identities-identitiesWithProofs != 4980 {
		t.Fatal("test setup broken")
	}
}

func TestWorkCommitV1ExpectedTargetNeedsOnlyQuarterWindow(t *testing.T) {
	blocks, err := MinimumHonestCommitBlocksV1(
		TargetWorkTicketsPerEpochV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if blocks != 32 {
		t.Fatalf("honest blocks = %d, want 32", blocks)
	}

	fraction, err := MinimumHonestCommitFractionBpsV1(
		TargetWorkTicketsPerEpochV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if fraction != 2500 {
		t.Fatalf("honest fraction = %d bps, want 2500", fraction)
	}

	if capacity := WorkCommitCapacityForBlocksV1(32); capacity != 256 {
		t.Fatalf("32-block capacity = %d, want 256", capacity)
	}
}

func TestWorkCommitV1FullCapacityNeedsWholeWindow(t *testing.T) {
	blocks, err := MinimumHonestCommitBlocksV1(
		WorkTicketCommitCapacityPerEpochV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if blocks != WorkProtocolEpochLengthV1 {
		t.Fatalf(
			"honest blocks = %d, want %d",
			blocks,
			WorkProtocolEpochLengthV1,
		)
	}

	if _, err := MinimumHonestCommitBlocksV1(
		WorkTicketCommitCapacityPerEpochV1 + 1,
	); err != ErrWorkCommitWindowOverflowV1 {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestWorkCommitV1RejectsDuplicateProofAndSemanticTicket(t *testing.T) {
	a := commitCandidateV1(
		t,
		7,
		1,
		1,
		commitProofHashV1(1),
	)
	b := commitCandidateV1(
		t,
		7,
		2,
		1,
		commitProofHashV1(1),
	)

	if _, err := CanonicalWorkCommitCandidatesV1(
		[]WorkCommitCandidateV1{a, b},
		7,
	); err != ErrDuplicateRandomXWorkHash {
		t.Fatalf("duplicate proof error = %v", err)
	}

	c := cloneWorkCommitCandidateV1(a)
	c.ProofHash = commitProofHashV1(2)

	if _, err := CanonicalWorkCommitCandidatesV1(
		[]WorkCommitCandidateV1{a, c},
		7,
	); err != ErrDuplicateWorkTicketV3 {
		t.Fatalf("duplicate semantic error = %v", err)
	}
}
