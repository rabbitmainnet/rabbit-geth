package eth

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const workV1TransportTestKey = "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"

func testWorkV1DatasetAnchor() common.Hash {
	return crypto.Keccak256Hash([]byte("rabbit-work-v1-transport-dataset-anchor"))
}

func testWorkV1ChallengeAnchor() common.Hash {
	return crypto.Keccak256Hash([]byte("rabbit-work-v1-transport-challenge-anchor"))
}

func testWorkV1Context() (lqcWorkV1Context, error) {
	return lqcWorkV1Context{
		Epoch:           7,
		DatasetAnchor:   testWorkV1DatasetAnchor(),
		ChallengeAnchor: testWorkV1ChallengeAnchor(),
		Difficulty:      big.NewInt(1),
		Eligibility: func(common.Address) error {
			return nil
		},
	}, nil
}

func testWorkV1TransportConfig() lqcWorkV1TransportConfig {
	return lqcWorkV1TransportConfig{
		Enabled:   true,
		ChainID:   big.NewInt(928),
		NetworkID: 928,
		Genesis: crypto.Keccak256Hash(
			[]byte("isolated-work-v1-lab-genesis"),
		),
		Context: testWorkV1Context,
		Hasher: func(
			common.Hash,
			[]byte,
		) (common.Hash, error) {
			return common.HexToHash("0x01"), nil
		},
	}
}

func signedWorkV1TransportCandidate(
	t *testing.T,
	proof common.Hash,
	nonce uint64,
) lqc.WorkCommitCandidateV1 {
	t.Helper()

	key, err := crypto.HexToECDSA(workV1TransportTestKey)
	if err != nil {
		t.Fatal(err)
	}

	ticket := lqc.RandomXWorkTicketV1{
		Version:     lqc.RandomXWorkProtocolVersion,
		Epoch:       7,
		Participant: crypto.PubkeyToAddress(key.PublicKey),
		Nonce:       nonce,
	}

	hash, err := lqc.RandomXWorkSigningHashV1(
		big.NewInt(928),
		testWorkV1ChallengeAnchor(),
		ticket,
		proof,
	)
	if err != nil {
		t.Fatal(err)
	}

	signature, err := crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatal(err)
	}

	return lqc.WorkCommitCandidateV1{
		Signed: lqc.SignedRandomXWorkTicketV1{
			Ticket:    ticket,
			Signature: signature,
		},
		ProofHash: proof,
	}
}

func TestLQCWorkV1TransportRequiresExplicitLabMode(t *testing.T) {
	config := testWorkV1TransportConfig()
	config.Enabled = false

	if _, err := newLQCWorkV1Transport(
		config,
	); !errors.Is(err, errLQCWorkV1TransportDisabled) {
		t.Fatalf("disabled error = %v", err)
	}

	config = testWorkV1TransportConfig()
	config.Hasher = nil
	if _, err := newLQCWorkV1Transport(
		config,
	); !errors.Is(err, errLQCWorkV1Context) {
		t.Fatalf("hasher error = %v", err)
	}
}

func TestLQCWorkV1TransportUsesDistinctBoundedProtocol(t *testing.T) {
	transport, err := newLQCWorkV1Transport(
		testWorkV1TransportConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	protocol := transport.Protocol()
	if protocol.Name != "lqcw" ||
		protocol.Version != 2 ||
		protocol.Length != 2 {
		t.Fatalf("protocol = %+v", protocol)
	}

	if MaxWorkV1CandidatesPerPacket != 8 ||
		lqcWorkV1MaxMessageSize != 8*1024 ||
		lqcWorkV1PeerBudget != 8 ||
		lqcWorkV1GlobalBudget != 16 ||
		lqcWorkV1MaxVerifyInFlight != 1 {
		t.Fatalf(
			"unsafe Work V1 bounds: packet=%d message=%d peer=%d global=%d inflight=%d",
			MaxWorkV1CandidatesPerPacket,
			lqcWorkV1MaxMessageSize,
			lqcWorkV1PeerBudget,
			lqcWorkV1GlobalBudget,
			lqcWorkV1MaxVerifyInFlight,
		)
	}
}

func TestLQCWorkV2ParticipantStatusReportsCanonicalSeat(t *testing.T) {
	participant := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	config := testWorkV1TransportConfig()
	config.SeatStatus = func(address common.Address) (
		uint64,
		uint64,
		bool,
		bool,
		error,
	) {
		if address != participant {
			return 0, 0, false, false, errors.New("unexpected participant")
		}
		return 7, 5000, true, false, nil
	}
	transport, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	status, err := newLQCWorkV1API(transport).
		WorkV2ParticipantStatus(participant)
	if err != nil {
		t.Fatal(err)
	}
	if status.Participant != participant ||
		uint64(status.SelectionEpoch) != 7 ||
		uint64(status.SeatCount) != 5000 ||
		!status.ActiveSeat ||
		status.Committed ||
		status.LocalPool {
		t.Fatalf("unexpected Work V2 participant status: %+v", status)
	}
}

func TestLQCWorkV1TransportHandshakeBindsVersionNetworkGenesisChain(
	t *testing.T,
) {
	tests := []struct {
		name   string
		change func(*lqcWorkV1TransportConfig)
		fail   bool
	}{
		{name: "matching"},
		{
			name: "network mismatch",
			fail: true,
			change: func(config *lqcWorkV1TransportConfig) {
				config.NetworkID++
			},
		},
		{
			name: "genesis mismatch",
			fail: true,
			change: func(config *lqcWorkV1TransportConfig) {
				config.Genesis[0] ^= 1
			},
		},
		{
			name: "chain mismatch",
			fail: true,
			change: func(config *lqcWorkV1TransportConfig) {
				config.ChainID = big.NewInt(929)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			leftConfig := testWorkV1TransportConfig()
			rightConfig := testWorkV1TransportConfig()
			if test.change != nil {
				test.change(&rightConfig)
			}

			leftNetwork, err := newLQCWorkV1Transport(leftConfig)
			if err != nil {
				t.Fatal(err)
			}
			rightNetwork, err := newLQCWorkV1Transport(rightConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer leftNetwork.Close()
			defer rightNetwork.Close()

			leftRW, rightRW := p2p.MsgPipe()
			defer leftRW.Close()
			defer rightRW.Close()

			leftPeer := &lqcWorkV1Peer{
				peer: p2p.NewPeer(
					enode.ID{2},
					"left",
					nil,
				),
				rw:    leftRW,
				known: make(map[common.Hash]struct{}),
			}
			rightPeer := &lqcWorkV1Peer{
				peer: p2p.NewPeer(
					enode.ID{3},
					"right",
					nil,
				),
				rw:    rightRW,
				known: make(map[common.Hash]struct{}),
			}

			results := make(chan error, 2)
			go func() {
				results <- leftNetwork.handshake(leftPeer)
			}()
			go func() {
				results <- rightNetwork.handshake(rightPeer)
			}()

			first, second := <-results, <-results
			if test.fail && (first == nil || second == nil) {
				t.Fatalf(
					"mismatch accepted: first=%v second=%v",
					first,
					second,
				)
			}
			if !test.fail && (first != nil || second != nil) {
				t.Fatalf(
					"matching handshake failed: first=%v second=%v",
					first,
					second,
				)
			}
		})
	}
}

func TestLQCWorkV1PeerSendsEachProofOnce(t *testing.T) {
	left, right := p2p.MsgPipe()
	defer left.Close()
	defer right.Close()

	peer := &lqcWorkV1Peer{
		peer: p2p.NewPeer(
			enode.ID{1},
			"work-v1-test",
			nil,
		),
		rw:    left,
		known: make(map[common.Hash]struct{}),
	}

	candidate := signedWorkV1TransportCandidate(
		t,
		common.HexToHash("0x01"),
		1,
	)

	done := make(chan error, 1)
	go func() {
		done <- peer.sendCandidates(
			[]lqc.WorkCommitCandidateV1{candidate},
		)
	}()

	message, err := right.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if message.Code != lqcWorkV1CandidatesMsg {
		t.Fatalf("message code = %d", message.Code)
	}

	var received []lqc.WorkCommitCandidateV1
	if err := message.Decode(&received); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 ||
		received[0].ProofHash != candidate.ProofHash {
		t.Fatalf("received = %+v", received)
	}

	if err := peer.sendCandidates(
		[]lqc.WorkCommitCandidateV1{candidate},
	); err != nil {
		t.Fatal(err)
	}
	if len(peer.known) != 1 {
		t.Fatalf("known = %d, want 1", len(peer.known))
	}
}

func TestLQCWorkV1TransportBudgetsAreTight(t *testing.T) {
	transport, err := newLQCWorkV1Transport(
		testWorkV1TransportConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	peer := &lqcWorkV1Peer{
		known: make(map[common.Hash]struct{}),
	}

	now := time.Unix(100, 0)

	for range lqcWorkV1PeerBudget {
		if !peer.consumeVerificationBudget(1, now) {
			t.Fatal("peer budget rejected allowed verification")
		}
	}
	if peer.consumeVerificationBudget(1, now) {
		t.Fatal("peer budget exceeded")
	}

	for range lqcWorkV1GlobalBudget {
		if !transport.consumeVerificationBudget(1, now) {
			t.Fatal("global budget rejected allowed verification")
		}
	}
	if transport.consumeVerificationBudget(1, now) {
		t.Fatal("global budget exceeded")
	}

	later := now.Add(lqcWorkV1BudgetWindow)
	if !peer.consumeVerificationBudget(1, later) ||
		!transport.consumeVerificationBudget(1, later) {
		t.Fatal("verification budgets did not reset")
	}
}

func TestLQCWorkV1TransportSubmitValidatesAndRetains(
	t *testing.T,
) {
	config := testWorkV1TransportConfig()

	proof := common.HexToHash("0x01")
	hashCalls := 0
	config.Hasher = func(
		epochKey common.Hash,
		input []byte,
	) (common.Hash, error) {
		hashCalls++
		if epochKey == (common.Hash{}) || len(input) == 0 {
			t.Fatal("invalid RandomX invocation")
		}
		return proof, nil
	}

	transport, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	candidate := signedWorkV1TransportCandidate(
		t,
		proof,
		1,
	)

	hash, err := transport.Submit(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if hash != proof {
		t.Fatalf("hash = %s, want %s", hash, proof)
	}
	if hashCalls != 1 {
		t.Fatalf("RandomX calls = %d, want 1", hashCalls)
	}

	status := transport.PoolStatus()
	if status.Epoch != 7 || status.Count != 1 {
		t.Fatalf("status = %+v", status)
	}

	if _, err := transport.Submit(candidate); !errors.Is(
		err,
		lqc.ErrWorkRelayAlreadyKnownV1,
	) {
		t.Fatalf("duplicate error = %v", err)
	}
	if hashCalls != 1 {
		t.Fatalf(
			"duplicate caused RandomX: calls=%d",
			hashCalls,
		)
	}
}

func TestLQCWorkV1TransportRejectsCanonicalReplayBeforeRandomX(
	t *testing.T,
) {
	config := testWorkV1TransportConfig()
	hashCalls := 0
	config.Hasher = func(
		common.Hash,
		[]byte,
	) (common.Hash, error) {
		hashCalls++
		return common.HexToHash("0x01"), nil
	}

	transport, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	candidate := signedWorkV1TransportCandidate(
		t,
		common.HexToHash("0x01"),
		91,
	)
	transport.included = func(
		got lqc.WorkCommitCandidateV1,
		epoch uint64,
	) bool {
		return epoch == 7 && got.ProofHash == candidate.ProofHash
	}

	if _, err := transport.Submit(candidate); !errors.Is(
		err,
		lqc.ErrWorkRelayAlreadyKnownV1,
	) {
		t.Fatalf("canonical replay error = %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("canonical replay caused RandomX: calls=%d", hashCalls)
	}
	if status := transport.PoolStatus(); status.Count != 0 {
		t.Fatalf("canonical replay entered pool: %+v", status)
	}
}

func TestLQCWorkV1TransportReconcilesPassivePoolBeforePending(
	t *testing.T,
) {
	transport, err := newLQCWorkV1Transport(
		testWorkV1TransportConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	// Initialize the persistent pool with the active commit epoch before
	// simulating a stale ticket restored by a passive node.
	if _, err := transport.currentContextRaw(); err != nil {
		t.Fatal(err)
	}

	candidate := signedWorkV1TransportCandidate(
		t,
		common.HexToHash("0x01"),
		92,
	)
	if err := transport.pool.AddVerifiedV1(candidate); err != nil {
		t.Fatal(err)
	}

	reconcileCalls := 0
	transport.reconcile = func(epoch uint64) error {
		reconcileCalls++
		if epoch != 7 {
			t.Fatalf("reconcile epoch=%d want=7", epoch)
		}
		if removed := transport.pool.RemoveIncludedV1(
			[]common.Hash{candidate.ProofHash},
		); removed != 1 {
			t.Fatalf("removed=%d want=1", removed)
		}
		return nil
	}

	pending, err := transport.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if reconcileCalls != 1 {
		t.Fatalf("reconcile calls=%d want=1", reconcileCalls)
	}
	if len(pending) != 0 || transport.PoolStatus().Count != 0 {
		t.Fatalf(
			"passive stale pool survived: pending=%d status=%+v",
			len(pending),
			transport.PoolStatus(),
		)
	}
}

func TestLQCWorkV1TransportRejectsFakeClaimAfterOneHash(
	t *testing.T,
) {
	config := testWorkV1TransportConfig()
	config.Hasher = func(
		common.Hash,
		[]byte,
	) (common.Hash, error) {
		return common.HexToHash("0x02"), nil
	}

	transport, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	candidate := signedWorkV1TransportCandidate(
		t,
		common.HexToHash("0x01"),
		1,
	)

	if _, err := transport.Submit(candidate); !errors.Is(
		err,
		lqc.ErrWorkRelayClaimMismatchV1,
	) {
		t.Fatalf("fake claim error = %v", err)
	}
	if transport.PoolStatus().Count != 0 {
		t.Fatal("fake claim entered Work V1 pool")
	}
}

func TestLQCWorkV1TransportEpochChangeResetsRelayPool(
	t *testing.T,
) {
	epoch := uint64(7)
	config := testWorkV1TransportConfig()
	config.Context = func() (lqcWorkV1Context, error) {
		return lqcWorkV1Context{
			Epoch:           epoch,
			DatasetAnchor:   testWorkV1DatasetAnchor(),
			ChallengeAnchor: testWorkV1ChallengeAnchor(),
			Difficulty:      big.NewInt(1),
			Eligibility: func(common.Address) error {
				return nil
			},
		}, nil
	}

	proof := common.HexToHash("0x01")
	config.Hasher = func(
		common.Hash,
		[]byte,
	) (common.Hash, error) {
		return proof, nil
	}

	transport, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	candidate := signedWorkV1TransportCandidate(
		t,
		proof,
		1,
	)
	if _, err := transport.Submit(candidate); err != nil {
		t.Fatal(err)
	}
	if transport.PoolStatus().Count != 1 {
		t.Fatal("candidate missing before epoch change")
	}

	epoch = 8
	if _, err := transport.currentContext(); err != nil {
		t.Fatal(err)
	}

	status := transport.PoolStatus()
	if status.Epoch != 8 || status.Count != 0 {
		t.Fatalf("status after epoch change = %+v", status)
	}
}
