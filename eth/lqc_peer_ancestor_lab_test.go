package eth

import (
    "testing"
    "github.com/ethereum/go-ethereum/common"
)

func TestLQCPeerRecentCanonicalAncestorConfirmation(t *testing.T) {
    canonical := common.HexToHash("0x1111")
    conflicting := common.HexToHash("0x2222")
    cases := []struct {
        name string
        local, peer uint64
        advertised, canonical common.Hash
        want bool
    }{
        {"one behind same chain", 100, 99, canonical, canonical, true},
        {"two behind same chain", 100, 98, canonical, canonical, true},
        {"three behind rejected", 100, 97, canonical, canonical, false},
        {"equal height not ancestry", 100, 100, canonical, canonical, false},
        {"ahead not ancestry", 100, 101, canonical, canonical, false},
        {"forked ancestor rejected", 100, 99, conflicting, canonical, false},
        {"unknown canonical hash rejected", 100, 99, canonical, common.Hash{}, false},
        {"zero advertised hash rejected", 100, 99, common.Hash{}, common.Hash{}, false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got := lqcPeerConfirmsRecentAncestor(tc.local, tc.peer, tc.advertised, tc.canonical)
            if got != tc.want {
                t.Fatalf("got %v, want %v", got, tc.want)
            }
        })
    }
}
