package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestKeyAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "UTC--test")
	if err := os.WriteFile(path, []byte(`{"address":"da5bf4a009e63d6db4effaf5a2d6910f4d5bd2a0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	address, err := keyAddress(path)
	if err != nil {
		t.Fatal(err)
	}
	if address.Hex() != "0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0" {
		t.Fatalf("address=%s", address)
	}
}

func TestSessionPasswordFile(t *testing.T) {
	path, cleanup, err := sessionPasswordFile(t.TempDir(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "secret\n" {
		t.Fatalf("password file contents=%q", encoded)
	}
}

func TestConfiguredBootnodesPrefersFlag(t *testing.T) {
	t.Setenv("RABBIT_BOOTNODES", "enode://environment")
	got := configuredBootnodes(options{bootnodes: "enode://flag"})
	if got != "enode://flag" {
		t.Fatalf("bootnodes=%q", got)
	}
}

func TestWalletBackupMessage(t *testing.T) {
	address := common.HexToAddress(
		"0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0",
	)
	keyFile := filepath.Join(
		t.TempDir(),
		"keystore",
		"UTC--rabbit-wallet",
	)

	message := walletBackupMessage(keyFile, address)

	for _, expected := range []string{
		address.Hex(),
		keyFile,
		"encrypted wallet",
		"safe backup location",
	} {
		if !strings.Contains(
			strings.ToLower(message),
			strings.ToLower(expected),
		) {
			t.Fatalf(
				"backup message %q does not contain %q",
				message,
				expected,
			)
		}
	}
}

func TestInitializeAppliesGenesisToNewAndExistingDataDirs(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			opts := options{
				dataDir: t.TempDir(),
				node:    "rabbit-node",
				genesis: "genesis.json",
			}
			if existing {
				if err := os.MkdirAll(
					filepath.Join(opts.dataDir, "rabbit", "chaindata"),
					0700,
				); err != nil {
					t.Fatal(err)
				}
			}

			calls := 0
			runner := func(
				_ context.Context,
				_ io.Writer,
				executable string,
				args ...string,
			) error {
				calls++
				if executable != opts.node {
					t.Fatalf("executable = %q, want %q", executable, opts.node)
				}
				want := []string{
					"--datadir",
					opts.dataDir,
					"init",
					opts.genesis,
				}
				if !slices.Equal(args, want) {
					t.Fatalf("args = %q, want %q", args, want)
				}
				return nil
			}

			if err := initializeWithRunner(
				context.Background(),
				opts,
				runner,
			); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("init calls = %d, want 1", calls)
			}
		})
	}
}

func TestInitializePropagatesGenesisInitFailure(t *testing.T) {
	opts := options{
		dataDir: t.TempDir(),
		node:    "rabbit-node",
		genesis: "genesis.json",
	}
	wantErr := errors.New("genesis configuration rejected")

	runner := func(
		_ context.Context,
		_ io.Writer,
		_ string,
		_ ...string,
	) error {
		return wantErr
	}

	err := initializeWithRunner(
		context.Background(),
		opts,
		runner,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("initialize error = %v, want %v", err, wantErr)
	}
}
