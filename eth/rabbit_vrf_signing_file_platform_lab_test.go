//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRabbitVRFSigningFilePlatformV1(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "private.json")
	if err := os.WriteFile(path, []byte("encrypted-test-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := rabbitVRFSigningFileV1(path); err != nil {
		t.Fatalf("valid regular private file rejected on %s: %v", runtime.GOOS, err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := rabbitVRFSigningFileV1(path); err == nil {
			t.Fatal("Unix public-readable private file accepted")
		}
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := rabbitVRFSigningFileV1(root); err == nil {
		t.Fatal("directory accepted as signing file")
	}
	if _, err := rabbitVRFSigningFileV1(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing signing file accepted")
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(path, link); err == nil {
		if _, err := rabbitVRFSigningFileV1(link); err == nil {
			t.Fatal("symbolic link accepted as signing file")
		}
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	} else {
		t.Log("Windows symlink creation unavailable:", err)
	}

	if err := os.Truncate(path, 1048577); err != nil {
		t.Fatal(err)
	}
	if _, err := rabbitVRFSigningFileV1(path); err == nil {
		t.Fatal("oversized signing file accepted")
	}
}
