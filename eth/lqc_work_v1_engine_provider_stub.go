//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package eth

import "github.com/ethereum/go-ethereum/consensus/lqc"

// Default builds deliberately do not feed lqcw tickets into the consensus
// engine. The active bridge exists only in an explicit Work V1 build.
func wireWorkV1EngineTicketProviderMaybeLab(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
) error {
	return nil
}

func wireLQCCommitteeClaimProviderLab(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
	engine *lqc.LQC,
) error {
	return nil
}

func startLQCCommitteeParticipantWorker(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
	engine *lqc.LQC,
) error {
	return nil
}
