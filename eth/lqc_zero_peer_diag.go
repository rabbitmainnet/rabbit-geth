package eth

import "os"

// lqcDiagnosticZeroPeerAllowed is a temporary local-lab escape hatch.
// It must be removed before any public Rabbit Core release.
func lqcDiagnosticZeroPeerAllowed() bool {
	return os.Getenv("RABBIT_LQC_DIAG_ALLOW_ZERO_PEER") == "1"
}
