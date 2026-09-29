//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func (n *rabbitVRFDKGTransport) keysetCertificateStoreV1() (
	*rabbitvrfstate.RabbitVRFKeysetCertificateStoreV1,
	error,
) {
	if n == nil ||
		n.runtime == nil ||
		n.runtime.backend == nil ||
		n.runtime.backend.vrfDKGInstanceDir == "" {
		return nil, errors.New(
			"rabbit vrf keyset certificate store unavailable",
		)
	}

	return rabbitvrfstate.NewRabbitVRFKeysetCertificateStoreV1(
		filepath.Join(
			n.runtime.backend.vrfDKGInstanceDir,
			"rabbit-vrf",
			"keyset-certificates",
		),
	)
}

func (n *rabbitVRFDKGTransport) restorePersistedKeysetCertificateV1() error {
	session, members, base, _, ready, err :=
		n.currentKeysetCertificateBaseV1()

	if err != nil || !ready {
		return err
	}

	n.mu.RLock()
	alreadyReady :=
		n.keysetCertificateSession == base.SessionID &&
			n.keysetCertificateRoot == base.KeysetRoot &&
			len(n.keysetCertificate.Signatures) == len(members)
	n.mu.RUnlock()

	if alreadyReady {
		return nil
	}

	store, err := n.keysetCertificateStoreV1()
	if err != nil {
		return err
	}

	persisted, err := store.Load(session, members)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	if persisted.SessionID != base.SessionID ||
		persisted.KeysetRoot != base.KeysetRoot ||
		persisted.ThresholdPublicKey != base.ThresholdPublicKey ||
		persisted.TranscriptRoot != base.TranscriptRoot ||
		len(persisted.VerificationShareSamples) != len(base.VerificationShareSamples) {
		return errors.New(
			"rabbit vrf persisted keyset certificate mismatch",
		)
	}
	for index := range persisted.VerificationShareSamples {
		if persisted.VerificationShareSamples[index] !=
			base.VerificationShareSamples[index] {
			return errors.New(
				"rabbit vrf persisted keyset certificate verification shares mismatch",
			)
		}
	}

	signatures := make(map[uint64][]byte, len(members))
	for index, member := range members {
		signature := persisted.Signatures[index]
		if len(signature) == 0 {
			continue
		}
		signatures[member.ShareID] =
			append([]byte(nil), signature...)
	}

	n.mu.Lock()
	n.keysetCertificateSession = persisted.SessionID
	n.keysetCertificateRoot = persisted.KeysetRoot
	n.keysetCertificate = cloneRabbitVRFKeysetCertificateV1(persisted)
	n.keysetCertificateSignatures = signatures
	n.mu.Unlock()

	return nil
}

var _ = common.Hash{}
