package eth

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func signedCommitteeClaimTransportFixture(
	t *testing.T,
) (lqc.CommitteeParticipationVerificationContextV1, lqc.CommitteeParticipationClaimGroupV1, common.Hash) {
	t.Helper()
	key, err := crypto.HexToECDSA(workV1TransportTestKey)
	if err != nil {
		t.Fatal(err)
	}
	seat := lqc.WorkSeatV1{
		Participant: crypto.PubkeyToAddress(key.PublicKey),
		TicketHash:  common.HexToHash("0x31"),
	}
	ctx := lqc.CommitteeParticipationVerificationContextV1{
		ChainID:       big.NewInt(928),
		DatasetKey:    common.HexToHash("0x32"),
		ParentHash:    common.HexToHash("0x33"),
		SelectionRoot: common.HexToHash("0x34"),
		Committee:     []lqc.WorkSeatV1{seat},
	}
	proofHash := common.HexToHash("0x35")
	proof := lqc.CommitteeParticipationV1{
		Version:       lqc.CommitteeParticipationVersionV1,
		BlockNumber:   100,
		ParentHash:    ctx.ParentHash,
		SelectionRoot: ctx.SelectionRoot,
		TicketHash:    seat.TicketHash,
		Participant:   seat.Participant,
	}
	signingHash, err := lqc.CommitteeParticipationSigningHashV1(
		ctx.ChainID, proof, proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(signingHash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, lqc.CommitteeParticipationClaimGroupV1{
		TargetBlock: 100,
		Participations: []lqc.CompactCommitteeParticipationV1{{
			Position: 0, Signature: signature,
		}},
	}, proofHash
}

func TestLQCCommitteeClaimTransportVerifiesRelaysAndDeduplicates(t *testing.T) {
	ctx, group, proofHash := signedCommitteeClaimTransportFixture(t)
	hashCalls := 0
	config := testWorkV1TransportConfig()
	config.CommitteeContext = func(target uint64) (lqc.CommitteeParticipationVerificationContextV1, error) {
		if target != group.TargetBlock {
			t.Fatalf("target=%d", target)
		}
		copy := ctx
		copy.Hasher = func(common.Hash, []byte) (common.Hash, error) {
			hashCalls++
			return proofHash, nil
		}
		return copy, nil
	}
	transport, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	if err := transport.SubmitCommitteeClaim(group); err != nil {
		t.Fatal(err)
	}
	if err := transport.SubmitCommitteeClaim(group); err != nil {
		t.Fatal(err)
	}
	if hashCalls != 1 {
		t.Fatalf("duplicate caused RandomX: calls=%d want=1", hashCalls)
	}
	if got := transport.pendingCommitteeClaims(101, nil); len(got) != 1 {
		t.Fatalf("pending=%+v", got)
	}

	left, right := p2p.MsgPipe()
	defer left.Close()
	defer right.Close()
	peer := &lqcWorkV1Peer{
		peer: p2p.NewPeer(enode.ID{9}, "committee-test", nil),
		rw:   left, known: make(map[common.Hash]struct{}),
	}
	done := make(chan error, 1)
	go func() { done <- peer.sendCommitteeClaims([]lqc.CommitteeParticipationClaimGroupV1{group}) }()
	message, err := right.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if message.Code != lqcWorkV1ClaimsMsg {
		t.Fatalf("message code=%d", message.Code)
	}
	var received []lqc.CommitteeParticipationClaimGroupV1
	if err := message.Decode(&received); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0].TargetBlock != group.TargetBlock ||
		!bytes.Equal(received[0].Participations[0].Signature, group.Participations[0].Signature) {
		t.Fatalf("received=%+v", received)
	}
}

func TestLQCCommitteeWalletSignDataMatchesConsensusVerification(t *testing.T) {
	ctx, _, proofHash := signedCommitteeClaimTransportFixture(t)
	store := keystore.NewKeyStore(t.TempDir(), keystore.LightScryptN, keystore.LightScryptP)
	account, err := store.NewAccount("rabbit-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Unlock(account, "rabbit-test"); err != nil {
		t.Fatal(err)
	}
	seat := lqc.WorkSeatV1{Participant: account.Address, TicketHash: common.HexToHash("0x41")}
	proof := lqc.CommitteeParticipationV1{
		Version: lqc.CommitteeParticipationVersionV1, BlockNumber: 101,
		ParentHash: ctx.ParentHash, SelectionRoot: ctx.SelectionRoot,
		TicketHash: seat.TicketHash, Participant: seat.Participant,
	}
	data, err := lqc.CommitteeParticipationSigningDataV1(ctx.ChainID, proof, proofHash)
	if err != nil {
		t.Fatal(err)
	}
	wallets := store.Wallets()
	if len(wallets) != 1 {
		t.Fatalf("wallets=%d want=1", len(wallets))
	}
	proof.Signature, err = wallets[0].SignData(account, accounts.MimetypeClique, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := lqc.VerifyCommitteeParticipationV1(ctx.ChainID, seat, proof, proofHash); err != nil {
		t.Fatalf("wallet signature rejected by consensus: %v", err)
	}
}
