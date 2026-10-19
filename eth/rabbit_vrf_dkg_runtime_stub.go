//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package eth

import "github.com/ethereum/go-ethereum/consensus/lqc"

type rabbitVRFDKGRuntime struct{}

func newRabbitVRFDKGRuntimeMaybeLab(
	backend *Ethereum,
	engine *lqc.LQC,
) (*rabbitVRFDKGRuntime, error) {
	return nil, nil
}

func (runtime *rabbitVRFDKGRuntime) Start() error {
	return nil
}

func (runtime *rabbitVRFDKGRuntime) Close() {}
