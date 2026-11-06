package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRabbitBootstrapRealSnapshot(t *testing.T) {
	if os.Getenv("RABBIT_TEST_REAL_BOOTSTRAP") != "1" {
		t.Skip("real snapshot test disabled")
	}

	source := os.Getenv("RABBIT_TEST_BOOTSTRAP_SOURCE")
	target := os.Getenv("RABBIT_TEST_BOOTSTRAP_TARGET")
	if source == "" || target == "" {
		t.Fatal("explicit bootstrap source and disposable target are required")
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("test target already exists; refusing overwrite")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}

	for i, expected := range rabbitBootstrapPartHashes {
		name := fmt.Sprintf("rabbit-testnet-bootstrap-385509.part-%03d", i)
		src := filepath.Join(source, name)
		dst := filepath.Join(target, ".rabbit-bootstrap-"+name)
		if valid, err := bootstrapFileMatches(src, expected); err != nil || !valid {
			t.Fatalf("source part %d verification failed: valid=%v err=%v", i, valid, err)
		}
		// Same-volume hard link avoids duplicating the large source parts.
		if err := os.Link(src, dst); err != nil {
			t.Fatalf("link part %d: %v", i, err)
		}
	}

	if err := bootstrapMultipartFrom(context.Background(), target, "https://example.invalid"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"chaindata", "triedb"} {
		if fi, err := os.Stat(filepath.Join(target, "rabbit", name)); err != nil || !fi.IsDir() {
			t.Fatalf("installed %s missing or invalid: %v", name, err)
		}
	}
	t.Log("Real Rabbit bootstrap installed successfully")
}
