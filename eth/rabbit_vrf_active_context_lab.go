//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/log"
)

// Preparation remains currentContext(); signing resolves the ACTIVE block epoch.
func (runtime *rabbitVRFDKGRuntime) thresholdContextV1() rabbitVRFDKGLocalContextV1 {
	if runtime == nil {
		return rabbitVRFDKGLocalContextV1{}
	}
	// Isolated transport fixtures have no blockchain; their explicit context is authoritative.
	if runtime.backend == nil || runtime.backend.blockchain == nil {
		return runtime.currentContext()
	}
	empty := rabbitVRFDKGLocalContextV1{}
	if runtime.engine == nil {
		return empty
	}
	head := runtime.backend.blockchain.CurrentHeader()
	if head == nil || head.Number == nil || !head.Number.IsUint64() {
		return empty
	}
	bridge, exists, err := runtime.engine.RabbitVRFActiveBridgeContextV1(runtime.backend.blockchain, head.Number.Uint64(), head.Hash(), head.Number.Uint64()+1)
	if err != nil || !exists {
		log.Debug("Rabbit VRF active epoch context waiting", "err", err)
		return empty
	}
	local, err := rabbitVRFDKGMatchLocalMembersV1(rabbitVRFDKGLocalWalletAddressesV1(runtime.backend.accountManager), bridge.Members)
	if err != nil {
		return empty
	}
	state, err := runtime.engine.LoadRabbitVRFDKGTransportKeySetStateV1(bridge.SessionID)
	if err != nil {
		log.Debug("Rabbit VRF active epoch bindings waiting", "session", bridge.SessionID, "err", err)
		return empty
	}
	context, err := rabbitVRFActiveContextFromBridgeV1(bridge, state, local)
	if err != nil {
		log.Debug("Rabbit VRF active epoch bindings rejected", "err", err)
		return empty
	}
	context.HeadNumber = head.Number.Uint64()
	context.HeadHash = head.Hash()
	return context
}
func rabbitVRFActiveContextFromBridgeV1(bridge lqc.RabbitVRFDKGBridgeV1, state *lqc.RabbitVRFDKGTransportKeySetStateV1, local []lqc.RabbitVRFCommitteeMemberV1) (rabbitVRFDKGLocalContextV1, error) {
	empty := rabbitVRFDKGLocalContextV1{}
	sid, err := lqc.RabbitVRFDKGSessionIDV1(bridge.Session)
	if err != nil || sid != bridge.SessionID || state == nil || state.SessionID != sid || state.Root == (common.Hash{}) || len(state.Members) != len(bridge.Members) || len(state.Bindings) != len(bridge.Members) {
		return empty, errors.New("active VRF binding/session mismatch")
	}
	for i, m := range bridge.Members {
		if state.Members[i] != m || state.Bindings[i].ShareID != m.ShareID || state.Bindings[i].Participant != m.Participant {
			return empty, errors.New("active VRF committee mismatch")
		}
	}
	return rabbitVRFDKGLocalContextV1{SessionID: sid, SourceWorkEpoch: bridge.SourceWorkEpoch, PreparationEpoch: bridge.PreparationEpoch, TargetVRFEpoch: bridge.TargetVRFEpoch, CanonicalSession: bridge.Session, CanonicalMembers: cloneRabbitVRFDKGMembersV1(bridge.Members), Members: cloneRabbitVRFDKGMembersV1(local), CanonicalTransportKeySetRoot: state.Root, CanonicalTransportBindings: cloneRabbitVRFDKGTransportBindingsV1(state.Bindings), CanonicalTransportEnvelopes: cloneRabbitVRFDKGTransportEnvelopesV1(state.Envelopes)}, nil
}
