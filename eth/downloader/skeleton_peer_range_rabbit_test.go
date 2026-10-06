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
