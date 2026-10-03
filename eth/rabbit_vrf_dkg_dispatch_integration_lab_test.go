//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
)

type rabbitVRFDKGEvaluationMemoryWireV1 struct {
	recipient *rabbitVRFDKGRuntime
	delivered int
}

func (w *rabbitVRFDKGEvaluationMemoryWireV1) ReadMsg() (p2p.Msg, error) {
	return p2p.Msg{}, io.EOF
}

func (w *rabbitVRFDKGEvaluationMemoryWireV1) WriteMsg(msg p2p.Msg) error {
	defer msg.Discard()
	switch msg.Code {
	case rabbitVRFDKGEncryptedEvaluationMsg:
		var packet lqc.RabbitVRFDKGEncryptedEvaluationV1
		if err := msg.Decode(&packet); err != nil {
			return err
		}
		if err := w.recipient.validateInboundEncryptedEvaluationV1(packet); err != nil {
			return err
		}
		if _, err := w.recipient.decryptInboundEncryptedEvaluationV1(packet); err != nil {
			return err
		}
		w.delivered++
		return nil
	case rabbitVRFDKGThresholdPartialMsg:
		var packet lqc.RabbitVRFThresholdPartialV1
		if err := msg.Decode(&packet); err != nil {
			return err
		}
		return w.recipient.backend.vrfDKGTransport.collectThresholdPartialV1(packet)
	case rabbitVRFDKGKeysetCertificateMsg:
		var envelope lqc.RabbitVRFDKGEnvelopeV1
		if err := msg.Decode(&envelope); err != nil {
			return err
		}
		_, err := w.recipient.backend.vrfDKGTransport.collectKeysetCertificateEnvelopeV1(envelope)
		return err

	default:
		return fmt.Errorf("unexpected message code %d", msg.Code)
	}
}

func TestRabbitVRFDKGDispatchThreeRuntimesMissingRouteThenSign(t *testing.T) {
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11, common.HexToHash("0xaaaa"), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}
	const password = "integration-test-only"
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 3)
	bindings := make([]lqc.RabbitVRFDKGTransportKeyBindingV1, 3)
	envelopes := make([]lqc.RabbitVRFDKGEnvelopeV1, 3)
	commitments := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, 3)
	runtimes := make([]*rabbitVRFDKGRuntime, 3)

	for i := range runtimes {
		dir := t.TempDir()
		wallet := keystore.NewKeyStore(
			filepath.Join(dir, "test-wallet"),
			keystore.LightScryptN, keystore.LightScryptP,
		)
		account, err := wallet.NewAccount(password)
		if err != nil {
			t.Fatal(err)
		}
		if err := wallet.Unlock(account, password); err != nil {
			t.Fatal(err)
		}
		manager := accounts.NewManager(&accounts.Config{}, wallet)
		t.Cleanup(func() { manager.Close() })
		members[i] = lqc.RabbitVRFCommitteeMemberV1{
			ShareID:     uint64(i + 1),
			TicketHash:  common.BigToHash(big.NewInt(int64(i + 1))),
			Participant: account.Address,
		}
		passwordFile := filepath.Join(dir, "test-password")
		if err := os.WriteFile(passwordFile, []byte(password), 0600); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(dir, "rabbit-vrf")
		transportStore, err := rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(filepath.Join(root, "dkg-transport"))
		if err != nil {
			t.Fatal(err)
		}
		privateKey, binding, err := transportStore.Create(session, members[i], password)
		if err != nil {
			t.Fatal(err)
		}
		zeroRabbitVRFDKGPrivateKeyV1(privateKey)
		bindings[i] = binding
		envelopes[i], err = rabbitVRFDKGSignTransportBindingEnvelopeV1(
			wallet.Wallets(), session, members[i], binding,
		)
		if err != nil {
			t.Fatal(err)
		}
		polynomialStore, err := rabbitvrfstate.NewStandardDKGDealerPolynomialStoreV1(filepath.Join(root, "dkg-dealer-polynomial"))
		if err != nil {
			t.Fatal(err)
		}
		polynomial, commitment, _, err := polynomialStore.Create(
			session, members[i], password,
		)
		if err != nil {
			t.Fatal(err)
		}
		polynomial.Destroy()
		commitments[i] = commitment
		runtimes[i] = &rabbitVRFDKGRuntime{
			backend: &Ethereum{
				vrfDKGInstanceDir: dir,
				config:            &ethconfig.Config{RabbitVRFDKGPasswordFile: passwordFile},
				accountManager:    manager,
				handler:           &handler{downloader: &downloader.Downloader{}},
			},
			secretReady: true,
		}
	}
	transportRoot, err := lqc.RabbitVRFDKGTransportKeySetRootV1(
		session, members, bindings, envelopes,
	)
	if err != nil {
		t.Fatal(err)
	}
	for i, runtime := range runtimes {
		runtime.current = rabbitVRFDKGLocalContextV1{
			SessionID: id, CanonicalSession: session,
			Members: members[i : i+1], CanonicalMembers: members,
			PolynomialCommitments:        commitments,
			CanonicalTransportKeySetRoot: transportRoot,
			CanonicalTransportBindings:   bindings,
			CanonicalTransportEnvelopes:  envelopes,
		}
		runtime.backend.vrfDKGTransport = &rabbitVRFDKGTransport{
			runtime:      runtime,
			routeSession: id,
			routes:       make(map[uint64]string),
			peers:        make(map[string]*rabbitVRFDKGPeer),
		}
	}
	wires := make([]*rabbitVRFDKGEvaluationMemoryWireV1, 0, 6)
	for i, runtime := range runtimes {
		transport := runtime.backend.vrfDKGTransport
		for j, recipient := range runtimes {
			if i == j {
				continue
			}
			name := fmt.Sprintf("test-node-%d", j)
			wire := &rabbitVRFDKGEvaluationMemoryWireV1{recipient: recipient}
			wires = append(wires, wire)
			transport.peers[name] = &rabbitVRFDKGPeer{rw: wire}
			// Routes are fixture inputs; peer-route authentication has separate tests.
			transport.routes[members[j].ShareID] = name
		}
	}

	first := runtimes[0]
	first.secretReady = false
	if err := first.dispatchLocalEvaluationsV1(); err != nil {
		t.Fatal(err)
	}
	if len(first.evaluationDispatch.entries) != 0 {
		t.Fatal("evaluations generated while secret runtime was unavailable")
	}
	first.secretReady = true
	delete(first.backend.vrfDKGTransport.routes, members[2].ShareID)

	for _, runtime := range runtimes {
		if err := runtime.dispatchLocalEvaluationsV1(); err != nil {
			t.Fatal(err)
		}
		if ready, err := runtime.ensureFinalKeysetV1(); !ready || err != nil {
			t.Fatalf("keyset: ready=%v err=%v", ready, err)
		}
	}
	if ready, err := runtimes[2].ensureLocalSecretSharesV1(); ready || err != nil {
		t.Fatalf("missing dealer evaluation: ready=%v err=%v", ready, err)
	}
	first.backend.vrfDKGTransport.routes[members[2].ShareID] = "test-node-2"
	if err := first.dispatchLocalEvaluationsV1(); err != nil {
		t.Fatal(err)
	}

	for i, runtime := range runtimes {
		if ready, err := runtime.ensureLocalSecretSharesV1(); !ready || err != nil {
			t.Fatalf("node %d share: ready=%v err=%v", i, ready, err)
		}
		keysets, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
			filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"),
		)
		if err != nil {
			t.Fatal(err)
		}
		keyset, err := keysets.Load(session)
		if err != nil {
			t.Fatal(err)
		}
		message, _, err := lqc.RabbitVRFThresholdMessageV1(
			session, keyset.KeysetRoot, common.HexToHash("0x1234"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := runtime.signLocalThresholdPartialV1(members[i], message); err != nil {
			t.Fatalf("node %d cannot sign after real evaluation dispatch: %v", i, err)
		}
	}
	delivered := 0
	for _, wire := range wires {
		delivered += wire.delivered
	}
	if delivered != 6 {
		t.Fatalf("remote evaluations=%d want=6", delivered)
	}
	// Isolate an immediate retry from the expensive cryptographic checks above.
	for _, runtime := range runtimes {
		for _, entry := range runtime.evaluationDispatch.entries {
			if entry.peer != nil {
				if entry.lastSent.IsZero() {
					t.Fatal("successful remote send did not record its timestamp")
				}
				entry.lastSent = time.Now()
			}
		}
	}
	for _, runtime := range runtimes {
		if err := runtime.dispatchLocalEvaluationsV1(); err != nil {
			t.Fatal(err)
		}
	}
	repeated := 0
	for _, wire := range wires {
		repeated += wire.delivered
	}
	if repeated != delivered {
		t.Fatal("immediate tick unnecessarily resent encrypted evaluations")
	}

	requestID := common.HexToHash("0x1234")
	for _, runtime := range runtimes {
		runtime.canonicalRequestLookup = func(got common.Hash) (rabbitVRFCanonicalRequestV1, error) {
			if got != requestID {
				return rabbitVRFCanonicalRequestV1{}, fmt.Errorf("unexpected request")
			}
			return rabbitVRFCanonicalRequestV1{
				RequestID:    got,
				Requester:    common.HexToAddress("0x2001"),
				RequestBlock: 100,
				Status:       rabbitVRFRequestStatusPendingV1,
			}, nil
		}
	}
	for i, runtime := range runtimes {
		if err := runtime.backend.vrfDKGTransport.processCanonicalPendingRequestV1(requestID); err != nil {
			t.Fatalf("node %d pending request flow: %v", i, err)
		}
	}

	for i, runtime := range runtimes {
		if err := runtime.backend.vrfDKGTransport.publishLocalKeysetCertificateSignaturesV1(); err != nil {
			t.Fatalf("node %d certificate publication: %v", i, err)
		}
	}
	var expectedRandomness common.Hash
	for i, runtime := range runtimes {
		finalizations, err := runtime.backend.vrfDKGTransport.rabbitVRFFinalizationsForBlockV1(101)
		if err != nil || len(finalizations) != 1 {
			t.Fatalf("node %d finalizations=%d err=%v", i, len(finalizations), err)
		}
		value := finalizations[0]
		certificate, ready, err := runtime.backend.vrfDKGTransport.rabbitVRFKeysetCertificateForBlockV1(101)
		if err != nil || !ready {
			t.Fatalf("node %d certificate ready=%v err=%v", i, ready, err)
		}
		if _, err := lqc.ValidateRabbitVRFKeysetCertificateV1(
			session, members, certificate,
		); err != nil {
			t.Fatalf("node %d invalid consensus certificate: %v", i, err)
		}
		if err := lqc.ValidateRabbitVRFFinalizationProofV1(
			session, members, certificate, value,
		); err != nil {
			t.Fatalf("node %d invalid consensus finalization proof: %v", i, err)
		}

		if value.RequestID != requestID || value.Round != 100 ||
			value.Epoch != session.TargetVRFEpoch {
			t.Fatal("finalization lost canonical request bindings")
		}
		store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
			filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"),
		)
		if err != nil {
			t.Fatal(err)
		}
		keyset, err := store.Load(session)
		if err != nil {
			t.Fatal(err)
		}
		message, _, err := lqc.RabbitVRFThresholdMessageV1(
			session, keyset.KeysetRoot, requestID,
		)
		if err != nil {
			t.Fatal(err)
		}
		randomness, err := rabbitvrf.VerifyAndDeriveRandomness(
			keyset.ThresholdPublicKey, message, value.Signature,
		)
		if err != nil || randomness != value.Randomness {
			t.Fatalf("node %d invalid final randomness: %v", i, err)
		}
		ids, err := lqc.RabbitVRFCompactParticipationShareIDsFromFixedBitmapV1(
			session, value.ParticipationBitmap,
		)
		if err != nil || uint64(len(ids)) != session.Threshold {
			t.Fatalf("node %d invalid participation bitmap: %v", i, err)
		}
		if i == 0 {
			expectedRandomness = randomness
		} else if randomness != expectedRandomness {
			t.Fatal("nodes derived different randomness for the same request")
		}
	}
	t.Log("CONSENSUS PROOF VERIFIED: three runtimes produced valid keyset certificates and finalization proofs")

}
