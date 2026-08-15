//go:build !rabbit_workv1 || !rabbit_randomx

package lqc

// WorkV1ProductionEnabled is false for default and laboratory-only binaries.
func WorkV1ProductionEnabled() bool { return false }
