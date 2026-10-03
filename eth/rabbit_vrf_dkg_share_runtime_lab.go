//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

type rabbitVRFDKGLocalSharesReadyV1 struct {
	session common.Hash
	keyset  common.Hash
	members []lqc.RabbitVRFCommitteeMemberV1
}

func rabbitVRFDKGLocalMembersEqualV1(a, b []lqc.RabbitVRFCommitteeMemberV1) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (runtime *rabbitVRFDKGRuntime) ensureLocalSecretSharesV1() (bool, error) {
	if runtime == nil || runtime.backend == nil || runtime.backend.config == nil ||
		runtime.backend.vrfDKGInstanceDir == "" {
		return false, errRabbitVRFDKGRuntimeV1
	}
	if !runtime.beginSecretOperationV1() {
		return false, nil
	}
	defer runtime.endSecretOperationV1()

	context := runtime.currentContext()
	if len(context.Members) == 0 {
		return false, nil
	}
	id, err := lqc.RabbitVRFDKGSessionIDV1(context.CanonicalSession)
	if err != nil || id != context.SessionID {
		return false, errRabbitVRFDKGArtifactSessionMismatch
	}
	if uint64(len(context.CanonicalMembers)) != context.CanonicalSession.CommitteeSize {
		return false, fmt.Errorf("rabbit vrf secret share canonical committee unavailable")
	}
	for i, member := range context.CanonicalMembers {
		if member.ShareID != uint64(i+1) {
			return false, fmt.Errorf("rabbit vrf secret share committee ordering mismatch")
		}
	}
	root := filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf")
	keysets, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(root, "dkg-final-keysets"),
	)
	if err != nil {
		return false, err
	}
	keyset, err := keysets.Load(context.CanonicalSession)
	if err != nil {
		return false, err
	}

	runtime.mu.RLock()
	cached := runtime.current.SessionID == context.SessionID &&
		runtime.localSharesReady.session == context.SessionID &&
		runtime.localSharesReady.keyset == keyset.KeysetRoot &&
		rabbitVRFDKGLocalMembersEqualV1(runtime.localSharesReady.members, context.Members)
	runtime.mu.RUnlock()
	if cached {
		return true, nil
	}
	password, err := readRabbitVRFDKGPasswordFileV1(
		runtime.backend.config.RabbitVRFDKGPasswordFile,
	)
	if err != nil {
		return false, err
	}
	shares, err := rabbitvrfstate.NewStandardDKGSecretShareStoreV1(
		filepath.Join(root, "dkg-secret-shares"),
	)
	if err != nil {
		return false, err
	}
	evaluations, err := rabbitvrfstate.NewStandardDKGVerifiedEvaluationStoreV1(
		filepath.Join(root, "dkg-evaluations"),
	)
	if err != nil {
		return false, err
	}

	for _, recipient := range context.Members {
		if recipient.ShareID == 0 ||
			recipient.ShareID > uint64(len(context.CanonicalMembers)) ||
			context.CanonicalMembers[recipient.ShareID-1] != recipient {
			return false, fmt.Errorf("rabbit vrf secret share local member mismatch")
		}
		share, loadErr := shares.Load(context.CanonicalSession, recipient, password)
		created := false
		if loadErr != nil {
			if !errors.Is(loadErr, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
				return false, loadErr
			}
			values := make([]rabbitvrf.DKGPolynomialEvaluationV1, 0, len(context.CanonicalMembers))
			for _, dealer := range context.CanonicalMembers {
				value, err := evaluations.Load(
					context.CanonicalSession, recipient, dealer, password,
				)
				if err != nil {
					if errors.Is(err, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
						return false, nil
					}
					return false, err
				}
				values = append(values, value)
			}
			share, err = rabbitvrf.AggregateDKGPolynomialEvaluationsV1(
				recipient.ShareID, values,
			)
			if err != nil {
				return false, err
			}
			created = true
		}
		publicKey, err := share.PublicKey()
		if err != nil {
			return false, err
		}
		matched := false
		for _, verification := range keyset.VerificationShares {
			if verification.ShareID == recipient.ShareID {
				matched = verification.PublicKey == publicKey
				break
			}
		}
		if !matched {
			return false, fmt.Errorf(
				"rabbit vrf local secret share %d does not match finalized keyset",
				recipient.ShareID,
			)
		}
		if runtime.currentContext().SessionID != context.SessionID {
			return false, errRabbitVRFDKGArtifactSessionMismatch
		}
		if created {
			if err := shares.Store(context.CanonicalSession, recipient, share, password); err != nil {
				return false, err
			}
		}
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.current.SessionID != context.SessionID ||
		!rabbitVRFDKGLocalMembersEqualV1(runtime.current.Members, context.Members) {
		return false, errRabbitVRFDKGArtifactSessionMismatch
	}
	runtime.localSharesReady = rabbitVRFDKGLocalSharesReadyV1{
		session: context.SessionID,
		keyset:  keyset.KeysetRoot,
		members: append([]lqc.RabbitVRFCommitteeMemberV1(nil), context.Members...),
	}
	return true, nil
}
