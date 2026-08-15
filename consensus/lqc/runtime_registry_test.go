package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRuntimeRegistryRegisterAndHeartbeat(t *testing.T) {
	ResetRuntimeRegistry()

	a := common.HexToAddress("0x1111111111111111111111111111111111111111")

	RegisterParticipant(nil, a, 5)
	UpdateParticipantActivity(nil, a, 12)

	reg := RealRegistry(12, nil, "open")
	if reg == nil {
		t.Fatal("expected registry")
	}

	entry := reg.Find(a)
	if entry == nil {
		t.Fatal("expected participant in runtime registry")
	}
	if !entry.Registered {
		t.Fatal("expected participant to be registered")
	}
	if !entry.Active {
		t.Fatal("expected participant to be active")
	}
	if entry.JoinedBlock != 5 {
		t.Fatalf("expected JoinedBlock=5, got %d", entry.JoinedBlock)
	}
	if entry.LastSeenBlock != 12 {
		t.Fatalf("expected LastSeenBlock=12, got %d", entry.LastSeenBlock)
	}
}
