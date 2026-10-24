package params

import "testing"

func TestRabbitForkV2OrderValidation(t *testing.T) {
	valid := LQCConfig{
		VRFProtocolBlock:         136193,
		ConsensusLivenessV5Block: 115000,
		ConsensusLivenessV6Block: 115022,
		VRFEconomicsV2Block:      312000,
		WorkSeatAdmissionV2Block: 312000,
	}
	if err := valid.validateRabbitForkV2Order(); err != nil {
		t.Fatal(err)
	}
	if err := (&LQCConfig{}).validateRabbitForkV2Order(); err != nil {
		t.Fatal("historical configuration rejected:", err)
	}
	var absent *LQCConfig
	if err := absent.validateRabbitForkV2Order(); err != nil {
		t.Fatal(err)
	}
	cases := []LQCConfig{
		{VRFEconomicsV2Block: 312000},
		{VRFProtocolBlock: 136193, VRFEconomicsV2Block: 136192},
		{VRFProtocolBlock: 136193, VRFEconomicsV2Block: 136193},
		{WorkSeatAdmissionV2Block: 312000},
		{ConsensusLivenessV5Block: 115000, WorkSeatAdmissionV2Block: 115000},
		{ConsensusLivenessV5Block: 115000,
			ConsensusLivenessV6Block: 115022, WorkSeatAdmissionV2Block: 115022},
	}
	for i, config := range cases {
		if err := config.validateRabbitForkV2Order(); err == nil {
			t.Fatalf("invalid configuration %d accepted", i)
		}
	}
}
