package lqc

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func headerRuntimeHasherV1(
	epochKey common.Hash,
	input []byte,
) (common.Hash, error) {
	return crypto.Keccak256Hash(epochKey.Bytes(), input), nil
}

func headerRuntimeOpenEligibilityV1(common.Address) error {
	return nil
}

func headerRuntimeSignedTicketV1(
	t *testing.T,
	chainID *big.Int,
	epoch uint64,
	datasetAnchor common.Hash,
	challengeAnchor common.Hash,
	nonce uint64,
	keyIndex uint64,
) SignedRandomXWorkTicketV1 {
	t.Helper()

	key, err := crypto.HexToECDSA(
		fmt.Sprintf("%064x", keyIndex),
	)
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
	epochKey, err := RandomXWorkDatasetKeyV1(
		chainID,
		epoch,
		datasetAnchor,
	)
	if err != nil {
		t.Fatal(err)
	}
	input, err := RandomXWorkChallengeInputV1(
		chainID,
		challengeAnchor,
		ticket,
	)
	if err != nil {
		t.Fatal(err)
	}
	proofHash, err := headerRuntimeHasherV1(epochKey, input)
	if err != nil {
		t.Fatal(err)
	}
	signingHash, err := RandomXWorkSigningHashV1(
		chainID,
		challengeAnchor,
		ticket,
		proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(signingHash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	return SignedRandomXWorkTicketV1{
		Ticket:    ticket,
		Signature: signature,
	}
}

func headerRuntimeAdvanceTo128V1(
	t *testing.T,
	chainID *big.Int,
	initialDifficulty *big.Int,
) (
	*CanonicalWorkRuntimeStateV1,
	common.Hash,
	common.Hash,
) {
	t.Helper()

	genesis := crypto.Keccak256Hash(
		[]byte("header-runtime-genesis"),
	)
	state, err := NewCanonicalWorkRuntimeStateV1(
		chainID,
		0,
		genesis,
		WorkProtocolEpochLengthV1,
		initialDifficulty,
	)
	if err != nil {
		t.Fatal(err)
	}

	var block1 common.Hash
	for number := uint64(1); number <= 128; number++ {
		childHash := crypto.Keccak256Hash(
			[]byte("header-runtime-block"),
			new(big.Int).SetUint64(number).Bytes(),
		)
		next, err := state.ApplyVerifiedBlockV1(
			chainID,
			number,
			childHash,
			state.Work.Hash,
			common.Hash{},
			nil,
		)
		if err != nil {
			t.Fatalf("block %d: %v", number, err)
		}
		state = next
		if number == 1 {
			block1 = childHash
		}
	}
	return state, genesis, block1
}

func TestLQCHeaderV3CanonicalWorkV1FirstCommitWindow(t *testing.T) {
	chainID := big.NewInt(928)
	parent, datasetAnchor, challengeAnchor :=
		headerRuntimeAdvanceTo128V1(
			t,
			chainID,
			big.NewInt(1),
		)

	ticket := headerRuntimeSignedTicketV1(
		t,
		chainID,
		1,
		datasetAnchor,
		challengeAnchor,
		1,
		1,
	)

	ctx := LQCHeaderWorkRuntimeContextV1{
		ChainID:         chainID,
		Parent:          parent,
		BlockNumber:     129,
		RegistryRoot:    crypto.Keccak256Hash([]byte("registry-root")),
		DatasetAnchor:   datasetAnchor,
		ChallengeAnchor: challengeAnchor,
		Eligibility:     headerRuntimeOpenEligibilityV1,
		Hasher:          headerRuntimeHasherV1,
	}

	extra, builtRoot, err := BuildLQCHeaderExtraV3WithCanonicalWorkV1(
		ctx,
		nil,
		[]SignedRandomXWorkTicketV1{ticket},
	)
	if err != nil {
		t.Fatal(err)
	}

	childHash := crypto.Keccak256Hash([]byte("real-child-129"))
	envelope, next, err :=
		ValidateAndApplyLQCHeaderExtraV3WithCanonicalWorkV1(
			ctx,
			childHash,
			extra,
		)
	if err != nil {
		t.Fatal(err)
	}

	if envelope.WorkStateRoot != builtRoot ||
		next.StateRoot != builtRoot {
		t.Fatal("build/validate/runtime roots diverged")
	}
	if next.Work.Hash != childHash {
		t.Fatal("runtime not linked to real child hash")
	}
	if len(next.Work.CommitSeats) != 1 {
		t.Fatalf("commit seats=%d want=1", len(next.Work.CommitSeats))
	}
}

func TestLQCHeaderV3CanonicalWorkV1RejectsTamperedRoot(t *testing.T) {
	chainID := big.NewInt(928)
	parent, datasetAnchor, challengeAnchor :=
		headerRuntimeAdvanceTo128V1(t, chainID, big.NewInt(1))

	ticket := headerRuntimeSignedTicketV1(
		t,
		chainID,
		1,
		datasetAnchor,
		challengeAnchor,
		2,
		2,
	)

	ctx := LQCHeaderWorkRuntimeContextV1{
		ChainID:         chainID,
		Parent:          parent,
		BlockNumber:     129,
		RegistryRoot:    crypto.Keccak256Hash([]byte("registry-root")),
		DatasetAnchor:   datasetAnchor,
		ChallengeAnchor: challengeAnchor,
		Eligibility:     headerRuntimeOpenEligibilityV1,
		Hasher:          headerRuntimeHasherV1,
	}

	extra, err := EncodeLQCHeaderExtraV3(
		129,
		ctx.RegistryRoot,
		crypto.Keccak256Hash([]byte("tampered-work-root")),
		nil,
		[]SignedRandomXWorkTicketV1{ticket},
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ValidateAndApplyLQCHeaderExtraV3WithCanonicalWorkV1(
		ctx,
		crypto.Keccak256Hash([]byte("child")),
		extra,
	)
	if !errors.Is(err, ErrLQCHeaderWorkStateRootMismatchV3) {
		t.Fatalf("error=%v", err)
	}
}

func TestLQCHeaderV3CanonicalWorkV1HardCapsEightTickets(t *testing.T) {
	chainID := big.NewInt(928)
	parent, datasetAnchor, challengeAnchor :=
		headerRuntimeAdvanceTo128V1(t, chainID, big.NewInt(1))

	ctx := LQCHeaderWorkRuntimeContextV1{
		ChainID:         chainID,
		Parent:          parent,
		BlockNumber:     129,
		RegistryRoot:    crypto.Keccak256Hash([]byte("registry-root")),
		DatasetAnchor:   datasetAnchor,
		ChallengeAnchor: challengeAnchor,
		Eligibility:     headerRuntimeOpenEligibilityV1,
		Hasher:          headerRuntimeHasherV1,
	}

	tickets := make([]SignedRandomXWorkTicketV1, 0, 9)
	for i := uint64(1); i <= 9; i++ {
		tickets = append(
			tickets,
			headerRuntimeSignedTicketV1(
				t,
				chainID,
				1,
				datasetAnchor,
				challengeAnchor,
				i,
				i+10,
			),
		)
	}

	_, _, err := BuildLQCHeaderExtraV3WithCanonicalWorkV1(
		ctx,
		nil,
		tickets,
	)
	if !errors.Is(err, ErrTooManyWorkTicketsV3) {
		t.Fatalf("error=%v", err)
	}
}

func TestLQCHeaderV3CanonicalWorkV1NoCommitWindow(t *testing.T) {
	chainID := big.NewInt(928)
	genesis := crypto.Keccak256Hash([]byte("genesis"))
	parent, err := NewCanonicalWorkRuntimeStateV1(
		chainID,
		0,
		genesis,
		WorkProtocolEpochLengthV1,
		big.NewInt(1),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := LQCHeaderWorkRuntimeContextV1{
		ChainID:      chainID,
		Parent:       parent,
		BlockNumber:  1,
		RegistryRoot: crypto.Keccak256Hash([]byte("registry-root")),
		Eligibility:  headerRuntimeOpenEligibilityV1,
		Hasher:       headerRuntimeHasherV1,
	}

	extra, root, err := BuildLQCHeaderExtraV3WithCanonicalWorkV1(
		ctx,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if root == (common.Hash{}) {
		t.Fatal("zero post-work root")
	}

	_, next, err := ValidateAndApplyLQCHeaderExtraV3WithCanonicalWorkV1(
		ctx,
		crypto.Keccak256Hash([]byte("block1")),
		extra,
	)
	if err != nil {
		t.Fatal(err)
	}
	if next.Work.Number != 1 {
		t.Fatalf("number=%d want=1", next.Work.Number)
	}
}
