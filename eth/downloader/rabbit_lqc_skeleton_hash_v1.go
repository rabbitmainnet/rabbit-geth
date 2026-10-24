package downloader

import "github.com/ethereum/go-ethereum/params"

// Gate the recovery request strategy by the actual chain configuration. It
// neither changes block validity nor selects a different consensus fork.
func lqcSkeletonHashAnchoredV1(chain chainReader) bool {
	provider, ok := chain.(interface{ Config() *params.ChainConfig })
	if !ok {
		return false
	}
	config := provider.Config()
	return config != nil && config.LQC != nil
}
