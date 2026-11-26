package downloader

import (
	"testing"

	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

type rabbitSkeletonRangePeer struct {
	Peer
	advertised *eth.BlockRangeUpdatePacket
}

func (p *rabbitSkeletonRangePeer) BlockRange() *eth.BlockRangeUpdatePacket {
	return p.advertised
}

func TestRabbitSkeletonPeerAdvertisedRange(t *testing.T) {
	peer := &rabbitSkeletonRangePeer{
		advertised: &eth.BlockRangeUpdatePacket{
			EarliestBlock: 100,
			LatestBlock:   21868,
		},
	}
	for _, test := range []struct {
		name   string
		number uint64
		want   bool
	}{
		{"behind_requested_head", 43463, false},
		{"latest_available", 21868, true},
		{"historical_available", 20000, true},
		{"earliest_available", 100, true},
		{"history_unavailable", 99, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := skeletonPeerCanServeHeader(peer, test.number); got != test.want {
				t.Fatalf("number=%d eligible=%v want=%v", test.number, got, test.want)
			}
		})
	}
	peer.advertised = nil
	if !skeletonPeerCanServeHeader(peer, 43463) {
		t.Fatal("peer with unknown range must retain existing behavior")
	}
	legacy := &rabbitSkeletonLegacyPeer{}
	if !skeletonPeerCanServeHeader(legacy, 43463) {
		t.Fatal("peer without range support must retain existing behavior")
	}
}

type rabbitSkeletonLegacyPeer struct {
	Peer
}

func TestRabbitSkeletonPeerAdvertisedBatchRange(t *testing.T) {
	const head uint64 = 2000
	needed := head - uint64(requestHeaders) + 1
	peer := &rabbitSkeletonRangePeer{advertised: &eth.BlockRangeUpdatePacket{EarliestBlock: needed, LatestBlock: head}}
	if !skeletonPeerCanServeHeaderBatch(peer, head) {
		t.Fatal("peer advertising the entire batch must be eligible")
	}
	peer.advertised.EarliestBlock = needed + 1
	if skeletonPeerCanServeHeaderBatch(peer, head) {
		t.Fatal("peer missing the oldest header in the batch must not be selected")
	}
	peer.advertised.EarliestBlock = 1
	peer.advertised.LatestBlock = head - 1
	if skeletonPeerCanServeHeaderBatch(peer, head) {
		t.Fatal("peer missing the requested head must not be selected")
	}
	// Short requests above genesis are not required to cover 192 headers.
	peer.advertised.EarliestBlock = 1
	peer.advertised.LatestBlock = 5
	if !skeletonPeerCanServeHeaderBatch(peer, 5) {
		t.Fatal("short above-genesis batch must be accepted")
	}
	peer.advertised = nil
	if !skeletonPeerCanServeHeaderBatch(peer, head) {
		t.Fatal("unknown range retains original behavior")
	}
	if !skeletonPeerCanServeHeaderBatch(&rabbitSkeletonLegacyPeer{}, head) {
		t.Fatal("legacy peer retains original behavior")
	}
}
