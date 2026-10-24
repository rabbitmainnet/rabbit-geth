package params

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestRabbitVRFCoordinatorV2RuntimeIntegrity(t *testing.T) {
	if len(RabbitVRFCoordinatorV2Code) == 0 {
		t.Fatal("empty V2 runtime")
	}
	if bytes.Equal(RabbitVRFCoordinatorV1Code, RabbitVRFCoordinatorV2Code) {
		t.Fatal("V2 unexpectedly equals V1")
	}
	got := fmt.Sprintf("%x", sha256.Sum256(RabbitVRFCoordinatorV2Code))
	if got != "26a5a0700663f46e2c7983ff5df62a2ed6a7788d47f089cf83185b67b7a9b81a" {
		t.Fatal("V2 runtime hash mismatch")
	}
}
