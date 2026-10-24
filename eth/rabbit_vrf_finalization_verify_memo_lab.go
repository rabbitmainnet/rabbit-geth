//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/sha256"
	"sync"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/rlp"
)

// Public cryptographic verdicts only. Canonical request admission is never cached.
var rabbitVRFFinalizationVerifyMemoV1 = struct {
	sync.Mutex
	known map[[32]byte]struct{}
	order [][32]byte
}{known: make(map[[32]byte]struct{})}

func rabbitVRFValidateFinalizationMemoV1(
	context lqc.RabbitVRFDKGSessionContextV1,
	members []lqc.RabbitVRFCommitteeMemberV1,
	certificate lqc.RabbitVRFKeysetCertificateV1,
	proof lqc.RabbitVRFFinalizationV1,
) error {
	encoded, err := rlp.EncodeToBytes(struct {
		Context     lqc.RabbitVRFDKGSessionContextV1
		Members     []lqc.RabbitVRFCommitteeMemberV1
		Certificate lqc.RabbitVRFKeysetCertificateV1
		Proof       lqc.RabbitVRFFinalizationV1
	}{context, members, certificate, proof})
	if err != nil {
		return err
	}
	key := sha256.Sum256(encoded)
	memo := &rabbitVRFFinalizationVerifyMemoV1
	memo.Lock()
	_, known := memo.known[key]
	memo.Unlock()
	if known {
		return nil
	}

	// No mutex is held during expensive cryptography.
	if err := lqc.ValidateRabbitVRFFinalizationProofV1(
		context, members, certificate, proof,
	); err != nil {
		return err
	}

	memo.Lock()
	defer memo.Unlock()
	if _, exists := memo.known[key]; !exists {
		if len(memo.order) >= 256 {
			delete(memo.known, memo.order[0])
			copy(memo.order, memo.order[1:])
			memo.order = memo.order[:len(memo.order)-1]
		}
		memo.known[key] = struct{}{}
		memo.order = append(memo.order, key)
	}
	return nil
}
