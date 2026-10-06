package lqc

import "testing"

func TestRabbitVRFFourOnlineWithHistoricalSeats(t *testing.T) {
	const historicalSeats uint64 = 36
	const onlineWallets uint64 = 4

	committeeSize := RabbitVRFCommitteeSizeV1(historicalSeats, 32, 128)
	threshold, maxFaults, err := RabbitVRFThresholdPolicyV1(committeeSize)
	if err != nil {
		t.Fatal(err)
	}
	if committeeSize != 32 || threshold != 22 || maxFaults != 10 {
		t.Fatalf("unexpected policy: committee=%d threshold=%d faults=%d",
			committeeSize, threshold, maxFaults)
	}
	if onlineWallets >= threshold {
		t.Fatal("fixture should reproduce insufficient online participants")
	}
	t.Logf("BLOCK REPRODUCED: %d historical seats, committee=%d, threshold=%d, only %d online wallets",
		historicalSeats, committeeSize, threshold, onlineWallets)
	t.Log("Even if all four online wallets are selected, they cannot meet this threshold.")
}
