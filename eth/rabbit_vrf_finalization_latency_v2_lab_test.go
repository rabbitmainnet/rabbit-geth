//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFFinalizationLocalLatencyV2(t *testing.T) {
	const samples = 100
	durations := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		sender, receiver := p2p.MsgPipe()
		peer := &rabbitVRFDKGPeer{
			rw:      sender,
			version: rabbitVRFDKGProtocolVersionV2,
		}

		proof := lqc.RabbitVRFFinalizationV1{
			Version:   lqc.RabbitVRFFinalizationVersionV1,
			ProofHash: common.BigToHash(new(big.Int).SetUint64(uint64(i + 1))),
		}

		sent := make(chan error, 1)
		start := time.Now()

		go func() {
			sent <- peer.sendVerifiedFinalizationV2(proof)
		}()

		msg, err := receiver.ReadMsg()
		if err != nil {
			sender.Close()
			receiver.Close()
			t.Fatal(err)
		}

		var decoded lqc.RabbitVRFFinalizationV1
		err = msg.Decode(&decoded)
		msg.Discard()

		if err != nil || decoded.ProofHash != proof.ProofHash {
			sender.Close()
			receiver.Close()
			t.Fatalf("invalid delivered proof: %v", err)
		}

		durations = append(durations, time.Since(start))

		if err := <-sent; err != nil {
			t.Fatal(err)
		}

		sender.Close()
		receiver.Close()
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	percentile := func(p int) time.Duration {
		index := (samples*p+99)/100 - 1
		return durations[index]
	}

	t.Logf("VRF P2P LOCAL samples=%d P50=%s P95=%s P99=%s",
		samples, percentile(50), percentile(95), percentile(99))
}
