// Copyright 2019 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// Package utils contains internal helper functions for go-ethereum commands.
package utils

import (
	"flag"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/urfave/cli/v2"
)

func Test_SplitTagsFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args string
		want map[string]string
	}{
		{
			"2 tags case",
			"host=localhost,bzzkey=123",
			map[string]string{
				"host":   "localhost",
				"bzzkey": "123",
			},
		},
		{
			"1 tag case",
			"host=localhost123",
			map[string]string{
				"host": "localhost123",
			},
		},
		{
			"empty case",
			"",
			map[string]string{},
		},
		{
			"garbage",
			"smth=smthelse=123",
			map[string]string{
				"smth": "smthelse=123",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SplitTagsFlag(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitTagsFlag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNetworkPresetUsesFlagValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "unset",
			want: false,
		},
		{
			name: "enabled",
			args: []string{"--sepolia"},
			want: true,
		},
		{
			name: "explicit false",
			args: []string{"--sepolia=false"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := newTestContext(t, tt.args, NetworkFlags...)
			if got := IsNetworkPreset(ctx); got != tt.want {
				t.Fatalf("IsNetworkPreset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRabbitMiningFlagsConfigureProducer(t *testing.T) {
	producer := "0x1000000000000000000000000000000000000001"
	ctx := newTestContext(t, []string{
		"--mine",
		"--miner.pending.feeRecipient", producer,
	}, MiningEnabledFlag, MinerPendingFeeRecipientFlag)

	var mining miner.Config
	setMiner(ctx, &mining)
	if !mining.Enabled {
		t.Fatal("--mine did not enable Rabbit LQC production")
	}

	var ethcfg ethconfig.Config
	setEtherbase(ctx, &ethcfg)
	if ethcfg.Miner.PendingFeeRecipient != common.HexToAddress(producer) {
		t.Fatalf("producer = %s, want %s", ethcfg.Miner.PendingFeeRecipient, producer)
	}
}

func newTestContext(t *testing.T, args []string, flags ...cli.Flag) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range flags {
		if err := f.Apply(set); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.Parse(args); err != nil {
		t.Fatal(err)
	}
	return cli.NewContext(nil, set, nil)
}
