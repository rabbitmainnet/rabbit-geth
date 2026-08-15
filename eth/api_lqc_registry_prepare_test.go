package eth

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func testPrepareParameters() RegistryParametersResult {
	return RegistryParametersResult{
		ChainID:              (*hexutil.Big)(big.NewInt(928)),
		CurrentBlock:         20,
		NextBlock:            21,
		ProofDifficulty:      1,
		MaxOperationLifetime: 256,
		RegistryRoot:         common.HexToHash("0x1234"),
		ActiveForNextBlock:   true,
	}
}

func TestPrepareRegistryRegistrationNewParticipant(t *testing.T) {
	address := common.HexToAddress("0x1111111111111111111111111111111111111111")
	parameters := testPrepareParameters()
	participant := RegistryParticipantResult{
		Address:        address,
		Exists:         false,
		Active:         false,
		CanonicalBlock: parameters.CurrentBlock,
		RegistryRoot:   parameters.RegistryRoot,
	}

	operation, attempts, err := prepareRegistryRegistration(context.Background(), parameters, participant, address)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Action != lqc.RegistryActionRegister {
		t.Fatalf("action = %d", operation.Action)
	}
	if operation.Sequence != 1 {
		t.Fatalf("sequence = %d want 1", operation.Sequence)
	}
	wantValidUntil := uint64(parameters.NextBlock) + uint64(parameters.MaxOperationLifetime) - 1
	if operation.ValidUntil != wantValidUntil {
		t.Fatalf("validUntil = %d want %d", operation.ValidUntil, wantValidUntil)
	}
	if attempts == 0 {
		t.Fatal("expected at least one LightHash attempt")
	}
	chainID := new(big.Int).Set((*big.Int)(parameters.ChainID))
	if !lqc.LightHashMeetsDifficulty(lqc.RegistryOperationSigningHash(chainID, operation), uint64(parameters.ProofDifficulty)) {
		t.Fatal("prepared LightHash proof does not meet difficulty")
	}
	if len(lqc.RegistryOperationWalletMessage(chainID, operation)) == 0 {
		t.Fatal("wallet message is empty")
	}
}

func TestPrepareRegistryRegistrationExistingInactiveIncrementsSequence(t *testing.T) {
	address := common.HexToAddress("0x2222222222222222222222222222222222222222")
	parameters := testPrepareParameters()
	participant := RegistryParticipantResult{
		Address:        address,
		Exists:         true,
		Active:         false,
		Sequence:       7,
		CanonicalBlock: parameters.CurrentBlock,
		RegistryRoot:   parameters.RegistryRoot,
	}

	operation, _, err := prepareRegistryRegistration(context.Background(), parameters, participant, address)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Sequence != 8 {
		t.Fatalf("sequence = %d want 8", operation.Sequence)
	}
}

func TestPrepareRegistryRegistrationRejectsActiveParticipant(t *testing.T) {
	address := common.HexToAddress("0x3333333333333333333333333333333333333333")
	parameters := testPrepareParameters()
	participant := RegistryParticipantResult{
		Address:        address,
		Exists:         true,
		Active:         true,
		Sequence:       1,
		CanonicalBlock: parameters.CurrentBlock,
		RegistryRoot:   parameters.RegistryRoot,
	}

	if _, _, err := prepareRegistryRegistration(context.Background(), parameters, participant, address); err != lqc.ErrParticipantAlreadyActive {
		t.Fatalf("err = %v want %v", err, lqc.ErrParticipantAlreadyActive)
	}
}
