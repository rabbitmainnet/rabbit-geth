//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
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

	participantKeys := make([]*ecdsa.PrivateKey, context.CommitteeSize)
	canonicalMembers := make([]lqc.RabbitVRFCommitteeMemberV1, context.CommitteeSize)
	participantKeys[0] = memberKey
	canonicalMembers[0] = member
	for index := 1; index < int(context.CommitteeSize); index++ {
		participantKey, err := gethcrypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		shareID := uint64(index + 1)
		participantKeys[index] = participantKey
		canonicalMembers[index] = lqc.RabbitVRFCommitteeMemberV1{
			ShareID:     shareID,
			TicketHash:  gethcrypto.Keccak256Hash([]byte{byte(shareID), 0x73}),
			Participant: gethcrypto.PubkeyToAddress(participantKey.PublicKey),
		}
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

	transportStore, err := rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "dkg-transport"),
	)
	if err != nil {
		t.Fatal(err)
	}
	p2pKey, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(p2pKey)

	canonicalBindings := make([]lqc.RabbitVRFDKGTransportKeyBindingV1, context.CommitteeSize)
	canonicalEnvelopes := make([]lqc.RabbitVRFDKGEnvelopeV1, context.CommitteeSize)
	for index, canonicalMember := range canonicalMembers {
		binding, err := rabbitVRFDKGLoadOrCreateTransportBindingV1(
			transportStore,
			context,
			canonicalMember,
			password,
			p2pKey,
		)
		if err != nil {
			t.Fatal(err)
		}
		bindingRoot, err := lqc.VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			canonicalMember,
			binding,
		)
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
			context,
			canonicalMember,
			lqc.RabbitVRFDKGMessageTransportKeyBindingV1,
			bindingRoot,
		)
		if err != nil {
			t.Fatal(err)
		}
		signingHash, err := lqc.RabbitVRFDKGEnvelopeSigningHashV1(context, envelope)
		if err != nil {
			t.Fatal(err)
		}
		envelope.Signature, err = gethcrypto.Sign(signingHash[:], participantKeys[index])
		if err != nil {
			t.Fatal(err)
		}
		if err := lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			canonicalMember,
			binding,
			envelope,
		); err != nil {
			t.Fatal(err)
		}
		canonicalBindings[index] = binding
		canonicalEnvelopes[index] = envelope
	}
	transportKeySetRoot, err := lqc.RabbitVRFDKGTransportKeySetRootV1(
		context,
		canonicalMembers,
		canonicalBindings,
		canonicalEnvelopes,
	)
	if err != nil {
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
				CanonicalMembers: append(
					[]lqc.RabbitVRFCommitteeMemberV1(nil),
					canonicalMembers...,
				),
				Members: []lqc.RabbitVRFCommitteeMemberV1{member},
				TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{
					canonicalBindings[0],
				},
				TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{
					canonicalEnvelopes[0],
				},
				CanonicalTransportKeySetRoot: transportKeySetRoot,
				CanonicalTransportBindings: append(
					[]lqc.RabbitVRFDKGTransportKeyBindingV1(nil),
					canonicalBindings...,
				),
				CanonicalTransportEnvelopes: append(
					[]lqc.RabbitVRFDKGEnvelopeV1(nil),
					canonicalEnvelopes...,
				),
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

		index := int(share.ID() - 1)
		remoteMember := canonicalMembers[index]
		remoteBinding := canonicalBindings[index]

		partialMessageID, err := lqc.RabbitVRFThresholdPartialMessageIDV1(packet)
		if err != nil {
			t.Fatal(err)
		}
		signingHash, err := lqc.RabbitVRFParticipationSigningHashV1(
			context,
			transportKeySetRoot,
			keysetRoot,
			requestID,
			packet.MessageHash,
			partialMessageID,
			remoteMember,
		)
		if err != nil {
			t.Fatal(err)
		}

		transportPrivateKey, persistedBinding, err := transportStore.Load(
			context,
			remoteMember,
			password,
		)
		if err != nil {
			t.Fatal(err)
		}
		if transportPrivateKey == nil {
			t.Fatal("nil remote rabbit vrf transport private key")
		}
		if persistedBinding != remoteBinding {
			zeroRabbitVRFDKGPrivateKeyV1(transportPrivateKey)
			t.Fatal("remote rabbit vrf transport binding mismatch")
		}

		participationSignature, err := gethcrypto.Sign(
			signingHash[:],
			transportPrivateKey,
		)
		zeroRabbitVRFDKGPrivateKeyV1(transportPrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		if len(participationSignature) != gethcrypto.SignatureLength {
			t.Fatal("invalid remote rabbit vrf participation signature size")
		}
		copy(
			packet.ParticipationSignature[:],
			participationSignature[:lqc.RabbitVRFParticipationSignatureSizeV1],
		)

		if err := lqc.ValidateRabbitVRFThresholdPartialParticipationV1(
			context,
			transportKeySetRoot,
			remoteMember,
			remoteBinding,
			packet,
		); err != nil {
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
