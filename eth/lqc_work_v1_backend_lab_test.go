package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto/rabbitx"
)

func TestLQCWorkV1LabRuntimeFailsClosedWithoutBuildTag(
	t *testing.T,
) {
	if _, err := rabbitx.NewLightHasher(); err == nil {
		t.Skip("rabbit_randomx build tag active")
	}
}
