package main

import (
	"os"
	"testing"
)

func TestRabbitBootstrapRealAssembledSnapshot(t *testing.T) {
	if os.Getenv("RABBIT_TEST_ASSEMBLED_BOOTSTRAP") != "1" {
		t.Skip("real assembled snapshot test disabled")
	}
	archive := os.Getenv("RABBIT_TEST_ASSEMBLED_ARCHIVE")
	target := os.Getenv("RABBIT_TEST_BOOTSTRAP_TARGET")
	if archive == "" || target == "" {
		t.Fatal("explicit archive and disposable target required")
	}
	valid, err := bootstrapFileMatches(archive, rabbitBootstrapSHA256)
	if err != nil || !valid {
		t.Fatalf("archive verification failed: valid=%v err=%v", valid, err)
	}
	t.Log("Complete archive SHA256 verified; extracting")
	if err := installVerifiedBootstrap(target, archive); err != nil {
		t.Fatal(err)
	}
	t.Log("Real bootstrap extraction and atomic installation passed")
}
