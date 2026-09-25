//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package eth

import "github.com/ethereum/go-ethereum/p2p"

type rabbitVRFDKGTransport struct{}

func newRabbitVRFDKGTransportMaybeLab(
	backend *Ethereum,
	runtime *rabbitVRFDKGRuntime,
	networkID uint64,
) (*rabbitVRFDKGTransport, error) {
	return nil, nil
}

func (n *rabbitVRFDKGTransport) Protocol() p2p.Protocol {
	return p2p.Protocol{}
}

func (n *rabbitVRFDKGTransport) Close() {}
