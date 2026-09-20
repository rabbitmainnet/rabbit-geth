package lqc

import (
	"errors"
	"testing"
)

func TestRabbitVRFEpochScheduleV1(
	t *testing.T,
) {
	tests := []struct {
		source      uint64
		preparation uint64
		target      uint64
	}{
		{
			source:      1,
			preparation: 3,
			target:      4,
		},
		{
			source:      7,
			preparation: 9,
			target:      10,
		},
		{
			source:      100,
			preparation: 102,
			target:      103,
		},
	}

	for _, test := range tests {
		preparation, err :=
			RabbitVRFDKGPreparationEpochForSourceV1(
				test.source,
			)
		if err != nil {
			t.Fatal(err)
		}

		if preparation != test.preparation {
			t.Fatalf(
				"source=%d preparation=%d want=%d",
				test.source,
				preparation,
				test.preparation,
			)
		}

		target, err :=
			RabbitVRFTargetEpochForSourceWorkEpochV1(
				test.source,
			)
		if err != nil {
			t.Fatal(err)
		}

		if target != test.target {
			t.Fatalf(
				"source=%d target=%d want=%d",
				test.source,
				target,
				test.target,
			)
		}

		recovered, ok, err :=
			RabbitVRFSourceWorkEpochForTargetEpochV1(
				target,
			)
		if err != nil {
			t.Fatal(err)
		}

		if !ok ||
			recovered != test.source {
			t.Fatalf(
				"target=%d recovered=(%d,%t) want=(%d,true)",
				target,
				recovered,
				ok,
				test.source,
			)
		}
	}
}

func TestRabbitVRFEpochScheduleV1FirstPossibleTarget(
	t *testing.T,
) {
	for target := uint64(1); target <= RabbitVRFSourceToTargetEpochDelayV1; target++ {
		source, ok, err :=
			RabbitVRFSourceWorkEpochForTargetEpochV1(
				target,
			)
		if err != nil {
			t.Fatal(err)
		}

		if ok || source != 0 {
			t.Fatalf(
				"target=%d unexpectedly has source=%d ok=%t",
				target,
				source,
				ok,
			)
		}
	}

	source, ok, err :=
		RabbitVRFSourceWorkEpochForTargetEpochV1(
			RabbitVRFSourceToTargetEpochDelayV1 + 1,
		)
	if err != nil {
		t.Fatal(err)
	}

	if !ok || source != 1 {
		t.Fatalf(
			"first target recovered source=%d ok=%t",
			source,
			ok,
		)
	}
}

func TestRabbitVRFEpochScheduleV1BlockBoundaries(
	t *testing.T,
) {
	const epochLength uint64 = 128

	epoch, err :=
		RabbitVRFEpochForBlockV1(
			128,
			epochLength,
		)
	if err != nil {
		t.Fatal(err)
	}

	if epoch != 1 {
		t.Fatalf(
			"block 128 epoch=%d want=1",
			epoch,
		)
	}

	epoch, err =
		RabbitVRFEpochForBlockV1(
			129,
			epochLength,
		)
	if err != nil {
		t.Fatal(err)
	}

	if epoch != 2 {
		t.Fatalf(
			"block 129 epoch=%d want=2",
			epoch,
		)
	}

	preparationStart, err :=
		RabbitVRFDKGPreparationStartBlockV1(
			1,
			epochLength,
		)
	if err != nil {
		t.Fatal(err)
	}

	if preparationStart != 257 {
		t.Fatalf(
			"source 1 preparation start=%d want=257",
			preparationStart,
		)
	}

	targetStart, err :=
		RabbitVRFTargetEpochStartBlockForSourceV1(
			1,
			epochLength,
		)
	if err != nil {
		t.Fatal(err)
	}

	if targetStart != 385 {
		t.Fatalf(
			"source 1 target start=%d want=385",
			targetStart,
		)
	}

	if targetStart-preparationStart != epochLength {
		t.Fatalf(
			"preparation window=%d want=%d",
			targetStart-preparationStart,
			epochLength,
		)
	}
}

func TestRabbitVRFEpochScheduleV1RejectsInvalid(
	t *testing.T,
) {
	if _, err :=
		RabbitVRFTargetEpochForSourceWorkEpochV1(
			0,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFEpochV1,
	) {
		t.Fatalf(
			"zero source error=%v",
			err,
		)
	}

	if _, _, err :=
		RabbitVRFSourceWorkEpochForTargetEpochV1(
			0,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFEpochV1,
	) {
		t.Fatalf(
			"zero target error=%v",
			err,
		)
	}

	if _, err :=
		RabbitVRFTargetEpochForSourceWorkEpochV1(
			^uint64(0),
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFEpochV1,
	) {
		t.Fatalf(
			"overflow source error=%v",
			err,
		)
	}

	if _, err :=
		RabbitVRFEpochForBlockV1(
			0,
			128,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFEpochV1,
	) {
		t.Fatalf(
			"zero block error=%v",
			err,
		)
	}

	if _, err :=
		RabbitVRFEpochStartBlockV1(
			1,
			0,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFEpochV1,
	) {
		t.Fatalf(
			"zero epoch length error=%v",
			err,
		)
	}
}
