//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

type realQueueReader321122 struct {
	cfg      *params.ChainConfig
	byNumber map[uint64]*types.Header
	byHash   map[common.Hash]*types.Header
	head     *types.Header
}

func (c *realQueueReader321122) Config() *params.ChainConfig                 { return c.cfg }
func (c *realQueueReader321122) CurrentHeader() *types.Header                { return c.head }
func (c *realQueueReader321122) GetHeaderByNumber(n uint64) *types.Header    { return c.byNumber[n] }
func (c *realQueueReader321122) GetHeaderByHash(h common.Hash) *types.Header { return c.byHash[h] }
func (c *realQueueReader321122) GetHeader(h common.Hash, n uint64) *types.Header {
	v := c.byNumber[n]
	if v != nil && v.Hash() == h {
		return v
	}
	return nil
}

func TestRabbitRealQueue321122(t *testing.T) {
	path := os.Getenv("RABBIT_QUEUE_REPLAY_FIXTURE")
	if path == "" {
		path = "testdata/rabbit-real-queue-321122.json"
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		ChainConfig *params.ChainConfig `json:"chainConfig"`
		Snapshots   map[string]struct {
			Value string `json:"value"`
		} `json:"snapshots"`
		ReplayHeaders []json.RawMessage `json:"replayHeaders"`
	}
	if err = json.Unmarshal(blob, &f); err != nil {
		t.Fatal(err)
	}
	if f.ChainConfig == nil || f.ChainConfig.LQC == nil {
		t.Fatal("missing config")
	}
	c := &realQueueReader321122{cfg: f.ChainConfig, byNumber: make(map[uint64]*types.Header), byHash: make(map[common.Hash]*types.Header)}
	for _, raw := range f.ReplayHeaders {
		var h types.Header
		if err = json.Unmarshal(raw, &h); err != nil {
			t.Fatal(err)
		}
		var meta struct {
			Hash common.Hash `json:"hash"`
		}
		if err = json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		if h.Hash() != meta.Hash {
			t.Fatalf("header hash mismatch at %d", h.Number.Uint64())
		}
		c.byNumber[h.Number.Uint64()] = &h
		c.byHash[h.Hash()] = &h
		if c.head == nil || h.Number.Cmp(c.head.Number) > 0 {
			c.head = &h
		}
	}
	for n := uint64(320641); n <= 321122; n++ {
		if c.byNumber[n] == nil || c.byNumber[n-1] == nil || c.byNumber[n].ParentHash != c.byNumber[n-1].Hash() {
			t.Fatalf("broken header chain at %d", n)
		}
	}
	rb, err := hexutil.Decode(f.Snapshots["registry"].Value)
	if err != nil {
		t.Fatal(err)
	}
	var parent RegistrySnapshot
	if err = rlp.DecodeBytes(rb, &parent); err != nil {
		t.Fatal(err)
	}
	if _, err = parent.Registry(); err != nil {
		t.Fatal(err)
	}
	wb, err := hexutil.Decode(f.Snapshots["work"].Value)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := decodeWorkV1EngineCheckpointV1(wb)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Runtime == nil || cp.Runtime.Work == nil || cp.Number != 321024 || parent.Number != 321024 || cp.Hash != parent.Hash || c.byNumber[321024].Hash() != parent.Hash {
		t.Fatal("checkpoint mismatch")
	}
	engine := New(f.ChainConfig.LQC, nil)
	runtime, err := workV1EngineLabRuntimeFor(engine)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.close(); workV1EngineLabRuntimes.Delete(engine) }()
	producer := common.HexToAddress("0x64389201da350f91b9daf910ec5b88786350a92f")
	reproduced := false
	for n := uint64(321025); n <= 321122; n++ {
		header := c.byNumber[n]
		source, has, err := WorkSelectionSourceEpochV1(n, cp.Runtime.Work.EpochLength)
		if err != nil || !has || source != cp.Runtime.Work.SelectionEpoch {
			t.Fatal("fixture crossed a selection epoch")
		}
		if n == 321122 {
			candidate := types.CopyHeader(header)
			candidate.Time = c.byNumber[n-1].Time + 13
			candidate.Coinbase = producer
			selection, _, err := engine.workV1EngineLabSelectionForHeader(c, cp.Runtime, &parent, candidate)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := parent.Registry()
			if err != nil {
				t.Fatal(err)
			}
			allowed, position := engine.isAuthorAllowedAt(n, selection, producer)
			t.Logf("REAL QUEUE: block=%d producer=%s allowed=%v position=%d", n, producer, allowed, position)
			missingBefore := 0
			for i, seat := range selection.Ordered {
				entry, exists := registry.Participant(seat.Address)
				if i < 6 {
					t.Logf("QUEUE[%d] %s registry=%v jailedUntil=%d", i, seat.Address, exists, entry.JailedUntil)
				}
				if i < position && !exists {
					missingBefore++
					t.Logf("MISSING BEFORE PRODUCER: %s", seat.Address)
				}
			}
			if !allowed || position != 1 || missingBefore == 0 {
				t.Fatal("real queue did not confirm the logged case; do not assume a diagnosis")
			}
			err = engine.workV1EngineLabApplySeatLiveness(registry.Clone(), n, selection, producer, engine.registryRules())
			if !errors.Is(err, ErrParticipantNotActive) {
				t.Fatalf("unexpected transition result: %v", err)
			}

			// Counterfactual activation ONLY inside this test. This is not
			// an activation proposal for the public network or a rewind.
			cfgV2 := *f.ChainConfig.LQC
			cfgV2.WorkSeatLivenessV2Block = n
			candidateEngine := New(&cfgV2, nil)
			corrected := registry.Clone()
			if err := candidateEngine.workV1EngineLabApplySeatLiveness(corrected, n, selection, producer, candidateEngine.registryRules()); err != nil {
				t.Fatalf("candidate still blocks the real authorized fallback: %v", err)
			}
			for i := 0; i < position; i++ {
				address := selection.Ordered[i].Address
				if _, existed := registry.Participant(address); !existed {
					entry, exists := corrected.Participant(address)
					if !exists || entry.Active || entry.LastHeartbeat != 0 || entry.MissedTurns != 1 || entry.RegisteredAt != n {
						t.Fatalf("incorrect missing-seat penalty bookkeeping: %+v", entry)
					}
				}
			}
			proposed := types.CopyHeader(candidate)
			proposed.Extra, err = EncodeRegistryHeaderExtra(n, corrected.Root(), nil)
			if err != nil {
				t.Fatal(err)
			}
			validated, err := candidateEngine.workV1EngineLabApplyRegistryBySeats(c, &parent, proposed, LQCHeaderEnvelopeV3{
				Version: LQCHeaderEnvelopeVersionV3, BlockNumber: n, RegistryRoot: corrected.Root(),
			}, selection)
			if err != nil || validated == nil || validated.RegistryRoot != corrected.Root() {
				t.Fatalf("candidate preparation/registry-validation mismatch: %v", err)
			}
			t.Log("CANDIDATE: real authorized fallback registry transition succeeds before recovery timeout; preparation and registry validation agree.")

			reproduced = true
		}
		env, err := ValidateLQCHeaderExtraV5(n, MaxWorkTicketsPerBlockV1, header.Extra)
		if err != nil {
			t.Fatal(err)
		}
		var child *RegistrySnapshot
		recovery := engine.openActivationForHeader(c, header)
		if recovery {
			extra, err := EncodeRegistryHeaderExtra(n, env.RegistryRoot, env.RegistryOperations)
			if err != nil {
				t.Fatal(err)
			}
			synthetic := types.CopyHeader(header)
			synthetic.Extra = extra
			child, err = engine.applyRegistryHeaderWithOpenActivation(&parent, c.cfg.ChainID, engine.registryRules(), synthetic, true)
			if err != nil {
				t.Fatalf("recovery registry replay %d: %v", n, err)
			}
			child.Hash = header.Hash()
		} else {
			selection, _, err := engine.workV1EngineLabSelectionForHeader(c, cp.Runtime, &parent, header)
			if err != nil {
				t.Fatal(err)
			}
			child, err = engine.workV1EngineLabApplyRegistryBySeats(c, &parent, header, LQCHeaderEnvelopeV3{
				Version: LQCHeaderEnvelopeVersionV3, BlockNumber: n, RegistryRoot: env.RegistryRoot,
				WorkStateRoot: env.WorkStateRoot, RegistryOperations: env.RegistryOperations,
			}, selection)
			if err != nil {
				t.Fatalf("normal registry replay %d: %v", n, err)
			}
		}
		if child.RegistryRoot != env.RegistryRoot || child.Hash != header.Hash() {
			t.Fatalf("root/hash mismatch at %d", n)
		}
		parent = *child
		if n == 321122 {
			t.Logf("ACTUAL BLOCK: interval=%ds recovery=%v registryRoot=%s", header.Time-c.byNumber[n-1].Time, recovery, parent.RegistryRoot)
		}
	}
	if !reproduced {
		t.Fatal("case not reached")
	}
	t.Log("98 REGISTRY TRANSITIONS MATCH THE CAPTURED HEADERS; REAL FALLBACK FAILURE REPRODUCED.")
	t.Log("Scope: registry transitions and selection; not full EVM replay or independent validation of all work/VRF proofs.")
}
