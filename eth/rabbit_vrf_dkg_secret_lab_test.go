//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestRabbitVRFDKGSecretV1PasswordFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password")

	if err := os.WriteFile(path, []byte("rabbit-secret\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	password, err := readRabbitVRFDKGPasswordFileV1(path)
	if err != nil {
		t.Fatal(err)
	}
	if password != "rabbit-secret" {
		t.Fatalf("unexpected password %q", password)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readRabbitVRFDKGPasswordFileV1(path); err == nil {
			t.Fatal("unsafe password-file permissions accepted")
		}

		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}

		link := filepath.Join(dir, "password-link")
		if err := os.Symlink(path, link); err != nil {
			t.Fatal(err)
		}
		if _, err := readRabbitVRFDKGPasswordFileV1(link); err == nil {
			t.Fatal("password-file symlink accepted")
		}
	}
}

func TestRabbitVRFDKGSecretV1RejectsEmptyPasswordFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")

	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := readRabbitVRFDKGPasswordFileV1(path); err == nil {
		t.Fatal("empty password file accepted")
	}
}

func TestRabbitVRFDKGSecretV1Zeroization(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	zeroRabbitVRFDKGBytesV1(data)

	for index, value := range data {
		if value != 0 {
			t.Fatalf("byte %d was not zeroized", index)
		}
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	zeroRabbitVRFDKGPrivateKeyV1(key)

	if key.D == nil || key.D.Sign() != 0 {
		t.Fatal("private scalar was not zeroized")
	}
}
