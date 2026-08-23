//go:build !rabbit_randomx || (!linux && !darwin && !windows) || !cgo

package rabbitx

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
)

var ErrUnavailable = errors.New(
	"rabbit RandomX runtime unavailable: build with linux+cgo+rabbit_randomx and link the pinned Rabbit RandomX library",
)

type LightHasher struct{}

func NewLightHasher() (*LightHasher, error) {
	return nil, ErrUnavailable
}

func (*LightHasher) Hash(common.Hash, []byte) (common.Hash, error) {
	return common.Hash{}, ErrUnavailable
}

func (*LightHasher) Close() {}
