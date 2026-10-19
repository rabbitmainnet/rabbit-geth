//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func TestRabbitVRFReadyPendingRequestBlockedByMissingKeyset(t *testing.T) {
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11, common.HexToHash("0x1234"), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}
	requestID := common.HexToHash("0x20ae")
	lookedUp := false
	runtime := &rabbitVRFDKGRuntime{
		backend:     &Ethereum{vrfDKGInstanceDir: t.TempDir()},
		secretReady: true,
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: session,
			Members: []lqc.RabbitVRFCommitteeMemberV1{
				{ShareID: 1, Participant: common.HexToAddress("0x1001")},
			},
		},
		canonicalRequestLookup: func(id common.Hash) (
			rabbitVRFCanonicalRequestV1, error,
		) {
			if id != requestID {
				t.Fatal("unexpected request")
			}
			lookedUp = true
			return rabbitVRFCanonicalRequestV1{
				RequestID:    id,
				Requester:    common.HexToAddress("0x2001"),
				RequestBlock: 100,
				Status:       rabbitVRFRequestStatusPendingV1,
			}, nil
		},
	}
	if !runtime.secretReadyV1() {
		t.Fatal("fixture is not ready")
	}
	transport := &rabbitVRFDKGTransport{runtime: runtime}
	err = transport.processCanonicalPendingRequestV1(requestID)
	if !lookedUp {
		t.Fatal("pending request lookup was not reached")
	}
	if err == nil || !strings.Contains(
		err.Error(), "load rabbit vrf dkg final keyset",
	) {
		t.Fatalf("expected missing-keyset gate, got %v", err)
	}
	t.Logf("BLOCK REPRODUCED despite secretReady=true: %v", err)
}
