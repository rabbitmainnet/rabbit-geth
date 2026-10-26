//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFPreparationCertificatePublishesWithoutFinalizationV1(t *testing.T) {
	session, members, keys, certificate := rabbitVRFAdversarialCertificateV1(t)
	walletStore := keystore.NewKeyStore(t.TempDir(), keystore.LightScryptN, keystore.LightScryptP)
	for _, key := range keys {
		account, err := walletStore.ImportECDSA(key, "certificate-test")
		if err != nil {
			t.Fatal(err)
		}
		if err = walletStore.Unlock(account, "certificate-test"); err != nil {
			t.Fatal(err)
		}
	}
	manager := accounts.NewManager(&accounts.Config{}, walletStore)
	defer manager.Close()
	dir := t.TempDir()
	shares, err := lqc.RabbitVRFKeysetCertificateVerificationSharesV1(session, certificate)
	if err != nil {
		t.Fatal(err)
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(dir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Store(session, rabbitvrfstate.DKGFinalKeysetV1{KeysetRoot: certificate.KeysetRoot, ThresholdPublicKey: certificate.ThresholdPublicKey, TranscriptRoot: certificate.TranscriptRoot, VerificationShares: shares}); err != nil {
		t.Fatal(err)
	}
	runtime := &rabbitVRFDKGRuntime{backend: &Ethereum{vrfDKGInstanceDir: dir, accountManager: manager}, current: rabbitVRFDKGLocalContextV1{SessionID: certificate.SessionID, CanonicalSession: session, CanonicalMembers: members, Members: members}}
	transport, err := newRabbitVRFDKGTransport(rabbitVRFDKGTransportConfig{ChainID: new(big.Int).Set(session.ChainID), NetworkID: 19282, Genesis: crypto.Keccak256Hash([]byte("preparation-publish")), Runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	runtime.backend.vrfDKGTransport = transport
	if len(transport.finalizations) != 0 {
		t.Fatal("fixture has a request/finalization")
	}
	if err = runtime.progressPreparationCertificateV1(); err != nil {
		t.Fatal(err)
	}
	certificates, err := transport.keysetCertificateStoreV1()
	if err != nil {
		t.Fatal(err)
	}
	got, err := certificates.Load(session, members)
	if err != nil {
		t.Fatalf("preparation did not persist verified quorum certificate: %v", err)
	}
	if got.KeysetRoot != certificate.KeysetRoot {
		t.Fatal("certificate root mismatch")
	}
	if _, err = lqc.ValidateRabbitVRFKeysetCertificateV1(session, members, got); err != nil {
		t.Fatal(err)
	}
	if err = runtime.progressPreparationCertificateV1(); err != nil {
		t.Fatal(err)
	}
	if len(transport.finalizations) != 0 {
		t.Fatal("certificate publication created a finalization")
	}
}
