package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

const auditVersion = "rabbit-permissionless-registry-auditor/1.0.0"

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type report struct {
	Version string  `json:"version"`
	Status  string  `json:"status"`
	Checks  []check `json:"checks"`
}

func auditConfig() lqc.HybridLQCConfig {
	return lqc.HybridLQCConfig{
		MinBond:         big.NewInt(25),
		ActivationDelay: 0,
		HeartbeatWindow: 64,
		HeartbeatGrace:  16,
		CommitteeSize:   6,
		FallbackCount:   2,
	}
}

func isolatedQueue(local common.Address, mode string, bootstrap []common.Address, parent common.Hash, block uint64) []common.Address {
	lqc.ResetRuntimeRegistry()
	if local != (common.Address{}) {
		lqc.RegisterParticipant(nil, local, block)
		lqc.UpdateParticipantActivity(nil, local, block)
	}
	registry := lqc.RealRegistry(block, bootstrap, mode)
	selection := lqc.BuildHybridSelection(
		bootstrap,
		registry.ToHybridParticipants(),
		parent,
		block,
		auditConfig(),
	)
	queue := make([]common.Address, 0, len(selection.Ordered))
	for _, participant := range selection.Ordered {
		queue = append(queue, participant.Address)
	}
	return queue
}

func contains(queue []common.Address, target common.Address) bool {
	for _, address := range queue {
		if address == target {
			return true
		}
	}
	return false
}

func formatQueue(queue []common.Address) []string {
	out := make([]string, len(queue))
	for index, address := range queue {
		out[index] = address.Hex()
	}
	return out
}

func runAudit() report {
	parent := common.HexToHash("0x9d0d2f3a5ff09864f1f264908dff6e41b21a24d705885541f8af181584f9712e")
	block := uint64(500)
	nodeA := common.HexToAddress("0x1000000000000000000000000000000000000001")
	nodeB := common.HexToAddress("0x2000000000000000000000000000000000000002")
	bootstrap := common.HexToAddress("0x3000000000000000000000000000000000000003")
	newcomer := common.HexToAddress("0x4000000000000000000000000000000000000004")

	queueA := isolatedQueue(nodeA, "native", nil, parent, block)
	queueB := isolatedQueue(nodeB, "native", nil, parent, block)
	observerQueue := isolatedQueue(common.Address{}, "native", nil, parent, block)
	bootstrapQueue := isolatedQueue(newcomer, "bootstrap", []common.Address{bootstrap}, parent, block)

	checks := make([]check, 0, 3)

	if reflect.DeepEqual(queueA, queueB) && reflect.DeepEqual(queueA, observerQueue) {
		checks = append(checks, check{
			Name:   "native-global-determinism",
			Status: "PASS",
			Detail: "independent nodes derived the same participant queue",
		})
	} else {
		checks = append(checks, check{
			Name:   "native-global-determinism",
			Status: "FAIL",
			Detail: fmt.Sprintf(
				"same canonical head produced different queues: nodeA=%v nodeB=%v observer=%v",
				formatQueue(queueA),
				formatQueue(queueB),
				formatQueue(observerQueue),
			),
		})
	}

	if contains(bootstrapQueue, newcomer) {
		checks = append(checks, check{
			Name:   "bootstrap-newcomer-admission",
			Status: "PASS",
			Detail: "a participant outside genesis entered the bootstrap queue",
		})
	} else {
		checks = append(checks, check{
			Name:   "bootstrap-newcomer-admission",
			Status: "FAIL",
			Detail: fmt.Sprintf(
				"newcomer %s was ignored; queue=%v",
				newcomer.Hex(),
				formatQueue(bootstrapQueue),
			),
		})
	}

	if len(observerQueue) > 0 {
		checks = append(checks, check{
			Name:   "fresh-node-reconstruction",
			Status: "PASS",
			Detail: "a fresh node reconstructed the active participant set from canonical data",
		})
	} else {
		checks = append(checks, check{
			Name:   "fresh-node-reconstruction",
			Status: "FAIL",
			Detail: "a fresh native node with the same canonical head reconstructed an empty queue",
		})
	}

	status := "PASS"
	for _, item := range checks {
		if item.Status != "PASS" {
			status = "FAIL"
		}
	}

	return report{
		Version: auditVersion,
		Status:  status,
		Checks:  checks,
	}
}

func main() {
	report := runAudit()

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: encode report: %v\n", err)
		os.Exit(2)
	}

	if output := os.Getenv("RABBIT_REGISTRY_AUDIT_JSON"); output != "" {
		if err := os.WriteFile(output, append(encoded, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: write report: %v\n", err)
			os.Exit(2)
		}
	}

	fmt.Printf("Rabbit Chain — %s\n", report.Version)
	for _, item := range report.Checks {
		fmt.Printf("%s: %s\n  %s\n", item.Name, item.Status, item.Detail)
	}
	fmt.Printf("\nPERMISSIONLESS REGISTRY: %s\n", report.Status)

	if report.Status != "PASS" {
		os.Exit(1)
	}
}
