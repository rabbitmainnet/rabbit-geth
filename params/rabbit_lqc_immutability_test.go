package params

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func rabbitMainnetLQCImmutabilityFixture() *LQCConfig {
	return &LQCConfig{
		CommitteeMin:          32,
		CommitteeMax:          128,
		CommitteeRatioBps:     3000,
		FallbackSlots:         5,
		FallbackWindowMs:      3000,
		TargetBlockTimeMs:     10000,
		EraLength:             8409600,
		ProofType:             "lighthash-v1",
		ProofDifficulty:       100000,
		ActivityWindow:        128,
		EpochLength:           128,
		RegistryMode:          "native",
		BootstrapParticipants: []common.Address{common.HexToAddress("0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0")},
		RegistryProtocolBlock: 1,
		OpenRegistry:          false,
		BootstrapOnlyUntil:    0,
		MinBond:               big.NewInt(25),
		ActivationDelay:       2,
		HeartbeatWindow:       64,
		HeartbeatGrace:        16,
		CommitteeSize:         0,
		FallbackCount:         5,
		JailBlocks:            256,
		MaxMissedTurns:        3,
		MinorSlashBps:         500,
		MajorSlashBps:         2000,
	}
}

func cloneRabbitLQCConfig(in *LQCConfig) *LQCConfig {
	out := *in
	out.BootstrapParticipants = append([]common.Address(nil), in.BootstrapParticipants...)
	if in.MinBond != nil {
		out.MinBond = new(big.Int).Set(in.MinBond)
	}
	return &out
}

func TestRabbitLQCProtocolRulesAreImmutable(t *testing.T) {
	base := rabbitMainnetLQCImmutabilityFixture()

	tests := map[string]func(*LQCConfig){
		"proofType":          func(c *LQCConfig) { c.ProofType = "changed" },
		"proofDifficulty":    func(c *LQCConfig) { c.ProofDifficulty++ },
		"activityWindow":     func(c *LQCConfig) { c.ActivityWindow++ },
		"epochLength":        func(c *LQCConfig) { c.EpochLength++ },
		"registryMode":       func(c *LQCConfig) { c.RegistryMode = "changed" },
		"openRegistry":       func(c *LQCConfig) { c.OpenRegistry = !c.OpenRegistry },
		"bootstrapOnlyUntil": func(c *LQCConfig) { c.BootstrapOnlyUntil++ },
		"minBond":            func(c *LQCConfig) { c.MinBond = big.NewInt(26) },
		"activationDelay":    func(c *LQCConfig) { c.ActivationDelay++ },
		"heartbeatWindow":    func(c *LQCConfig) { c.HeartbeatWindow++ },
		"heartbeatGrace":     func(c *LQCConfig) { c.HeartbeatGrace++ },
		"committeeMin":       func(c *LQCConfig) { c.CommitteeMin++ },
		"committeeMax":       func(c *LQCConfig) { c.CommitteeMax++ },
		"committeeSize":      func(c *LQCConfig) { c.CommitteeSize++ },
		"committeeRatioBps":  func(c *LQCConfig) { c.CommitteeRatioBps++ },
		"fallbackSlots":      func(c *LQCConfig) { c.FallbackSlots++ },
		"fallbackCount":      func(c *LQCConfig) { c.FallbackCount++ },
		"fallbackWindowMs":   func(c *LQCConfig) { c.FallbackWindowMs++ },
		"targetBlockTimeMs":  func(c *LQCConfig) { c.TargetBlockTimeMs++ },
		"eraLength":          func(c *LQCConfig) { c.EraLength++ },
		"jailBlocks":         func(c *LQCConfig) { c.JailBlocks++ },
		"maxMissedTurns":     func(c *LQCConfig) { c.MaxMissedTurns++ },
		"minorSlashBps":      func(c *LQCConfig) { c.MinorSlashBps++ },
		"majorSlashBps":      func(c *LQCConfig) { c.MajorSlashBps++ },
		"bootstrapParticipants": func(c *LQCConfig) {
			c.BootstrapParticipants[0] = common.HexToAddress("0x0000000000000000000000000000000000000001")
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := cloneRabbitLQCConfig(base)
			mutate(changed)
			if base.registryProtocolRulesEqual(changed) {
				t.Fatalf("changed %s was incorrectly accepted as protocol-compatible", name)
			}
		})
	}

	equal := cloneRabbitLQCConfig(base)
	if !base.registryProtocolRulesEqual(equal) {
		t.Fatal("identical LQC protocol rules were rejected")
	}

	nilBond := cloneRabbitLQCConfig(base)
	nilBond.MinBond = nil
	if base.registryProtocolRulesEqual(nilBond) {
		t.Fatal("nil minBond was incorrectly accepted as protocol-compatible")
	}
}
