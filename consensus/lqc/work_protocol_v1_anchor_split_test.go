package lqc

import "testing"

func TestWorkAnchorScheduleV1(t *testing.T) {
	tests := []struct{ epoch, dataset, challenge uint64 }{{1, 0, 1}, {2, 0, 128}, {3, 128, 256}, {4, 256, 384}}
	for _, tc := range tests {
		d, err := WorkDatasetAnchorBlockV1(tc.epoch, WorkProtocolEpochLengthV1)
		if err != nil {
			t.Fatal(err)
		}
		c, err := WorkChallengeAnchorBlockV1(tc.epoch, WorkProtocolEpochLengthV1)
		if err != nil {
			t.Fatal(err)
		}
		if d != tc.dataset || c != tc.challenge {
			t.Fatalf("epoch %d got %d/%d want %d/%d", tc.epoch, d, c, tc.dataset, tc.challenge)
		}
	}
}

func TestWorkAnchorScheduleV1RejectsZero(t *testing.T) {
	if _, err := WorkDatasetAnchorBlockV1(0, 128); err == nil {
		t.Fatal("zero epoch accepted")
	}
	if _, err := WorkChallengeAnchorBlockV1(1, 0); err == nil {
		t.Fatal("zero epoch length accepted")
	}
}
