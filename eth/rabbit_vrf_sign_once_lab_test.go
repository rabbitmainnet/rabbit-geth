//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	vrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFSingleShareSignsBothMessagesV1(t *testing.T) {
	context, members, _, cert := rabbitVRFAdversarialCertificateV1(t)
	shares, err := lqc.RabbitVRFKeysetCertificateVerificationSharesV1(context, cert)
	if err != nil {
		t.Fatal(err)
	}
	keyset := rabbitvrfstate.DKGFinalKeysetV1{KeysetRoot: cert.KeysetRoot, ThresholdPublicKey: cert.ThresholdPublicKey, TranscriptRoot: cert.TranscriptRoot, VerificationShares: shares}
	request := crypto.Keccak256Hash([]byte("single-share-two-real-signatures"))
	message, _, err := lqc.RabbitVRFThresholdMessageV1(context, keyset.KeysetRoot, request)
	if err != nil {
		t.Fatal(err)
	}
	share := rabbitVRFTestSecretShareV1(t, 1, 10)
	packet, err := rabbitVRFSignThresholdPacketWithShareV1(context, members[0], keyset, share, request, message)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := vrf.NewVerificationShare(1, shares[0].PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = vrf.VerifyPartial(verification, message, vrf.PartialSignature{ShareID: 1, Signature: packet.Signature}); err != nil {
		t.Fatal(err)
	}
	compact, _, err := lqc.RabbitVRFCompactParticipationMessageV1(context, keyset.KeysetRoot, request, packet.MessageHash, members[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = vrf.VerifyPartial(verification, compact, vrf.PartialSignature{ShareID: 1, Signature: packet.CompactParticipationSignature}); err != nil {
		t.Fatal(err)
	}
	packet.CompactParticipationSignature[0] ^= 1
	if _, err = vrf.VerifyPartial(verification, compact, vrf.PartialSignature{ShareID: 1, Signature: packet.CompactParticipationSignature}); err == nil {
		t.Fatal("tampered compact signature accepted")
	}
	wrong := rabbitVRFTestSecretShareV1(t, 1, 11)
	if _, err = rabbitVRFSignThresholdPacketWithShareV1(context, members[0], keyset, wrong, request, message); err == nil {
		t.Fatal("wrong secret share accepted")
	}
}
