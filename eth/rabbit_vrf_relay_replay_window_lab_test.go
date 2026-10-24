//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

// Re-encryption uses fresh ECIES randomness: two authenticated packets can
// persist the same evaluation yet have distinct IDs. Alternating replays must
// not reopen the password-protected files after both deliveries succeeded.
func TestRabbitVRFRelayAlternatingCiphertextReplayV1(t *testing.T) {
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(big.NewInt(9280), 11, common.HexToHash("0x1234"), 3)
	check(err)
	sid, err := lqc.RabbitVRFDKGSessionIDV1(session)
	check(err)
	ownerKey, err := crypto.GenerateKey()
	check(err)
	dealerKey, err := crypto.GenerateKey()
	check(err)
	owner := lqc.RabbitVRFCommitteeMemberV1{ShareID: 1, TicketHash: common.HexToHash("0x01"), Participant: crypto.PubkeyToAddress(ownerKey.PublicKey)}
	dealer := lqc.RabbitVRFCommitteeMemberV1{ShareID: 2, TicketHash: common.HexToHash("0x02"), Participant: crypto.PubkeyToAddress(dealerKey.PublicKey)}
	third := lqc.RabbitVRFCommitteeMemberV1{ShareID: 3, TicketHash: common.HexToHash("0x03"), Participant: common.Address{3}}
	dir := t.TempDir()
	password := "replay-regression-test"
	passwordFile := filepath.Join(dir, "password")
	check(os.WriteFile(passwordFile, []byte(password), 0600))
	keys, err := rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(filepath.Join(dir, "rabbit-vrf", "dkg-transport"))
	check(err)
	privateKey, binding, err := keys.Create(session, owner, password)
	check(err)
	defer zeroRabbitVRFDKGPrivateKeyV1(privateKey)
	root, err := lqc.RabbitVRFDKGTransportKeyRootV1(session, binding)
	check(err)
	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(session, owner, lqc.RabbitVRFDKGMessageTransportKeyBindingV1, root)
	check(err)
	hash, err := lqc.RabbitVRFDKGEnvelopeSigningHashV1(session, envelope)
	check(err)
	envelope.Signature, err = crypto.Sign(hash[:], ownerKey)
	check(err)
	polys, err := rabbitvrfstate.NewDKGDealerPolynomialStoreV1(t.TempDir(), 2, 1)
	check(err)
	poly, commitment, _, err := polys.Create(session, dealer, password)
	check(err)
	defer poly.Destroy()
	evaluation, err := poly.Evaluate(owner.ShareID)
	check(err)
	// Real encrypted files. The transport key uses production KDF parameters;
	// this fixture's evaluation and polynomial files use cheaper parameters.
	evaluations, err := rabbitvrfstate.NewDKGVerifiedEvaluationStoreV1(filepath.Join(dir, "rabbit-vrf", "dkg-evaluations"), 2, 1)
	check(err)
	check(evaluations.Store(session, owner, dealer, evaluation, password))
	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{vrfDKGInstanceDir: dir,
			config:  &ethconfig.Config{RabbitVRFDKGPasswordFile: passwordFile},
			handler: &handler{downloader: &downloader.Downloader{}}},
		secretReady: true,
		current: rabbitVRFDKGLocalContextV1{SessionID: sid, CanonicalSession: session,
			CanonicalMembers:            []lqc.RabbitVRFCommitteeMemberV1{owner, dealer, third},
			Members:                     []lqc.RabbitVRFCommitteeMemberV1{owner},
			CanonicalTransportBindings:  []lqc.RabbitVRFDKGTransportKeyBindingV1{binding, {}, {}},
			CanonicalTransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{envelope, {}, {}},
			PolynomialCommitments:       []lqc.RabbitVRFDKGPolynomialCommitmentV1{commitment}},
	}
	n := &rabbitVRFDKGTransport{runtime: runtime}
	runtime.backend.vrfDKGTransport = n
	packets := make([]lqc.RabbitVRFDKGEncryptedEvaluationV1, 2)
	for i := range packets {
		packets[i], _, _, err = lqc.NewRabbitVRFDKGEncryptedEvaluationV1(session, dealer, commitment, owner, binding, envelope, evaluation)
		check(err)
		hash, err := lqc.RabbitVRFDKGEncryptedEvaluationSigningHashV1(session, packets[i])
		check(err)
		packets[i].Signature, err = crypto.Sign(hash[:], dealerKey)
		check(err)
		check(n.handleInboundEncryptedRelayV1(nil, packets[i]))
	}
	a, err := lqc.RabbitVRFDKGEncryptedEvaluationIDV1(packets[0])
	check(err)
	b, err := lqc.RabbitVRFDKGEncryptedEvaluationIDV1(packets[1])
	check(err)
	if a == b {
		t.Fatal("fixture did not produce independent ciphertext IDs")
	}
	runtime.backend.config.RabbitVRFDKGPasswordFile = filepath.Join(dir, "absent-password")
	for i := 0; i < 8; i++ {
		if err := n.handleInboundEncryptedRelayV1(nil, packets[i%2]); err != nil {
			t.Fatalf("alternating persisted replay reopened encrypted store: %v", err)
		}
	}
	forged := cloneRabbitVRFEncryptedRelayPacketV1(packets[0])
	forged.Ciphertext[0] ^= 1
	if err := n.handleInboundEncryptedRelayV1(nil, forged); err == nil {
		t.Fatal("forged replay bypassed authentication")
	}
	// A new, authenticated ciphertext has not succeeded and must not receive a
	// cached success merely because another ciphertext occupies the same slot.
	fresh, _, _, err := lqc.NewRabbitVRFDKGEncryptedEvaluationV1(session, dealer, commitment, owner, binding, envelope, evaluation)
	check(err)
	hash, err = lqc.RabbitVRFDKGEncryptedEvaluationSigningHashV1(session, fresh)
	check(err)
	fresh.Signature, err = crypto.Sign(hash[:], dealerKey)
	check(err)
	if err := n.handleInboundEncryptedRelayV1(nil, fresh); err == nil {
		t.Fatal("unprocessed ciphertext was marked delivered")
	}
	freshID, err := lqc.RabbitVRFDKGEncryptedEvaluationIDV1(fresh)
	check(err)
	if n.encryptedRelay.deliveredIDV1([2]uint64{dealer.ShareID, owner.ShareID}, freshID) {
		t.Fatal("failed delivery polluted replay window")
	}
}

func TestRabbitVRFRelayReplayWindowBoundV1(t *testing.T) {
	var cache rabbitVRFEncryptedRelayCacheV1
	slot := [2]uint64{2, 1}
	for i := int64(1); i <= 100; i++ {
		cache.rememberDeliveredIDV1(slot, common.BigToHash(big.NewInt(i)))
	}
	if len(cache.deliveredRecent[slot]) != rabbitVRFDeliveredReplayWindowV1 {
		t.Fatal("unbounded replay window")
	}
	if cache.deliveredIDV1(slot, common.BigToHash(big.NewInt(1))) {
		t.Fatal("old ID was not evicted")
	}
	for i := int64(93); i <= 100; i++ {
		id := common.BigToHash(big.NewInt(i))
		if !cache.deliveredIDV1(slot, id) {
			t.Fatal("recent successful ID forgotten")
		}
		if cache.deliveredIDV1([2]uint64{1, 2}, id) {
			t.Fatal("delivery crossed recipient/dealer slot")
		}
		cache.rememberDeliveredIDV1(slot, id)
	}
	if len(cache.deliveredRecent[slot]) != rabbitVRFDeliveredReplayWindowV1 {
		t.Fatal("duplicate enlarged replay window")
	}
}
