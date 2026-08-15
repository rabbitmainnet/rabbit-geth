//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package eth

// Default builds deliberately do not feed lqcw tickets into the consensus
// engine. The active bridge exists only in an explicit Work V1 build.
func wireWorkV1EngineTicketProviderMaybeLab(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
) error {
	return nil
}
