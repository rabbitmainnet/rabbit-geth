package downloader

import (
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

type lqcHashTestReader struct{ fakeChainReader }

func (*lqcHashTestReader) Config() *params.ChainConfig {
	return &params.ChainConfig{LQC: &params.LQCConfig{}}
}

type lqcHashTestPeer struct {
	*skeletonTestPeer
	available bool
	corrupt   bool
	requested chan common.Hash
}

func (p *lqcHashTestPeer) RequestHeadersByNumber(uint64, int, int, bool, chan *eth.Response) (*eth.Request, error) {
	panic("LCQ must not request canonical headers by number")
}
func (p *lqcHashTestPeer) RequestHeadersByHash(hash common.Hash, amount, skip int, reverse bool, sink chan *eth.Response) (*eth.Request, error) {
	p.requested <- hash
	var headers []*types.Header
	if p.available {
		for i, h := range p.headers {
			if h.Hash() == hash {
				for j := i; j >= 1 && len(headers) < amount; j-- {
					headers = append(headers, p.headers[j])
				}
				break
			}
		}
	}
	if p.corrupt && len(headers) > 0 {
		h := types.CopyHeader(headers[0])
		h.Extra = []byte("wrong branch")
		headers[0] = h
	}
	req := &eth.Request{Peer: p.id}
	res := &eth.Response{Req: req, Res: (*eth.BlockHeadersRequest)(&headers), Time: 1, Done: make(chan error)}
	go func() {
		sink <- res
		if err := <-res.Done; err != nil {
			p.dropped.Add(1)
		}
	}()
	return req, nil
}

func TestLQCSkeletonHashAnchoredForkRecoveryV1(t *testing.T) {
	for _, mode := range []string{"available-side-branch", "missing-branch", "corrupt-anchor"} {
		t.Run(mode, func(t *testing.T) {
			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			headers := []*types.Header{{Number: big.NewInt(0), Extra: []byte("genesis")}}
			for i := 1; i <= 4; i++ {
				headers = append(headers, &types.Header{Number: big.NewInt(int64(i)), ParentHash: headers[i-1].Hash(), Extra: []byte("selected-side-branch")})
			}
			rawdb.WriteHeader(db, headers[0])
			rawdb.WriteCanonicalHash(db, headers[0].Hash(), 0)
			rawdb.WriteBody(db, headers[0].Hash(), 0, &types.Body{})
			rawdb.WriteReceipts(db, headers[0].Hash(), 0, types.Receipts{})
			peers := newPeerSet()
			peer := &lqcHashTestPeer{skeletonTestPeer: newSkeletonTestPeer("branch-peer", headers), available: mode != "missing-branch", corrupt: mode == "corrupt-anchor", requested: make(chan common.Hash, 8)}
			conn := newPeerConnection(peer.id, eth.ETH69, peer, log.New())
			if err := peers.Register(conn); err != nil {
				t.Fatal(err)
			}
			var drops atomic.Int32
			sk := newSkeleton(db, peers, func(string) { drops.Add(1) }, newHookedBackfiller(), &lqcHashTestReader{})
			defer sk.Terminate()
			if !sk.lqcHashAnchored {
				t.Fatal("LCQ strategy not enabled")
			}
			if err := sk.Sync(headers[4], nil, true); err != nil {
				t.Fatal(err)
			}
			select {
			case hash := <-peer.requested:
				if hash != headers[3].Hash() {
					t.Fatalf("wrong anchor %s", hash)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no hash request")
			}
			deadline := time.Now().Add(3 * time.Second)
			switch mode {
			case "available-side-branch":
				for rawdb.ReadSkeletonHeader(db, 1) == nil && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if h := rawdb.ReadSkeletonHeader(db, 1); h == nil || h.Hash() != headers[1].Hash() {
					t.Fatal("selected branch not recovered")
				}
				if drops.Load() != 0 || peer.dropped.Load() != 0 {
					t.Fatal("honest peer dropped")
				}
			case "missing-branch":
				time.Sleep(50 * time.Millisecond)
				if drops.Load() != 0 || peer.dropped.Load() != 0 {
					t.Fatal("missing branch disconnected honest peer")
				}
				if rawdb.ReadSkeletonHeader(db, 3) != nil {
					t.Fatal("missing branch accepted")
				}
			case "corrupt-anchor":
				for peer.dropped.Load() == 0 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if peer.dropped.Load() == 0 {
					t.Fatal("wrong hash accepted")
				}
				if rawdb.ReadSkeletonHeader(db, 3) != nil {
					t.Fatal("corrupt anchor persisted")
				}
			}
		})
	}
}
func TestLQCSkeletonHashAnchoringLeavesBeaconModeV1(t *testing.T) {
	if lqcSkeletonHashAnchoredV1(&fakeChainReader{}) {
		t.Fatal("generic beacon strategy changed")
	}
}
