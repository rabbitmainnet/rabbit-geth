//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFKeysetCertificatePersistenceAdversarialV1(
	t *testing.T,
) {
	session, members, keys, certificate :=
		rabbitVRFAdversarialCertificateV1(t)

	newStore := func(
		t *testing.T,
	) *rabbitvrfstate.RabbitVRFKeysetCertificateStoreV1 {
		t.Helper()

		store, err :=
			rabbitvrfstate.NewRabbitVRFKeysetCertificateStoreV1(
				t.TempDir(),
			)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}

	resign := func(
		t *testing.T,
		certificate lqc.RabbitVRFKeysetCertificateV1,
	) lqc.RabbitVRFKeysetCertificateV1 {
		t.Helper()

		payloadHash, err :=
			lqc.RabbitVRFKeysetCertificatePayloadHashV1(
				session,
				certificate,
			)
		if err != nil {
			t.Fatal(err)
		}

		certificate.Signatures =
			make([][]byte, len(members))

		for i, member := range members {
			envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
				session,
				member,
				lqc.RabbitVRFDKGMessageKeysetCertificateV1,
				payloadHash,
			)
			if err != nil {
				t.Fatal(err)
			}

			signingHash, err :=
				lqc.RabbitVRFDKGEnvelopeSigningHashV1(
					session,
					envelope,
				)
			if err != nil {
				t.Fatal(err)
			}

			certificate.Signatures[i], err =
				gethcrypto.Sign(signingHash[:], keys[i])
			if err != nil {
				t.Fatal(err)
			}
		}

		validated, err :=
			lqc.ValidateRabbitVRFKeysetCertificateV1(
				session,
				members,
				certificate,
			)
		if err != nil {
			t.Fatal(err)
		}

		return validated
	}

	t.Run("duplicate_store_is_idempotent", func(t *testing.T) {
		store := newStore(t)

		if err := store.Store(
			session,
			members,
			certificate,
		); err != nil {
			t.Fatal(err)
		}

		if err := store.Store(
			session,
			members,
			certificate,
		); err != nil {
			t.Fatalf("duplicate store rejected: %v", err)
		}
	})

	t.Run("corrupted_file_is_rejected", func(t *testing.T) {
		store := newStore(t)

		if err := store.Store(
			session,
			members,
			certificate,
		); err != nil {
			t.Fatal(err)
		}

		path, err := store.Path(session)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			path,
			[]byte("{not-valid-json"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		_, err = store.Load(session, members)
		if !errors.Is(
			err,
			rabbitvrfstate.ErrInvalidRabbitVRFKeysetCertificateStoreV1,
		) {
			t.Fatalf(
				"corrupt load error=%v want=%v",
				err,
				rabbitvrfstate.ErrInvalidRabbitVRFKeysetCertificateStoreV1,
			)
		}
	})

	t.Run("stored_session_mismatch_is_rejected", func(t *testing.T) {
		store := newStore(t)

		if err := store.Store(
			session,
			members,
			certificate,
		); err != nil {
			t.Fatal(err)
		}

		path, err := store.Path(session)
		if err != nil {
			t.Fatal(err)
		}

		encoded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		var record map[string]any
		if err := json.Unmarshal(encoded, &record); err != nil {
			t.Fatal(err)
		}

		record["sessionId"] =
			gethcrypto.Keccak256Hash(
				[]byte("wrong-persisted-session"),
			).Hex()

		encoded, err = json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')

		if err := os.WriteFile(
			path,
			encoded,
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		_, err = store.Load(session, members)
		if !errors.Is(
			err,
			rabbitvrfstate.ErrInvalidRabbitVRFKeysetCertificateStoreV1,
		) {
			t.Fatalf(
				"session mismatch error=%v want=%v",
				err,
				rabbitvrfstate.ErrInvalidRabbitVRFKeysetCertificateStoreV1,
			)
		}
	})

	t.Run("conflicting_valid_certificate_is_rejected", func(t *testing.T) {
		store := newStore(t)

		if err := store.Store(
			session,
			members,
			certificate,
		); err != nil {
			t.Fatal(err)
		}

		conflict := cloneRabbitVRFCertificateV1(certificate)

		verificationShares, err :=
			lqc.RabbitVRFKeysetCertificateVerificationSharesV1(
				session,
				conflict,
			)
		if err != nil {
			t.Fatal(err)
		}

		conflict.TranscriptRoot =
			gethcrypto.Keccak256Hash(
				[]byte("conflicting-valid-transcript"),
			)

		conflictRoot, _, err := lqc.RabbitVRFKeysetRootV1(
			session.ChainID,
			session.TargetVRFEpoch,
			session.CommitteeRoot,
			session.CommitteeSize,
			session.Threshold,
			conflict.ThresholdPublicKey,
			conflict.TranscriptRoot,
			verificationShares,
		)
		if err != nil {
			t.Fatal(err)
		}
		conflict.KeysetRoot = conflictRoot
		conflict = resign(t, conflict)

		err = store.Store(
			session,
			members,
			conflict,
		)
		if !errors.Is(
			err,
			rabbitvrfstate.ErrRabbitVRFKeysetCertificateStoreConflictV1,
		) {
			t.Fatalf(
				"conflict error=%v want=%v",
				err,
				rabbitvrfstate.ErrRabbitVRFKeysetCertificateStoreConflictV1,
			)
		}
	})
}
