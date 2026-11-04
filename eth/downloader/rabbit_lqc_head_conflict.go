package downloader

import "errors"

// IsLQCBeaconHeadConflict limits forced skeleton replacement to
// a genuine head reorg, gap or fork. Other errors must not restart sync.
func IsLQCBeaconHeadConflict(err error) bool {
	return errors.Is(err, errChainReorged) ||
		errors.Is(err, errChainGapped) ||
		errors.Is(err, errChainForked)
}
