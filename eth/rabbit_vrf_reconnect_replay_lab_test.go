//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func TestRabbitVRFReconnectReplaysOnlyLocalSharesV1(t *testing.T) {
	session := common.Hash{1}
	keyset := common.Hash{2}
	request := common.Hash{3}
	context := rabbitVRFDKGLocalContextV1{
		SessionID: session,
		Members:   []lqc.RabbitVRFCommitteeMemberV1{{ShareID: 1}},
	}
	transport := &rabbitVRFDKGTransport{
		partialSession:    session,
		partialKeysetRoot: keyset,
		partials: map[common.Hash]map[uint64]lqc.RabbitVRFThresholdPartialV1{
			request: {
				1: {SessionID: session, KeysetRoot: keyset, RequestID: request, ShareID: 1},
				2: {SessionID: session, KeysetRoot: keyset, RequestID: request, ShareID: 2},
			},
		},
	}
	selected := transport.pendingLocalThresholdPartialsV1(context)
	if len(selected) != 1 || selected[0].ShareID != 1 {
		t.Fatalf("replayed %v; expected only local share 1", selected)
	}
	context.SessionID = common.Hash{9}
	if got := transport.pendingLocalThresholdPartialsV1(context); len(got) != 0 {
		t.Fatalf("replayed stale session packets: %v", got)
	}
}
