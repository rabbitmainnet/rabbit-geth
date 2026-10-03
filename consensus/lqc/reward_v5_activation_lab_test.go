//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type rewardV5ActivationChain struct {
	consensus.ChainHeaderReader
	config *params.ChainConfig
}

func (c *rewardV5ActivationChain) Config() *params.ChainConfig {
	return c.config
}

func TestRewardV5ActivationBoundaries(t *testing.T) {
	cases := []struct {
		name                   string
		activation, block, vrf uint64
		v5, wantOK             bool
	}{
		{"disabled", 0, 200000, 136193, true, false},
		{"before", 200000, 199999, 136193, true, false},
		{"at", 200000, 200000, 136193, true, true},
		{"after", 200000, 200001, 136193, true, true},
		{"vrfDisabled", 200000, 200000, 0, true, false},
		{"historicalV4", 200000, 77000, 136193, false, true},
		{"v4WithoutVRF", 200000, 200000, 0, false, true},
		{"v4RejectedInV5Era", 200000, 200000, 136193, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := common.HexToHash("0x31")
			work := common.HexToHash("0x32")
			claims := common.HexToHash("0x33")
			var extra []byte
			var err error
			if tc.v5 {
				extra, err = EncodeLQCHeaderExtraV5(
					tc.block, registry, work, claims,
					nil, nil, nil, nil, MaxWorkTicketsPerBlockV1,
				)
			} else {
				extra, err = EncodeLQCHeaderExtraV4(
					tc.block, registry, work, claims,
					nil, nil, nil, MaxWorkTicketsPerBlockV1,
				)
			}
			if err != nil {
				t.Fatal(err)
			}
			config := &params.LQCConfig{
				MiningRewardV5Block: tc.activation,
				VRFProtocolBlock:    tc.vrf,
			}
			chain := &rewardV5ActivationChain{
				config: &params.ChainConfig{LQC: config},
			}
			engine := &LQC{config: config}
			header := &types.Header{
				Number: new(big.Int).SetUint64(tc.block),
				Extra:  extra,
			}
			envelope, err := engine.workV1EngineLabRewardEnvelopeForHeader(
				chain, header,
			)
			if (err == nil) != tc.wantOK {
				t.Fatalf("success=%v want=%v err=%v", err == nil, tc.wantOK, err)
			}
			if !tc.wantOK {
				return
			}
			if envelope.BlockNumber != tc.block ||
				envelope.RegistryRoot != registry ||
				envelope.WorkStateRoot != work ||
				envelope.CommitteeClaimRoot != claims {
				t.Fatal("reward fields changed during decoding")
			}

			header.Number = new(big.Int).SetUint64(tc.block + 1)
			if _, err := engine.workV1EngineLabRewardEnvelopeForHeader(
				chain, header,
			); err == nil {
				t.Fatal("mismatched block number accepted")
			}

			header.Number = new(big.Int).SetUint64(tc.block)
			header.Extra = []byte("invalid")
			if _, err := engine.workV1EngineLabRewardEnvelopeForHeader(
				chain, header,
			); err == nil {
				t.Fatal("malformed header accepted")
			}
		})
	}
}
