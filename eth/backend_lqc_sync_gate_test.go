package eth

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestLQCMayRecoverWithoutSync(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	live := &types.Header{Number: big.NewInt(10), Time: uint64(now.Add(-time.Second).Unix())}
	stale := &types.Header{Number: big.NewInt(10), Time: uint64(now.Add(-2 * lqcHeadFreshness).Unix())}
	genesis := &types.Header{Number: new(big.Int), Time: uint64(now.Unix())}

	if lqcMayRecoverWithoutSync(live, 2, lqcSyncDiscoveryGrace-time.Second, now) {
		t.Fatal("live head must wait for discovery grace")
	}
	if !lqcMayRecoverWithoutSync(live, 2, lqcSyncDiscoveryGrace, now) {
		t.Fatal("live head with peers should recover after discovery grace")
	}
	if lqcMayRecoverWithoutSync(live, 0, lqcSyncDiscoveryGrace, now) {
		t.Fatal("isolated live head must never recover without peers")
	}
	if lqcMayRecoverWithoutSync(live, 0, lqcOfflineRecoveryGrace, now) {
		t.Fatal("isolated live head must remain blocked after offline recovery grace")
	}
	if lqcMayRecoverWithoutSync(stale, 2, lqcSyncDiscoveryGrace, now) {
		t.Fatal("stale head must not use the short discovery grace")
	}
	if lqcMayRecoverWithoutSync(genesis, 2, lqcSyncDiscoveryGrace-time.Second, now) {
		t.Fatal("connected genesis must wait for peer discovery grace")
	}
	if !lqcMayRecoverWithoutSync(genesis, 2, lqcSyncDiscoveryGrace, now) {
		t.Fatal("connected genesis must permit permissionless block-one bootstrap")
	}
	if lqcMayRecoverWithoutSync(stale, 0, lqcOfflineRecoveryGrace, now) {
		t.Fatal("isolated stale chain must never produce after offline recovery grace")
	}
	if !lqcMayRecoverWithoutSync(stale, 2, lqcOfflineRecoveryGrace, now) {
		t.Fatal("stale chain with peers should recover after the bounded grace")
	}
	if lqcMayRecoverWithoutSync(genesis, 0, lqcOfflineRecoveryGrace, now) {
		t.Fatal("fresh genesis must never enter offline recovery and create an isolated public-chain fork")
	}
}
