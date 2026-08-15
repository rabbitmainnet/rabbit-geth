package lqc

import (
	"os"
	"strings"
	"testing"
)

func TestRabbitMainnetMiningRewardsAreImmediatelySpendable(t *testing.T) {
	raw, err := os.ReadFile("lqc.go")
	if err != nil {
		t.Fatalf("read lqc.go: %v", err)
	}
	src := string(raw)

	for _, forbidden := range []string{
		"vesting.CreditReward(",
		"vesting.ReleaseAllUnlockedRewards(",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("mining vesting path reintroduced into active LCQ consensus: %s", forbidden)
		}
	}

	if strings.Count(src, "tracing.BalanceIncreaseRewardMineBlock") < 3 {
		t.Fatal("active LCQ mining reward direct-credit paths are missing")
	}
}
