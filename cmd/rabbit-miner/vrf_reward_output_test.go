package main

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestVRFOutputDoesNotLabelOtherCredits(t *testing.T) {
	activity := rewardActivityRPC{
		ProducerReward: (*hexutil.Big)(big.NewInt(1200000000000000000)),
	}
	if lines := vrfRewardLines(activity, 1, big.NewInt(1), "tRAB"); len(lines) != 0 {
		t.Fatalf("LCQ reward incorrectly labelled VRF: %v", lines)
	}
	activity.VRFCredits = []vrfRewardCreditRPC{
		{Amount: nil},
		{Amount: (*hexutil.Big)(big.NewInt(0))},
		{Amount: (*hexutil.Big)(big.NewInt(-1))},
	}
	if lines := vrfRewardLines(activity, 1, big.NewInt(1), "tRAB"); len(lines) != 0 {
		t.Fatalf("nonpositive amounts printed: %v", lines)
	}
}

func TestVRFOutputAlongsideLCQ(t *testing.T) {
	request := common.HexToHash("0x1234")
	activity := rewardActivityRPC{
		ProducerReward: (*hexutil.Big)(big.NewInt(1200000000000000000)),
		VRFCredits: []vrfRewardCreditRPC{
			{RequestID: request, Role: "PRODUCER", Amount: (*hexutil.Big)(big.NewInt(400))},
			{RequestID: request, Role: "THRESHOLD PARTICIPANT", Amount: (*hexutil.Big)(big.NewInt(150))},
		},
	}
	lines := vrfRewardLines(activity, 42, big.NewInt(1000), "tRAB")
	if len(lines) != 2 {
		t.Fatalf("have %d VRF entries, want 2", len(lines))
	}
	for i, role := range []string{"PRODUCER", "THRESHOLD PARTICIPANT"} {
		if !strings.Contains(lines[i], "VRF REWARD | #42") ||
			!strings.Contains(lines[i], role) ||
			!strings.Contains(lines[i], request.Hex()) {
			t.Fatalf("incorrect VRF output: %q", lines[i])
		}
	}
	if (*big.Int)(activity.ProducerReward).Int64() != 1200000000000000000 {
		t.Fatal("LCQ reward modified")
	}
}
