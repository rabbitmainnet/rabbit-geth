//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFThresholdLocalFlowV1RestartReuse(t *testing.T) {
	chainID := big.NewInt(9280)
	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		chainID,
		11,
		gethcrypto.Keccak256Hash([]byte("rabbit-vrf-local-flow-committee")),
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
	}

	memberKey, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID:     1,
		TicketHash:  gethcrypto.Keccak256Hash([]byte("rabbit-vrf-local-flow-ticket")),
		Participant: gethcrypto.PubkeyToAddress(memberKey.PublicKey),
	}

	makeShare := func(id uint64, value byte) *rabbitvrf.SecretShare {
		encoded := make([]byte, rabbitvrf.SecretKeySize)
		encoded[len(encoded)-1] = value
		share, err := rabbitvrf.SecretShareFromBytes(id, encoded)
		if err != nil {
			t.Fatal(err)
		}
		return share
	}

	shares := []*rabbitvrf.SecretShare{
		makeShare(1, 10),
		makeShare(2, 13),
		makeShare(3, 16),
	}

	verificationShares := make([]lqc.RabbitVRFVerificationShareV1, 3)
	for index, share := range shares {
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		verificationShares[index] = lqc.RabbitVRFVerificationShareV1{
			ShareID:   uint64(index + 1),
			PublicKey: publicKey,
		}
	}

	master := makeShare(99, 7)
	thresholdPublicKey, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	transcriptRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-local-flow-transcript"))
	keysetRoot, canonicalShares, err := lqc.RabbitVRFKeysetRootV1(
		chainID,
		11,
		context.CommitteeRoot,
		3,
		context.Threshold,
		thresholdPublicKey,
		transcriptRoot,
		verificationShares,
	)
	if err != nil {
		t.Fatal(err)
	}

	keyset := rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot:         keysetRoot,
		ThresholdPublicKey: thresholdPublicKey,
		TranscriptRoot:     transcriptRoot,
		VerificationShares: canonicalShares,
	}

	instanceDir := t.TempDir()

	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "dkg-final-keysets"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := keysetStore.Store(context, keyset); err != nil {
		t.Fatal(err)
	}

	passwordPath := filepath.Join(instanceDir, "dkg-password")
	const password = "rabbit-vrf-local-flow-password"
	if err := os.WriteFile(passwordPath, []byte(password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	shareStore, err := rabbitvrfstate.NewDKGSecretShareStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "dkg-secret-shares"),
		2,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := shareStore.Store(context, member, shares[0], password); err != nil {
		t.Fatal(err)
	}

	requestID := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-local-flow-request"))
	lookup := func(got common.Hash) (rabbitVRFCanonicalRequestV1, error) {
		if got != requestID {
			return rabbitVRFCanonicalRequestV1{}, errors.New("request not canonical")
		}
		return rabbitVRFCanonicalRequestV1{
			RequestID:    got,
			Requester:    common.Address{1},
			RequestBlock: 1,
			Status:       rabbitVRFRequestStatusPendingV1,
		}, nil
	}

	makeRuntime := func() *rabbitVRFDKGRuntime {
		return &rabbitVRFDKGRuntime{
			backend: &Ethereum{
				config: &ethconfig.Config{
					RabbitVRFDKGPasswordFile: passwordPath,
				},
				handler:           &handler{downloader: &downloader.Downloader{}},
				vrfDKGInstanceDir: instanceDir,
			},
			secretReady: true,
			current: rabbitVRFDKGLocalContextV1{
				SessionID:        sessionID,
				CanonicalSession: context,
				Members:          []lqc.RabbitVRFCommitteeMemberV1{member},
			},
			canonicalRequestLookup: lookup,
		}
	}

	runtime1 := makeRuntime()
	transport1, err := newRabbitVRFDKGTransport(rabbitVRFDKGTransportConfig{
		ChainID:   chainID,
		NetworkID: 9280,
		Genesis:   gethcrypto.Keccak256Hash([]byte("rabbit-vrf-local-flow-genesis")),
		Runtime:   runtime1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := transport1.processCanonicalPendingRequestV1(requestID); err != nil {
		t.Fatal(err)
	}

	partialStore, err := rabbitvrfstate.NewThresholdPartialStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "threshold-partials"),
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := partialStore.Load(context, keysetRoot, requestID, member.ShareID)
	if err != nil {
		t.Fatal(err)
	}

	transport1.mu.RLock()
	firstCollected := transport1.partials[requestID][member.ShareID]
	transport1.mu.RUnlock()

	if firstCollected != stored {
		t.Fatal("local partial was not collected canonically")
	}

	// Persist valid remote partials before restart. The restarted node must
	// recover them from disk together with its local partial.
	message, _, err := lqc.RabbitVRFThresholdMessageV1(context, keysetRoot, requestID)
	if err != nil {
		t.Fatal(err)
	}
	for _, share := range shares[1:] {
		partial, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(share, share.ID(), keyset, message)
		if err != nil {
			t.Fatal(err)
		}
		packet, err := lqc.NewRabbitVRFThresholdPartialV1(context, keysetRoot, requestID, partial)
		if err != nil {
			t.Fatal(err)
		}
		if err := partialStore.Store(context, packet); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(passwordPath); err != nil {
		t.Fatal(err)
	}

	runtime2 := makeRuntime()
	transport2, err := newRabbitVRFDKGTransport(rabbitVRFDKGTransportConfig{
		ChainID:   chainID,
		NetworkID: 9280,
		Genesis:   gethcrypto.Keccak256Hash([]byte("rabbit-vrf-local-flow-genesis")),
		Runtime:   runtime2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := transport2.processCanonicalPendingRequestV1(requestID); err != nil {
		t.Fatalf("restart failed to reuse persisted partial: %v", err)
	}

	transport2.mu.RLock()
	restartedCollected := transport2.partials[requestID][member.ShareID]
	transport2.mu.RUnlock()

	if restartedCollected != stored {
		t.Fatal("restart changed persisted threshold partial")
	}

	transport2.mu.RLock()
	_, thresholdRecovered := transport2.partialResults[requestID]
	restoredCount := len(transport2.partials[requestID])
	transport2.mu.RUnlock()
	if !thresholdRecovered {
		t.Fatalf("restart did not restore persisted remote partials: restored=%d want>=3", restoredCount)
	}
}
