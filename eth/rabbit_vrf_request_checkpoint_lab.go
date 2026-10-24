//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
)

var rabbitVRFCheckpointKeyV1 = []byte("rabbit/vrf/request-worker/checkpoint/v1")

type rabbitVRFRequestCheckpointV1 struct {
	Version  uint8         `json:"version"`
	Genesis  common.Hash   `json:"genesis"`
	Next     uint64        `json:"next"`
	LastHash common.Hash   `json:"lastHash"`
	Pending  []common.Hash `json:"pending"`
}

func rabbitVRFValidateCheckpointV1(c rabbitVRFRequestCheckpointV1, activation uint64) error {
	if activation == 0 || c.Version != 1 || c.Next < activation || c.Genesis == (common.Hash{}) {
		return errors.New("invalid rabbit vrf checkpoint")
	}
	if c.Next > activation && c.LastHash == (common.Hash{}) {
		return errors.New("missing canonical checkpoint hash")
	}
	seen := make(map[common.Hash]bool)
	for _, id := range c.Pending {
		if id == (common.Hash{}) || seen[id] {
			return errors.New("invalid or duplicate pending request")
		}
		seen[id] = true
	}
	return nil
}

func rabbitVRFSaveCheckpointV1(db ethdb.KeyValueWriter, c rabbitVRFRequestCheckpointV1, activation uint64) error {
	if err := rabbitVRFValidateCheckpointV1(c, activation); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := db.Put(rabbitVRFCheckpointKeyV1, data); err != nil {
		return fmt.Errorf("save rabbit vrf checkpoint: %w", err)
	}
	return nil
}

func rabbitVRFLoadCheckpointV1(db ethdb.KeyValueReader, activation uint64) (rabbitVRFRequestCheckpointV1, error) {
	var c rabbitVRFRequestCheckpointV1
	data, err := db.Get(rabbitVRFCheckpointKeyV1)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("decode rabbit vrf checkpoint: %w", err)
	}
	if err := rabbitVRFValidateCheckpointV1(c, activation); err != nil {
		return c, err
	}
	return c, nil
}
