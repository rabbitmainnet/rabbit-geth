//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

// The projection consumes public bindings already validated by the engine loader.
// Existing adversarial tests separately enforce certificate and BLS validation.
func TestRabbitVRFActiveContextSeparatedFromPreparationV1(t *testing.T) {
	session, members, _, cert := rabbitVRFAdversarialCertificateV1(t)
	bridge := lqc.RabbitVRFDKGBridgeV1{Session: session, SessionID: cert.SessionID, Members: members, TargetVRFEpoch: session.TargetVRFEpoch}
	state := &lqc.RabbitVRFDKGTransportKeySetStateV1{SessionID: cert.SessionID, Root: crypto.Keccak256Hash([]byte("validated-binding-state")), Members: append([]lqc.RabbitVRFCommitteeMemberV1(nil), members...)}
	for _, m := range members {
		state.Bindings = append(state.Bindings, lqc.RabbitVRFDKGTransportKeyBindingV1{SessionID: cert.SessionID, ShareID: m.ShareID, Participant: m.Participant})
	}
	prep := rabbitVRFDKGLocalContextV1{SessionID: crypto.Keccak256Hash([]byte("next-preparation"))}
	runtime := &rabbitVRFDKGRuntime{current: prep}
	active, err := rabbitVRFActiveContextFromBridgeV1(bridge, state, members[:1])
	if err != nil {
		t.Fatal(err)
	}
	if active.SessionID != cert.SessionID || active.CanonicalSession.TargetVRFEpoch != session.TargetVRFEpoch || active.SessionID == prep.SessionID {
		t.Fatal("selected preparation instead of active epoch")
	}
	if runtime.currentContext().SessionID != prep.SessionID {
		t.Fatal("preparation runtime was mutated")
	}
	if len(active.Members) != 1 || active.Members[0] != members[0] {
		t.Fatal("local active committee mismatch")
	}
	active.CanonicalMembers[0].Participant = common.Address{}
	if state.Members[0].Participant == (common.Address{}) {
		t.Fatal("active context aliases persisted members")
	}
	crossed := *state
	crossed.SessionID = prep.SessionID
	if _, err := rabbitVRFActiveContextFromBridgeV1(bridge, &crossed, members[:1]); err == nil {
		t.Fatal("cross-session binding accepted")
	}
	crossed = *state
	crossed.Bindings = append([]lqc.RabbitVRFDKGTransportKeyBindingV1(nil), state.Bindings...)
	crossed.Bindings[0].Participant = common.Address{}
	if _, err := rabbitVRFActiveContextFromBridgeV1(bridge, &crossed, members[:1]); err == nil {
		t.Fatal("cross-committee binding accepted")
	}
	if _, err := rabbitVRFActiveContextFromBridgeV1(bridge, nil, nil); err == nil {
		t.Fatal("missing active binding accepted")
	}
}
