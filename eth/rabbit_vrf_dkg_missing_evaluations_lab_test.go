//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFDKGReadyWithoutEvaluationsCannotBuildShare(t *testing.T) {
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11, common.HexToHash("0xaaaa"), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 3)
	for i := range members {
		members[i] = lqc.RabbitVRFCommitteeMemberV1{
			ShareID:     uint64(i + 1),
			TicketHash:  common.BigToHash(big.NewInt(int64(i + 1))),
			Participant: common.BigToAddress(big.NewInt(int64(i + 1))),
		}
	}
	dir := t.TempDir()
	passwordFile := filepath.Join(dir, "test-password")
	if err := os.WriteFile(passwordFile, []byte("test-only-password"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{
			vrfDKGInstanceDir: dir,
			config: &ethconfig.Config{
				RabbitVRFDKGPasswordFile: passwordFile,
			},
			handler: &handler{downloader: &downloader.Downloader{}},
		},
		secretReady: true,
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        id,
			CanonicalSession: session,
			CanonicalMembers: members,
			Members:          members[:1],
		},
	}
	share, _, err := runtime.buildLocalSecretShareV1(members[0])
	if share != nil {
		t.Fatal("aggregation without evaluations returned a secret share")
	}
	if !errors.Is(err, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
		t.Fatalf("expected missing evaluation file, got: %v", err)
	}
	secretDir := filepath.Join(dir, "rabbit-vrf", "dkg-secret-shares")
	if _, statErr := os.Stat(secretDir); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected secret-share directory: %v", statErr)
	}
	t.Logf("MISSING EVALUATION CONFIRMED: %v", err)
}
