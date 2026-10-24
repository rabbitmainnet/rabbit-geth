//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

// Pairing verification is bounded independently of the server's RPC concurrency.
var rabbitVRFPublicProofSlotsV1 = make(chan struct{}, 4)

// RabbitVRFPublicProofResultV1 is an early public proof, not an EVM receipt or a
// finality attestation. Consumers must track its canonical head/request hashes.
// The RLP fields contain exclusively public consensus artifacts, never shares,
// passwords, transport private keys, or the local signing-unlock cache.
type RabbitVRFPublicProofResultV1 struct {
	Version            uint8          `json:"version"`
	Status             string         `json:"status"`
	ChainID            *hexutil.Big   `json:"chainId"`
	GenesisHash        common.Hash    `json:"genesisHash"`
	RequestID          common.Hash    `json:"requestId"`
	HeadNumber         hexutil.Uint64 `json:"headNumber"`
	HeadHash           common.Hash    `json:"headHash"`
	RequestBlock       hexutil.Uint64 `json:"requestBlock"`
	RequestBlockHash   common.Hash    `json:"requestBlockHash"`
	CanonicalAtHead    bool           `json:"canonicalAtHead"`
	OnChainConfirmed   bool           `json:"onChainConfirmed"`
	ConsensusFinalized bool           `json:"consensusFinalized"`
	SessionID          common.Hash    `json:"sessionId"`
	Randomness         common.Hash    `json:"randomness"`
	ProofHash          common.Hash    `json:"proofHash"`
	HeadHeaderRLP      hexutil.Bytes  `json:"headHeaderRlp"`
	RequestHeaderRLP   hexutil.Bytes  `json:"requestHeaderRlp"`
	SessionRLP         hexutil.Bytes  `json:"sessionRlp,omitempty"`
	MembersRLP         hexutil.Bytes  `json:"membersRlp,omitempty"`
	CertificateRLP     hexutil.Bytes  `json:"certificateRlp,omitempty"`
	FinalizationRLP    hexutil.Bytes  `json:"finalizationRlp,omitempty"`
}

type rabbitVRFPublicProofChainV1 interface {
	CurrentBlock() *types.Header
	GetHeaderByNumber(uint64) *types.Header
	Genesis() *types.Block
	Config() *params.ChainConfig
}

type rabbitVRFPublicProofQueryV1 struct {
	chain       rabbitVRFPublicProofChainV1
	syncing     func() bool
	request     func(common.Hash) (rabbitVRFCanonicalRequestV1, error)
	bridge      func(uint64) (lqc.RabbitVRFDKGBridgeV1, uint64, bool, error)
	certificate func(lqc.RabbitVRFDKGSessionContextV1, []lqc.RabbitVRFCommitteeMemberV1) (lqc.RabbitVRFKeysetCertificateV1, error)
	proof       func(common.Hash, common.Hash) (lqc.RabbitVRFFinalizationV1, bool)
}

// RabbitVRFProof is exposed as lqc_rabbitVRFProof. It only reads proofs already
// produced by the canonical request flow. It cannot create or sign a request.
func (api *LQCRegistryAPI) RabbitVRFProof(ctx context.Context, requestID common.Hash) (*RabbitVRFPublicProofResultV1, error) {
	if api == nil || api.eth == nil || api.eth.blockchain == nil || api.eth.vrfDKGTransport == nil {
		return nil, errors.New("rabbit vrf public proof runtime unavailable")
	}
	n := api.eth.vrfDKGTransport
	if n.runtime == nil {
		return nil, errors.New("rabbit vrf public proof runtime unavailable")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case rabbitVRFPublicProofSlotsV1 <- struct{}{}:
		defer func() { <-rabbitVRFPublicProofSlotsV1 }()
	default:
		return nil, errors.New("rabbit vrf public proof query busy; retry")
	}
	query := rabbitVRFPublicProofQueryV1{
		chain: api.eth.blockchain,
		syncing: func() bool {
			n.runtime.mu.RLock()
			syncing := n.runtime.syncing
			n.runtime.mu.RUnlock()
			return syncing || !api.eth.Synced()
		},
		request: n.runtime.canonicalPendingRequestV1,
		bridge:  n.rabbitVRFServingBridgeForBlockV1,
		certificate: func(session lqc.RabbitVRFDKGSessionContextV1, members []lqc.RabbitVRFCommitteeMemberV1) (lqc.RabbitVRFKeysetCertificateV1, error) {
			store, err := n.keysetCertificateStoreV1()
			if err != nil {
				return lqc.RabbitVRFKeysetCertificateV1{}, err
			}
			return store.Load(session, members)
		},
		proof: func(sessionID, id common.Hash) (lqc.RabbitVRFFinalizationV1, bool) {
			n.mu.RLock()
			defer n.mu.RUnlock()
			if n.closed {
				return lqc.RabbitVRFFinalizationV1{}, false
			}
			proof, found := n.finalizations[rabbitVRFFinalizationKeyV1{SessionID: sessionID, RequestID: id}]
			return proof, found
		},
	}
	return query.read(ctx, requestID)
}

func (q rabbitVRFPublicProofQueryV1) read(ctx context.Context, requestID common.Hash) (*RabbitVRFPublicProofResultV1, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if requestID == (common.Hash{}) || q.chain == nil || q.syncing() {
		return nil, errors.New("rabbit vrf public proof query unavailable or syncing")
	}
	head := q.chain.CurrentBlock()
	config := q.chain.Config()
	genesis := q.chain.Genesis()
	if head == nil || head.Number == nil || !head.Number.IsUint64() || head.Number.Uint64() == ^uint64(0) || config == nil || config.ChainID == nil || config.LQC == nil || genesis == nil {
		return nil, errors.New("rabbit vrf public proof canonical head unavailable")
	}
	headHash := head.Hash()
	request, err := q.request(requestID)
	if err != nil {
		return nil, fmt.Errorf("rabbit vrf public proof pending request: %w", err)
	}
	if request.RequestID != requestID || request.Status != rabbitVRFRequestStatusPendingV1 || request.Requester == (common.Address{}) || request.RequestBlock == 0 || request.RequestBlock > head.Number.Uint64() || request.Randomness != (common.Hash{}) || request.ProofHash != (common.Hash{}) {
		return nil, errors.New("rabbit vrf public proof requires an existing canonical pending request")
	}
	requestHeader := q.chain.GetHeaderByNumber(request.RequestBlock)
	if requestHeader == nil || requestHeader.Number == nil || requestHeader.Number.Uint64() != request.RequestBlock {
		return nil, errors.New("rabbit vrf public proof request header unavailable")
	}
	result := &RabbitVRFPublicProofResultV1{
		Version: 1, Status: "pending", RequestID: requestID,
		ChainID: (*hexutil.Big)(new(big.Int).Set(config.ChainID)), GenesisHash: genesis.Hash(),
		HeadNumber: hexutil.Uint64(head.Number.Uint64()), HeadHash: headHash,
		RequestBlock: hexutil.Uint64(request.RequestBlock), RequestBlockHash: requestHeader.Hash(),
	}
	result.HeadHeaderRLP, err = rlp.EncodeToBytes(head)
	if err != nil {
		return nil, err
	}
	result.RequestHeaderRLP, err = rlp.EncodeToBytes(requestHeader)
	if err != nil {
		return nil, err
	}
	bridge, epochLength, exists, err := q.bridge(head.Number.Uint64() + 1)
	if err != nil {
		return nil, err
	}
	if exists {
		sessionID, err := lqc.RabbitVRFDKGSessionIDV1(bridge.Session)
		epoch, epochErr := lqc.RabbitVRFEpochForBlockV1(head.Number.Uint64()+1, epochLength)
		if err != nil || epochErr != nil || bridge.Session.ChainID == nil || bridge.Session.ChainID.Cmp(config.ChainID) != 0 || sessionID != bridge.SessionID || bridge.TargetVRFEpoch != epoch || bridge.Session.TargetVRFEpoch != epoch {
			return nil, errors.New("rabbit vrf public proof active session mismatch")
		}
		if proof, found := q.proof(sessionID, requestID); found {
			certificate, err := q.certificate(bridge.Session, bridge.Members)
			if err != nil {
				return nil, fmt.Errorf("rabbit vrf public proof certificate: %w", err)
			}
			certificate, err = lqc.ValidateRabbitVRFKeysetCertificateV1(bridge.Session, bridge.Members, certificate)
			if err != nil {
				return nil, fmt.Errorf("rabbit vrf public proof certificate verification: %w", err)
			}
			if proof.RequestID != requestID || proof.Round != request.RequestBlock {
				return nil, errors.New("rabbit vrf public proof request/round mismatch")
			}
			if err := rabbitVRFValidateFinalizationMemoV1(bridge.Session, bridge.Members, certificate, proof); err != nil {
				return nil, fmt.Errorf("rabbit vrf public proof verification: %w", err)
			}
			values := []struct {
				value interface{}
				out   *hexutil.Bytes
			}{
				{bridge.Session, &result.SessionRLP}, {bridge.Members, &result.MembersRLP},
				{certificate, &result.CertificateRLP}, {proof, &result.FinalizationRLP},
			}
			for _, value := range values {
				encoded, err := rlp.EncodeToBytes(value.value)
				if err != nil {
					return nil, err
				}
				*value.out = encoded
			}
			result.Status, result.SessionID = "proof_ready_unsettled", sessionID
			result.Randomness, result.ProofHash = proof.Randomness, proof.ProofHash
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current := q.chain.CurrentBlock()
	requestNow := q.chain.GetHeaderByNumber(request.RequestBlock)
	if q.syncing() || current == nil || current.Hash() != headHash || requestNow == nil || requestNow.Hash() != result.RequestBlockHash {
		return nil, errors.New("rabbit vrf public proof canonical head changed; retry")
	}
	result.CanonicalAtHead = true
	// OnChainConfirmed and ConsensusFinalized deliberately remain false.
	return result, nil
}
