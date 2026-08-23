// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestRabbitConsensusEraLength(t *testing.T) {
	const want = uint64(8_409_600)
	for name, config := range map[string]*ChainConfig{
		"mainnet": RabbitChainConfig,
		"devnet":  RabbitDevnetChainConfig,
	} {
		if config.LQC == nil {
			t.Fatalf("%s has no LQC config", name)
		}
		if config.LQC.EraLength != want {
			t.Fatalf("%s era length = %d, want %d", name, config.LQC.EraLength, want)
		}
	}
}

func TestRabbitRegistryProtocolConfigValidation(t *testing.T) {
	participant := common.HexToAddress("0x1000000000000000000000000000000000000001")
	valid := &ChainConfig{LQC: &LQCConfig{
		RegistryProtocolBlock: 1,
		ProofDifficulty:       1,
		BootstrapParticipants: []common.Address{participant},
	}}
	if err := valid.CheckConfigForkOrder(); err != nil {
		t.Fatalf("valid registry configuration rejected: %v", err)
	}
	permissionless := &ChainConfig{LQC: &LQCConfig{
		RegistryProtocolBlock: 1,
		ProofDifficulty:       1,
		RecoveryTimeoutMs:     60_000,
	}}
	if err := permissionless.CheckConfigForkOrder(); err != nil {
		t.Fatalf("permissionless registry configuration rejected: %v", err)
	}

	tests := []struct {
		name   string
		config *LQCConfig
	}{
		{
			name: "zero proof difficulty",
			config: &LQCConfig{
				RegistryProtocolBlock: 1,
				BootstrapParticipants: []common.Address{participant},
			},
		},
		{
			name: "zero bootstrap address",
			config: &LQCConfig{
				RegistryProtocolBlock: 1,
				ProofDifficulty:       1,
				BootstrapParticipants: []common.Address{{}},
			},
		},
		{
			name: "duplicate bootstrap address",
			config: &LQCConfig{
				RegistryProtocolBlock: 1,
				ProofDifficulty:       1,
				BootstrapParticipants: []common.Address{participant, participant},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := (&ChainConfig{LQC: test.config}).CheckConfigForkOrder(); err == nil {
				t.Fatal("invalid registry configuration accepted")
			}
		})
	}

	// Disabled remains backward compatible and imposes no new genesis fields.
	if err := (&ChainConfig{LQC: &LQCConfig{}}).CheckConfigForkOrder(); err != nil {
		t.Fatalf("disabled registry protocol rejected: %v", err)
	}
}

func TestRabbitRegistryProtocolConfigCompatibility(t *testing.T) {
	config := func(activation uint64) *ChainConfig {
		return &ChainConfig{LQC: &LQCConfig{RegistryProtocolBlock: activation}}
	}
	if err := config(10).CheckCompatible(config(20), 9, 0); err != nil {
		t.Fatalf("future activation change rejected before either fork: %v", err)
	}
	err := config(10).CheckCompatible(config(20), 20, 0)
	if err == nil || err.What != "LQC registry protocol fork block" || err.RewindToBlock != 9 {
		t.Fatalf("unexpected registry compatibility error: %+v", err)
	}
	if err := config(0).CheckCompatible(config(0), 1_000, 0); err != nil {
		t.Fatalf("disabled registry configuration is incompatible with itself: %v", err)
	}

	stored := &ChainConfig{LQC: &LQCConfig{
		RegistryProtocolBlock: 10,
		ProofDifficulty:       100,
		BootstrapParticipants: []common.Address{
			common.HexToAddress("0x1000000000000000000000000000000000000001"),
			common.HexToAddress("0x2000000000000000000000000000000000000002"),
		},
	}}
	changedRules := &ChainConfig{LQC: &LQCConfig{
		RegistryProtocolBlock: 10,
		ProofDifficulty:       101,
		BootstrapParticipants: append([]common.Address(nil), stored.LQC.BootstrapParticipants...),
	}}
	if err := stored.CheckCompatible(changedRules, 9, 0); err != nil {
		t.Fatalf("future rule change rejected before activation: %v", err)
	}
	err = stored.CheckCompatible(changedRules, 10, 0)
	if err == nil || err.What != "LQC registry protocol rules" || err.RewindToBlock != 9 {
		t.Fatalf("unexpected registry-rules compatibility error: %+v", err)
	}
	reordered := &ChainConfig{LQC: &LQCConfig{
		RegistryProtocolBlock: 10,
		ProofDifficulty:       100,
		BootstrapParticipants: []common.Address{
			stored.LQC.BootstrapParticipants[1],
			stored.LQC.BootstrapParticipants[0],
		},
	}}
	if err := stored.CheckCompatible(reordered, 10, 0); err != nil {
		t.Fatalf("canonical bootstrap reordering should be compatible: %v", err)
	}
}

func TestCheckCompatible(t *testing.T) {
	type test struct {
		stored, new   *ChainConfig
		headBlock     uint64
		headTimestamp uint64
		wantErr       *ConfigCompatError
	}
	tests := []test{
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, headBlock: 0, headTimestamp: 0, wantErr: nil},
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, headBlock: 0, headTimestamp: uint64(time.Now().Unix()), wantErr: nil},
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, headBlock: 100, wantErr: nil},
		{
			stored:    &ChainConfig{EIP150Block: big.NewInt(10)},
			new:       &ChainConfig{EIP150Block: big.NewInt(20)},
			headBlock: 9,
			wantErr:   nil,
		},
		{
			stored:    AllEthashProtocolChanges,
			new:       &ChainConfig{HomesteadBlock: nil},
			headBlock: 3,
			wantErr: &ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      nil,
				RewindToBlock: 0,
			},
		},
		{
			stored:    AllEthashProtocolChanges,
			new:       &ChainConfig{HomesteadBlock: big.NewInt(1)},
			headBlock: 3,
			wantErr: &ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      big.NewInt(1),
				RewindToBlock: 0,
			},
		},
		{
			stored:    &ChainConfig{HomesteadBlock: big.NewInt(30), EIP150Block: big.NewInt(10)},
			new:       &ChainConfig{HomesteadBlock: big.NewInt(25), EIP150Block: big.NewInt(20)},
			headBlock: 25,
			wantErr: &ConfigCompatError{
				What:          "EIP150 fork block",
				StoredBlock:   big.NewInt(10),
				NewBlock:      big.NewInt(20),
				RewindToBlock: 9,
			},
		},
		{
			stored:    &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:       &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(30)},
			headBlock: 40,
			wantErr:   nil,
		},
		{
			stored:    &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:       &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(31)},
			headBlock: 40,
			wantErr: &ConfigCompatError{
				What:          "Petersburg fork block",
				StoredBlock:   nil,
				NewBlock:      big.NewInt(31),
				RewindToBlock: 30,
			},
		},
		{
			stored:        &ChainConfig{ShanghaiTime: newUint64(10)},
			new:           &ChainConfig{ShanghaiTime: newUint64(20)},
			headTimestamp: 9,
			wantErr:       nil,
		},
		{
			stored:        &ChainConfig{ShanghaiTime: newUint64(10)},
			new:           &ChainConfig{ShanghaiTime: newUint64(20)},
			headTimestamp: 25,
			wantErr: &ConfigCompatError{
				What:         "Shanghai fork timestamp",
				StoredTime:   newUint64(10),
				NewTime:      newUint64(20),
				RewindToTime: 9,
			},
		},
	}

	for _, test := range tests {
		err := test.stored.CheckCompatible(test.new, test.headBlock, test.headTimestamp)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nnew: %v\nheadBlock: %v\nheadTimestamp: %v\nerr: %v\nwant: %v", test.stored, test.new, test.headBlock, test.headTimestamp, err, test.wantErr)
		}
	}
}

func TestConfigRules(t *testing.T) {
	c := &ChainConfig{
		LondonBlock:  new(big.Int),
		ShanghaiTime: newUint64(500),
	}
	var stamp uint64
	if r := c.Rules(big.NewInt(0), true, stamp); r.IsShanghai {
		t.Errorf("expected %v to not be shanghai", stamp)
	}
	stamp = 500
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsShanghai {
		t.Errorf("expected %v to be shanghai", stamp)
	}
	stamp = math.MaxInt64
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsShanghai {
		t.Errorf("expected %v to be shanghai", stamp)
	}
}

func TestTimestampCompatError(t *testing.T) {
	require.Equal(t, new(ConfigCompatError).Error(), "")

	errWhat := "Shanghai fork timestamp"
	require.Equal(t, newTimestampCompatError(errWhat, nil, newUint64(1681338455)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp nil, want timestamp 1681338455, rewindto timestamp 1681338454)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(1681338455), nil).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 1681338455, want timestamp nil, rewindto timestamp 1681338454)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(1681338455), newUint64(600624000)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 1681338455, want timestamp 600624000, rewindto timestamp 600623999)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(0), newUint64(1681338455)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 0, want timestamp 1681338455, rewindto timestamp 0)")
}
