package eth

import (
	"context"
	"errors"
	"math/big"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

var errLQCRegistryUnavailable = errors.New("lqc canonical registry is unavailable")

type LQCRegistryAPI struct {
	eth *Ethereum
}

type RegistryOperationArgs struct {
	Version    hexutil.Uint64 `json:"version"`
	Action     hexutil.Uint64 `json:"action"`
	Address    common.Address `json:"address"`
	Sequence   hexutil.Uint64 `json:"sequence"`
	ValidUntil hexutil.Uint64 `json:"validUntil"`
	ProofNonce hexutil.Uint64 `json:"proofNonce"`
	Signature  hexutil.Bytes  `json:"signature"`
}

type RegistryOperationResult struct {
	Hash       common.Hash    `json:"hash"`
	Version    hexutil.Uint64 `json:"version"`
	Action     hexutil.Uint64 `json:"action"`
	Address    common.Address `json:"address"`
	Sequence   hexutil.Uint64 `json:"sequence"`
	ValidUntil hexutil.Uint64 `json:"validUntil"`
	ProofNonce hexutil.Uint64 `json:"proofNonce"`
	Signature  hexutil.Bytes  `json:"signature"`
}

type RegistryParametersResult struct {
	ChainID              *hexutil.Big   `json:"chainId"`
	ActivationBlock      hexutil.Uint64 `json:"activationBlock"`
	CurrentBlock         hexutil.Uint64 `json:"currentBlock"`
	NextBlock            hexutil.Uint64 `json:"nextBlock"`
	ProofDifficulty      hexutil.Uint64 `json:"proofDifficulty"`
	ActivationDelay      hexutil.Uint64 `json:"activationDelay"`
	HeartbeatWindow      hexutil.Uint64 `json:"heartbeatWindow"`
	HeartbeatGrace       hexutil.Uint64 `json:"heartbeatGrace"`
	MaxOperationLifetime hexutil.Uint64 `json:"maxOperationLifetime"`
	PoolCapacity         hexutil.Uint64 `json:"poolCapacity"`
	RegistryRoot         common.Hash    `json:"registryRoot"`
	ParticipantCount     hexutil.Uint64 `json:"participantCount"`
	ActiveForNextBlock   bool           `json:"activeForNextBlock"`
}

type RegistryParticipantResult struct {
	Address        common.Address `json:"address"`
	Exists         bool           `json:"exists"`
	Active         bool           `json:"active"`
	EligibleNext   bool           `json:"eligibleNext"`
	CanonicalBlock hexutil.Uint64 `json:"canonicalBlock"`
	RegistryRoot   common.Hash    `json:"registryRoot"`
	RegisteredAt   hexutil.Uint64 `json:"registeredAt"`
	LastHeartbeat  hexutil.Uint64 `json:"lastHeartbeat"`
	Sequence       hexutil.Uint64 `json:"sequence"`
}

type RegistrySigningRequestResult struct {
	Operation       RegistryOperationArgs `json:"operation"`
	Message         string                `json:"message"`
	MessageBytes    hexutil.Bytes         `json:"messageBytes"`
	SigningHash     common.Hash           `json:"signingHash"`
	ProofAttempts   hexutil.Uint64        `json:"proofAttempts"`
	ProofDifficulty hexutil.Uint64        `json:"proofDifficulty"`
	CurrentBlock    hexutil.Uint64        `json:"currentBlock"`
	NextBlock       hexutil.Uint64        `json:"nextBlock"`
	RegistryRoot    common.Hash           `json:"registryRoot"`
}

func NewLQCRegistryAPI(eth *Ethereum) *LQCRegistryAPI {
	return &LQCRegistryAPI{eth: eth}
}

func (s *Ethereum) lqcRegistryEngine() *lqc.LQC {
	if s == nil {
		return nil
	}
	engine, _ := s.engine.(*lqc.LQC)
	return engine
}

// PrepareRegistryRegistration prepares a permissionless REGISTER operation using
// only canonical state from this local node. It does not sign and does not submit
// anything. A wallet may sign Message with personal_sign/EIP-191 and return the
// signature through SubmitRegistryOperation.
func (api *LQCRegistryAPI) PrepareRegistryRegistration(ctx context.Context, address common.Address) (RegistrySigningRequestResult, error) {
	if api == nil || api.eth == nil || api.eth.blockchain == nil {
		return RegistrySigningRequestResult{}, errLQCRegistryUnavailable
	}
	if address == (common.Address{}) {
		return RegistrySigningRequestResult{}, lqc.ErrInvalidRegistryAddress
	}

	var (
		parameters  RegistryParametersResult
		participant RegistryParticipantResult
		consistent  bool
	)
	for attempt := 0; attempt < 4; attempt++ {
		var err error
		parameters, err = api.RegistryParameters()
		if err != nil {
			return RegistrySigningRequestResult{}, err
		}
		participant, err = api.RegistryParticipant(address)
		if err != nil {
			return RegistrySigningRequestResult{}, err
		}
		if participant.CanonicalBlock == parameters.CurrentBlock && participant.RegistryRoot == parameters.RegistryRoot {
			consistent = true
			break
		}
		select {
		case <-ctx.Done():
			return RegistrySigningRequestResult{}, ctx.Err()
		default:
		}
	}
	if !consistent {
		return RegistrySigningRequestResult{}, errors.New("canonical registry head changed repeatedly while preparing registration")
	}
	if parameters.ChainID == nil || (*big.Int)(parameters.ChainID).Sign() <= 0 {
		return RegistrySigningRequestResult{}, errLQCRegistryUnavailable
	}
	if !parameters.ActiveForNextBlock {
		return RegistrySigningRequestResult{}, errors.New("lqc canonical registry is not active for the next block")
	}

	operation, attempts, err := prepareRegistryRegistration(ctx, parameters, participant, address)
	if err != nil {
		return RegistrySigningRequestResult{}, err
	}
	chainID := new(big.Int).Set((*big.Int)(parameters.ChainID))
	message := lqc.RegistryOperationWalletMessage(chainID, operation)

	return RegistrySigningRequestResult{
		Operation: RegistryOperationArgs{
			Version:    hexutil.Uint64(operation.Version),
			Action:     hexutil.Uint64(operation.Action),
			Address:    operation.Address,
			Sequence:   hexutil.Uint64(operation.Sequence),
			ValidUntil: hexutil.Uint64(operation.ValidUntil),
			ProofNonce: hexutil.Uint64(operation.ProofNonce),
			Signature:  nil,
		},
		Message:         string(message),
		MessageBytes:    append(hexutil.Bytes(nil), message...),
		SigningHash:     lqc.RegistryOperationWalletSigningHash(chainID, operation),
		ProofAttempts:   hexutil.Uint64(attempts),
		ProofDifficulty: parameters.ProofDifficulty,
		CurrentBlock:    parameters.CurrentBlock,
		NextBlock:       parameters.NextBlock,
		RegistryRoot:    parameters.RegistryRoot,
	}, nil
}

func prepareRegistryRegistration(ctx context.Context, parameters RegistryParametersResult, participant RegistryParticipantResult, address common.Address) (lqc.RegistryOperation, uint64, error) {
	if address == (common.Address{}) {
		return lqc.RegistryOperation{}, 0, lqc.ErrInvalidRegistryAddress
	}
	if parameters.ChainID == nil || (*big.Int)(parameters.ChainID).Sign() <= 0 {
		return lqc.RegistryOperation{}, 0, errLQCRegistryUnavailable
	}
	if uint64(parameters.ProofDifficulty) == 0 || uint64(parameters.MaxOperationLifetime) == 0 {
		return lqc.RegistryOperation{}, 0, errors.New("unsafe lqc registry parameters")
	}
	if participant.Exists && participant.Active {
		return lqc.RegistryOperation{}, 0, lqc.ErrParticipantAlreadyActive
	}

	sequence := uint64(1)
	if participant.Exists {
		sequence = uint64(participant.Sequence) + 1
		if sequence == 0 {
			return lqc.RegistryOperation{}, 0, lqc.ErrInvalidRegistrySequence
		}
	}

	maxLifetime := uint64(parameters.MaxOperationLifetime)
	lifetime := maxLifetime
	if lifetime == 0 {
		return lqc.RegistryOperation{}, 0, errors.New("zero lqc registry operation lifetime")
	}

	nextBlock := uint64(parameters.NextBlock)
	offset := lifetime - 1
	if ^uint64(0)-nextBlock < offset {
		return lqc.RegistryOperation{}, 0, errors.New("lqc registry validity overflow")
	}

	operation := lqc.RegistryOperation{
		Version:    lqc.RegistryProtocolVersion,
		Action:     lqc.RegistryActionRegister,
		Address:    address,
		Sequence:   sequence,
		ValidUntil: nextBlock + offset,
	}

	chainID := new(big.Int).Set((*big.Int)(parameters.ChainID))
	difficulty := uint64(parameters.ProofDifficulty)

	nonce, attempts, err := findRegistryProofNonce(ctx, chainID, operation, difficulty)
	if err != nil {
		return lqc.RegistryOperation{}, attempts, err
	}
	operation.ProofNonce = nonce
	return operation, attempts, nil
}

func findRegistryProofNonce(ctx context.Context, chainID *big.Int, operation lqc.RegistryOperation, difficulty uint64) (uint64, uint64, error) {
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > 32 {
		workers = 32
	}

	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	found := make(chan uint64, 1)
	var wg sync.WaitGroup
	step := uint64(workers)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(nonce uint64) {
			defer wg.Done()
			candidate := operation
			iterations := uint64(0)

			for {
				if iterations&255 == 0 {
					select {
					case <-searchCtx.Done():
						return
					default:
					}
				}

				candidate.ProofNonce = nonce
				hash := lqc.RegistryOperationSigningHash(chainID, candidate)
				if lqc.LightHashMeetsDifficulty(hash, difficulty) {
					select {
					case found <- nonce:
						cancel()
					default:
					}
					return
				}

				if ^uint64(0)-nonce < step {
					return
				}
				nonce += step
				iterations++
			}
		}(uint64(worker))
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	attemptsFor := func(nonce uint64) uint64 {
		if nonce == ^uint64(0) {
			return ^uint64(0)
		}
		return nonce + 1
	}

	select {
	case nonce := <-found:
		return nonce, attemptsFor(nonce), nil
	case <-ctx.Done():
		select {
		case nonce := <-found:
			return nonce, attemptsFor(nonce), nil
		default:
		}
		return 0, 0, ctx.Err()
	case <-done:
		select {
		case nonce := <-found:
			return nonce, attemptsFor(nonce), nil
		default:
		}
		return 0, ^uint64(0), errors.New("lqc LightHash nonce space exhausted")
	}
}

func (api *LQCRegistryAPI) SubmitRegistryOperation(ctx context.Context, args RegistryOperationArgs) (common.Hash, error) {
	_ = ctx
	if api == nil || api.eth == nil || api.eth.blockchain == nil {
		return common.Hash{}, errLQCRegistryUnavailable
	}
	engine := api.eth.lqcRegistryEngine()
	if engine == nil {
		return common.Hash{}, errLQCRegistryUnavailable
	}
	if uint64(args.Version) > uint64(^uint8(0)) {
		return common.Hash{}, lqc.ErrInvalidRegistryVersion
	}
	if uint64(args.Action) > uint64(^uint8(0)) {
		return common.Hash{}, lqc.ErrInvalidRegistryAction
	}
	operation := args.operation()
	hash, err := engine.SubmitRegistryOperation(api.eth.blockchain, operation)
	if err != nil {
		return common.Hash{}, err
	}
	if api.eth.registryNetwork != nil {
		api.eth.registryNetwork.BroadcastOperations([]lqc.RegistryOperation{operation}, "")
	}
	return hash, nil
}

func (api *LQCRegistryAPI) RegistryPoolStatus() (lqc.RegistryPoolStatus, error) {
	if api == nil || api.eth == nil || api.eth.lqcRegistryEngine() == nil {
		return lqc.RegistryPoolStatus{}, errLQCRegistryUnavailable
	}
	return api.eth.lqcRegistryEngine().RegistryOperationPoolStatus(), nil
}

func (api *LQCRegistryAPI) PendingRegistryOperations() ([]RegistryOperationResult, error) {
	if api == nil || api.eth == nil || api.eth.blockchain == nil {
		return nil, errLQCRegistryUnavailable
	}
	engine := api.eth.lqcRegistryEngine()
	if engine == nil {
		return nil, errLQCRegistryUnavailable
	}
	chainID := api.eth.blockchain.Config().ChainID
	operations := engine.PendingRegistryOperations(api.eth.blockchain)
	results := make([]RegistryOperationResult, 0, len(operations))
	for _, operation := range operations {
		results = append(results, registryOperationResult(chainID, operation))
	}
	return results, nil
}

// RegistryParameters returns the canonical rules needed by an external
// signer. Consensus parameters are read from the local chain configuration;
// canonical state is derived from the current head.
func (api *LQCRegistryAPI) RegistryParameters() (RegistryParametersResult, error) {
	if api == nil || api.eth == nil || api.eth.blockchain == nil || api.eth.blockchain.Config() == nil || api.eth.blockchain.Config().ChainID == nil {
		return RegistryParametersResult{}, errLQCRegistryUnavailable
	}
	engine := api.eth.lqcRegistryEngine()
	if engine == nil {
		return RegistryParametersResult{}, errLQCRegistryUnavailable
	}
	status, err := engine.RegistryStatus(api.eth.blockchain)
	if err != nil {
		return RegistryParametersResult{}, err
	}
	return RegistryParametersResult{
		ChainID:              (*hexutil.Big)(new(big.Int).Set(api.eth.blockchain.Config().ChainID)),
		ActivationBlock:      hexutil.Uint64(status.ActivationBlock),
		CurrentBlock:         hexutil.Uint64(status.CurrentBlock),
		NextBlock:            hexutil.Uint64(status.NextBlock),
		ProofDifficulty:      hexutil.Uint64(status.ProofDifficulty),
		ActivationDelay:      hexutil.Uint64(status.ActivationDelay),
		HeartbeatWindow:      hexutil.Uint64(status.HeartbeatWindow),
		HeartbeatGrace:       hexutil.Uint64(status.HeartbeatGrace),
		MaxOperationLifetime: hexutil.Uint64(status.MaxOperationLifetime),
		PoolCapacity:         hexutil.Uint64(status.PoolCapacity),
		RegistryRoot:         status.RegistryRoot,
		ParticipantCount:     hexutil.Uint64(status.ParticipantCount),
		ActiveForNextBlock:   status.ActiveForNextBlock,
	}, nil
}

// RegistryParticipant returns the canonical state for an address. Pending
// relay-pool operations are intentionally excluded.
func (api *LQCRegistryAPI) RegistryParticipant(address common.Address) (RegistryParticipantResult, error) {
	if api == nil || api.eth == nil || api.eth.blockchain == nil {
		return RegistryParticipantResult{}, errLQCRegistryUnavailable
	}
	engine := api.eth.lqcRegistryEngine()
	if engine == nil {
		return RegistryParticipantResult{}, errLQCRegistryUnavailable
	}
	status, err := engine.RegistryParticipant(api.eth.blockchain, address)
	if err != nil {
		return RegistryParticipantResult{}, err
	}
	return RegistryParticipantResult{
		Address:        address,
		Exists:         status.Exists,
		Active:         status.Participant.Active,
		EligibleNext:   status.EligibleNext,
		CanonicalBlock: hexutil.Uint64(status.CanonicalBlock),
		RegistryRoot:   status.RegistryRoot,
		RegisteredAt:   hexutil.Uint64(status.Participant.RegisteredAt),
		LastHeartbeat:  hexutil.Uint64(status.Participant.LastHeartbeat),
		Sequence:       hexutil.Uint64(status.Participant.Sequence),
	}, nil
}

func (args RegistryOperationArgs) operation() lqc.RegistryOperation {
	return lqc.RegistryOperation{
		Version:    uint8(args.Version),
		Action:     lqc.RegistryAction(args.Action),
		Address:    args.Address,
		Sequence:   uint64(args.Sequence),
		ValidUntil: uint64(args.ValidUntil),
		ProofNonce: uint64(args.ProofNonce),
		Signature:  append([]byte(nil), args.Signature...),
	}
}

func registryOperationResult(chainID *big.Int, operation lqc.RegistryOperation) RegistryOperationResult {
	hash := lqc.RegistryOperationHash(chainID, operation)
	return RegistryOperationResult{
		Hash:       hash,
		Version:    hexutil.Uint64(operation.Version),
		Action:     hexutil.Uint64(operation.Action),
		Address:    operation.Address,
		Sequence:   hexutil.Uint64(operation.Sequence),
		ValidUntil: hexutil.Uint64(operation.ValidUntil),
		ProofNonce: hexutil.Uint64(operation.ProofNonce),
		Signature:  append(hexutil.Bytes(nil), operation.Signature...),
	}
}
