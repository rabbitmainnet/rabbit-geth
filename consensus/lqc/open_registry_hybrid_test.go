package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestOpenRegistryHybridSelection(t *testing.T) {
	ResetRuntimeRegistry()

	a := common.HexToAddress("0x1111111111111111111111111111111111111111")
	b := common.HexToAddress("0x2222222222222222222222222222222222222222")
	c := common.HexToAddress("0x3333333333333333333333333333333333333333")
	d := common.HexToAddress("0x4444444444444444444444444444444444444444")

	RegisterParticipant(nil, a, 1)
	RegisterParticipant(nil, b, 1)
	RegisterParticipant(nil, c, 1)
	RegisterParticipant(nil, d, 1)

	UpdateParticipantActivity(nil, a, 80)
	UpdateParticipantActivity(nil, b, 80)
	UpdateParticipantActivity(nil, c, 80)
	UpdateParticipantActivity(nil, d, 80)

	reg := RealRegistry(80, nil, "open")
	if reg == nil {
		t.Fatal("expected registry")
	}

	cfg := normalizeHybridConfig(HybridLQCConfig{
		MinBond:         big.NewInt(25),
		ActivationDelay: 16,
		HeartbeatWindow: 64,
		HeartbeatGrace:  16,
		CommitteeSize:   2,
		FallbackCount:   2,
	})

	parentHash := common.HexToHash("0xabc123")
	sel := BuildHybridSelection(nil, reg.ToHybridParticipants(), parentHash, 80, cfg)

	if len(sel.Ordered) != 4 {
		t.Fatalf("expected 4 ordered participants, got %d", len(sel.Ordered))
	}
	if sel.Producer == nil {
		t.Fatal("expected producer")
	}
	if len(sel.Fallbacks) != 2 {
		t.Fatalf("expected 2 fallbacks, got %d", len(sel.Fallbacks))
	}
	if len(sel.Committee) != 1 {
		t.Fatalf("expected 1 committee member after producer+2 fallbacks, got %d", len(sel.Committee))
	}
}

func TestBootstrapModeOnlyUsesBootstrap(t *testing.T) {
	ResetRuntimeRegistry()

	a := common.HexToAddress("0x1111111111111111111111111111111111111111")
	b := common.HexToAddress("0x2222222222222222222222222222222222222222")
	c := common.HexToAddress("0x3333333333333333333333333333333333333333")

	RegisterParticipant(nil, c, 50)
	UpdateParticipantActivity(nil, c, 80)

	reg := RealRegistry(80, []common.Address{a, b}, "bootstrap")
	cfg := normalizeHybridConfig(HybridLQCConfig{
		MinBond:            big.NewInt(25),
		ActivationDelay:    0,
		HeartbeatWindow:    64,
		HeartbeatGrace:     16,
		CommitteeSize:      2,
		FallbackCount:      1,
		BootstrapOnlyUntil: ^uint64(0),
	})

	sel := BuildHybridSelection([]common.Address{a, b}, reg.ToHybridParticipants(), common.HexToHash("0x1234"), 80, cfg)

	if len(sel.Ordered) != 2 {
		t.Fatalf("expected only bootstrap participants, got %d", len(sel.Ordered))
	}
}
