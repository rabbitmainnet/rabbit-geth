package eth

import (
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestLQCCommitteeForkClaimKeepsWireAndRejectsInvalidV1(t *testing.T) {
	ctx, valid, proofHash := signedCommitteeClaimTransportFixture(t)
	ctx.Hasher = func(common.Hash, []byte) (common.Hash, error) { return proofHash, nil }
	// Sign with the same real private key but a different parent context.
	fork := ctx
	fork.ParentHash = common.HexToHash("0x9999")
	seat := ctx.Committee[0]
	proof := lqc.CommitteeParticipationV1{
		Version: lqc.CommitteeParticipationVersionV1, BlockNumber: valid.TargetBlock,
		ParentHash: fork.ParentHash, SelectionRoot: fork.SelectionRoot,
		TicketHash: seat.TicketHash, Participant: seat.Participant,
	}
	digest, err := lqc.CommitteeParticipationSigningHashV1(fork.ChainID, proof, proofHash)
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.HexToECDSA(workV1TransportTestKey)
	if err != nil {
		t.Fatal(err)
	}
	proof.Signature, err = crypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}
	if err := lqc.VerifyCommitteeParticipationV1(fork.ChainID, seat, proof, proofHash); err != nil {
		t.Fatal(err)
	}
	stale := lqc.CommitteeParticipationClaimGroupV1{TargetBlock: valid.TargetBlock,
		Participations: []lqc.CompactCommitteeParticipationV1{{Position: 0, Signature: proof.Signature}}}
	if _, err := lqc.VerifyCommitteeParticipationClaimGroupV1(ctx, stale); !errors.Is(err, lqc.ErrInvalidCommitteeSignatureV1) {
		t.Fatalf("fork signature must fail local consensus verification: %v", err)
	}
	config := testWorkV1TransportConfig()
	config.CommitteeContext = func(uint64) (lqc.CommitteeParticipationVerificationContextV1, error) { return ctx, nil }
	receiver, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	remote, err := newLQCWorkV1Transport(testWorkV1TransportConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	left, right := p2p.MsgPipe()
	defer left.Close()
	defer right.Close()
	done := make(chan error, 1)
	go func() { done <- receiver.runPeer(p2p.NewPeer(enode.ID{21}, "claims-remote", nil), left) }()
	outbound := &lqcWorkV1Peer{peer: p2p.NewPeer(enode.ID{22}, "claims-local", nil), rw: right, known: make(map[common.Hash]struct{})}
	if err := remote.handshake(outbound); err != nil {
		t.Fatal(err)
	}
	// An invalid signature must neither enter the pool nor kill the wire.
	if err := p2p.Send(right, lqcWorkV1ClaimsMsg, []lqc.CommitteeParticipationClaimGroupV1{stale}); err != nil {
		t.Fatal(err)
	}
	// A valid group on the same connection is the processing barrier.
	if err := p2p.Send(right, lqcWorkV1ClaimsMsg, []lqc.CommitteeParticipationClaimGroupV1{valid}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !receiver.claimPool.contains(valid) && time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("wire disconnected: %v", err)
		default:
		}
		time.Sleep(time.Millisecond)
	}
	if !receiver.claimPool.contains(valid) {
		t.Fatal("valid claim not accepted on surviving wire")
	}
	if receiver.claimPool.contains(stale) {
		t.Fatal("fork claim accepted")
	}
	// The same failure remains visible to a local producer, never rewarded.
	if err := receiver.SubmitCommitteeClaim(stale); !errors.Is(err, lqc.ErrInvalidCommitteeSignatureV1) {
		t.Fatalf("local failure hidden: %v", err)
	}
	// Malformed protocol packets are still fatal.
	if err := p2p.Send(right, lqcWorkV1ClaimsMsg, []lqc.CommitteeParticipationClaimGroupV1{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("empty packet accepted")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("malformed packet did not stop wire")
	}
}
