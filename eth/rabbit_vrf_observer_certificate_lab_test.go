//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	vrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFObserverCertificatePublicKeysetV1(t *testing.T) {
	session, members, keys, _ := rabbitVRFAdversarialCertificateV1(t)
	sid, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	runtime := &rabbitVRFDKGRuntime{backend: &Ethereum{vrfDKGInstanceDir: dir}, current: rabbitVRFDKGLocalContextV1{SessionID: sid, CanonicalSession: session, CanonicalMembers: members}}
	n, err := newRabbitVRFDKGTransport(rabbitVRFDKGTransportConfig{ChainID: new(big.Int).Set(session.ChainID), NetworkID: 19282, Genesis: crypto.Keccak256Hash([]byte("observer")), Runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, ready, err := n.currentKeysetCertificateBaseV1()
	if err != nil || ready {
		t.Fatalf("missing commitments: ready=%v err=%v", ready, err)
	}
	var commitments []lqc.RabbitVRFDKGPolynomialCommitmentV1
	for i, member := range members {
		coefficients := make([]vrf.DKGCoefficientCommitmentV1, session.Threshold)
		for j := range coefficients {
			sk, e := vrf.GenerateSecretKey()
			if e != nil {
				t.Fatal(e)
			}
			pk, e := sk.PublicKey()
			if e != nil {
				t.Fatal(e)
			}
			copy(coefficients[j][:], pk[:])
		}
		commitment, payload, e := lqc.NewRabbitVRFDKGPolynomialCommitmentV1(session, member.ShareID, coefficients)
		if e != nil {
			t.Fatal(e)
		}
		envelope, e := lqc.NewRabbitVRFDKGEnvelopeV1(session, member, lqc.RabbitVRFDKGMessagePolynomialCommitmentV1, payload)
		if e != nil {
			t.Fatal(e)
		}
		hash, e := lqc.RabbitVRFDKGEnvelopeSigningHashV1(session, envelope)
		if e != nil {
			t.Fatal(e)
		}
		envelope.Signature, e = crypto.Sign(hash[:], keys[i])
		if e != nil {
			t.Fatal(e)
		}
		packet := rabbitVRFDKGPolynomialCommitmentPacketV1{Commitment: commitment, Envelope: envelope}
		if e = n.validatePolynomialCommitmentV1(packet); e != nil {
			t.Fatal(e)
		}
		if _, e = n.storeRemotePolynomialCommitmentV1(packet); e != nil {
			t.Fatal(e)
		}
		commitments = append(commitments, commitment)
	}
	root, pk, transcript, shares, err := lqc.RabbitVRFDKGFinalKeysetV1(session, commitments)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, _, ready, err := n.currentKeysetCertificateBaseV1()
	if err != nil || !ready {
		t.Fatalf("observer not ready: %v", err)
	}
	if cert.KeysetRoot != root || cert.ThresholdPublicKey != pk || cert.TranscriptRoot != transcript {
		t.Fatal("observer derived different public keyset")
	}
	if len(runtime.currentContext().Members) != 0 {
		t.Fatal("observer gained secret membership")
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(dir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(session)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.VerificationShares) != len(shares) {
		t.Fatal("verification shares missing")
	}
	for i := range shares {
		if got.VerificationShares[i] != shares[i] {
			t.Fatal("verification share mismatch")
		}
	}
}
