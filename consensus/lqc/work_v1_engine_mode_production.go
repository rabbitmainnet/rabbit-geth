//go:build rabbit_workv1 && rabbit_randomx

package lqc

// WorkV1ProductionEnabled reports whether this binary contains the production
// Work V1 consensus and RandomX implementation.
func WorkV1ProductionEnabled() bool { return true }
