package rabbitvrfstate

import (
	"errors"
	"os"
	"testing"
)

func TestDKGVerifiedEvaluationMissingPreservesFilesystemCauseV1(t *testing.T) {
	context, member := dkgTransportKeyStoreFixtureV1(t)
	store, err := NewDKGVerifiedEvaluationStoreV1(t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load(context, member, member, "test-password")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("filesystem cause lost: %v", err)
	}
	if !errors.Is(err, ErrDKGTransportKeyStoreMissingV1) {
		t.Fatalf("legacy error lost: %v", err)
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("original path error lost: %v", err)
	}
}
