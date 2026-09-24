package lqc

import (
	"bytes"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	RabbitVRFDKGLifecycleVersionV1 uint8 = 1

	RabbitVRFDKGLifecyclePhasePreparedV1 uint8 = 1
)

var (
	ErrInvalidRabbitVRFDKGLifecycleV1 = errors.New(
		"invalid rabbit vrf dkg lifecycle v1",
	)

	ErrRabbitVRFDKGLifecycleConflictV1 = errors.New(
		"rabbit vrf dkg lifecycle conflict v1",
	)
)

var rabbitVRFDKGLifecyclePrefixV1 = []byte(
	"rabbit-lqc-vrf-dkg-lifecycle-v1:",
)

var rabbitVRFDKGLifecycleMu sync.Mutex

// RabbitVRFDKGLifecycleV1 is the persisted public lifecycle identity for one
// immutable Rabbit VRF DKG session.
//
// It deliberately stores no private key, password, credential or P2P state.
// Those concerns remain outside this deterministic consensus-facing record.
type RabbitVRFDKGLifecycleV1 struct {
	Version          uint8
	Phase            uint8
	SessionID        common.Hash
	SourceWorkEpoch  uint64
	PreparationEpoch uint64
	TargetVRFEpoch   uint64
	SelectionRoot    common.Hash
	DatasetKey       common.Hash
	Entropy          common.Hash
	CommitteeSeed    common.Hash
	CommitteeRoot    common.Hash
	Session          RabbitVRFDKGSessionContextV1
}

func rabbitVRFDKGLifecycleKeyV1(
	sessionID common.Hash,
) []byte {
	key := make(
		[]byte,
		0,
		len(rabbitVRFDKGLifecyclePrefixV1)+len(sessionID),
	)
	key = append(key, rabbitVRFDKGLifecyclePrefixV1...)
	key = append(key, sessionID[:]...)
	return key
}

func validateRabbitVRFDKGBridgeLifecycleInputV1(
	bridge RabbitVRFDKGBridgeV1,
) error {
	if bridge.SessionID == (common.Hash{}) ||
		bridge.SourceWorkEpoch == 0 ||
		bridge.PreparationEpoch == 0 ||
		bridge.TargetVRFEpoch == 0 ||
		bridge.SelectionRoot == (common.Hash{}) ||
		bridge.DatasetKey == (common.Hash{}) ||
		bridge.Entropy == (common.Hash{}) ||
		bridge.CommitteeSeed == (common.Hash{}) ||
		bridge.CommitteeRoot == (common.Hash{}) ||
		len(bridge.Members) == 0 {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	if err := ValidateRabbitVRFDKGSessionContextV1(
		bridge.Session,
	); err != nil {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(
		bridge.Session,
	)
	if err != nil || sessionID != bridge.SessionID {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	preparationEpoch, err :=
		RabbitVRFDKGPreparationEpochForSourceV1(
			bridge.SourceWorkEpoch,
		)
	if err != nil ||
		preparationEpoch != bridge.PreparationEpoch {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	targetEpoch, err :=
		RabbitVRFTargetEpochForSourceWorkEpochV1(
			bridge.SourceWorkEpoch,
		)
	if err != nil ||
		targetEpoch != bridge.TargetVRFEpoch {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	if bridge.Session.TargetVRFEpoch !=
		bridge.TargetVRFEpoch ||
		bridge.Session.CommitteeRoot !=
			bridge.CommitteeRoot ||
		bridge.Session.CommitteeSize !=
			uint64(len(bridge.Members)) {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	committee := make(
		[]WorkSeatV1,
		len(bridge.Members),
	)

	for index, member := range bridge.Members {
		if member.ShareID != uint64(index+1) ||
			member.TicketHash == (common.Hash{}) ||
			member.Participant == (common.Address{}) {
			return ErrInvalidRabbitVRFDKGLifecycleV1
		}

		committee[index] = WorkSeatV1{
			TicketHash:  member.TicketHash,
			Participant: member.Participant,
		}
	}

	committeeRoot, canonicalMembers, err :=
		RabbitVRFCommitteeRootV1(
			bridge.Session.ChainID,
			bridge.SourceWorkEpoch,
			bridge.SelectionRoot,
			bridge.CommitteeSeed,
			committee,
		)
	if err != nil ||
		committeeRoot != bridge.CommitteeRoot ||
		len(canonicalMembers) != len(bridge.Members) {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	for index := range canonicalMembers {
		if canonicalMembers[index] != bridge.Members[index] {
			return ErrInvalidRabbitVRFDKGLifecycleV1
		}
	}

	return nil
}

func newRabbitVRFDKGLifecycleV1(
	bridge RabbitVRFDKGBridgeV1,
) (*RabbitVRFDKGLifecycleV1, error) {
	if err :=
		validateRabbitVRFDKGBridgeLifecycleInputV1(
			bridge,
		); err != nil {
		return nil, err
	}

	return &RabbitVRFDKGLifecycleV1{
		Version:          RabbitVRFDKGLifecycleVersionV1,
		Phase:            RabbitVRFDKGLifecyclePhasePreparedV1,
		SessionID:        bridge.SessionID,
		SourceWorkEpoch:  bridge.SourceWorkEpoch,
		PreparationEpoch: bridge.PreparationEpoch,
		TargetVRFEpoch:   bridge.TargetVRFEpoch,
		SelectionRoot:    bridge.SelectionRoot,
		DatasetKey:       bridge.DatasetKey,
		Entropy:          bridge.Entropy,
		CommitteeSeed:    bridge.CommitteeSeed,
		CommitteeRoot:    bridge.CommitteeRoot,
		Session:          bridge.Session,
	}, nil
}

func ValidateRabbitVRFDKGLifecycleV1(
	state *RabbitVRFDKGLifecycleV1,
) error {
	if state == nil ||
		state.Version != RabbitVRFDKGLifecycleVersionV1 ||
		state.Phase != RabbitVRFDKGLifecyclePhasePreparedV1 ||
		state.SessionID == (common.Hash{}) ||
		state.SourceWorkEpoch == 0 ||
		state.PreparationEpoch == 0 ||
		state.TargetVRFEpoch == 0 ||
		state.SelectionRoot == (common.Hash{}) ||
		state.DatasetKey == (common.Hash{}) ||
		state.Entropy == (common.Hash{}) ||
		state.CommitteeSeed == (common.Hash{}) ||
		state.CommitteeRoot == (common.Hash{}) {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	if err := ValidateRabbitVRFDKGSessionContextV1(
		state.Session,
	); err != nil {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(
		state.Session,
	)
	if err != nil || sessionID != state.SessionID {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	preparationEpoch, err :=
		RabbitVRFDKGPreparationEpochForSourceV1(
			state.SourceWorkEpoch,
		)
	if err != nil ||
		preparationEpoch != state.PreparationEpoch {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	targetEpoch, err :=
		RabbitVRFTargetEpochForSourceWorkEpochV1(
			state.SourceWorkEpoch,
		)
	if err != nil ||
		targetEpoch != state.TargetVRFEpoch {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	if state.Session.TargetVRFEpoch !=
		state.TargetVRFEpoch ||
		state.Session.CommitteeRoot !=
			state.CommitteeRoot {
		return ErrInvalidRabbitVRFDKGLifecycleV1
	}

	return nil
}

func rabbitVRFDKGLifecycleMatchesBridgeV1(
	state *RabbitVRFDKGLifecycleV1,
	bridge RabbitVRFDKGBridgeV1,
) bool {
	if ValidateRabbitVRFDKGLifecycleV1(state) != nil ||
		validateRabbitVRFDKGBridgeLifecycleInputV1(
			bridge,
		) != nil {
		return false
	}

	if state.Session.ChainID == nil ||
		bridge.Session.ChainID == nil {
		return false
	}

	return state.Version ==
		RabbitVRFDKGLifecycleVersionV1 &&
		state.Phase ==
			RabbitVRFDKGLifecyclePhasePreparedV1 &&
		state.SessionID == bridge.SessionID &&
		state.SourceWorkEpoch == bridge.SourceWorkEpoch &&
		state.PreparationEpoch == bridge.PreparationEpoch &&
		state.TargetVRFEpoch == bridge.TargetVRFEpoch &&
		state.SelectionRoot == bridge.SelectionRoot &&
		state.DatasetKey == bridge.DatasetKey &&
		state.Entropy == bridge.Entropy &&
		state.CommitteeSeed == bridge.CommitteeSeed &&
		state.CommitteeRoot == bridge.CommitteeRoot &&
		state.Session.Version == bridge.Session.Version &&
		state.Session.ChainID.Cmp(
			bridge.Session.ChainID,
		) == 0 &&
		state.Session.TargetVRFEpoch ==
			bridge.Session.TargetVRFEpoch &&
		state.Session.CommitteeRoot ==
			bridge.Session.CommitteeRoot &&
		state.Session.CommitteeSize ==
			bridge.Session.CommitteeSize &&
		state.Session.Threshold ==
			bridge.Session.Threshold &&
		state.Session.MaxFaults ==
			bridge.Session.MaxFaults
}

func encodeRabbitVRFDKGLifecycleV1(
	state *RabbitVRFDKGLifecycleV1,
) ([]byte, error) {
	if err := ValidateRabbitVRFDKGLifecycleV1(
		state,
	); err != nil {
		return nil, err
	}

	return rlp.EncodeToBytes(state)
}

func decodeRabbitVRFDKGLifecycleV1(
	blob []byte,
) (*RabbitVRFDKGLifecycleV1, error) {
	if len(blob) == 0 {
		return nil, ErrInvalidRabbitVRFDKGLifecycleV1
	}

	var state RabbitVRFDKGLifecycleV1

	if err := rlp.DecodeBytes(
		blob,
		&state,
	); err != nil {
		return nil, ErrInvalidRabbitVRFDKGLifecycleV1
	}

	if err := ValidateRabbitVRFDKGLifecycleV1(
		&state,
	); err != nil {
		return nil, err
	}

	canonical, err := rlp.EncodeToBytes(&state)
	if err != nil ||
		!bytes.Equal(canonical, blob) {
		return nil, ErrInvalidRabbitVRFDKGLifecycleV1
	}

	return &state, nil
}

// LoadRabbitVRFDKGLifecycleV1 restores one immutable lifecycle by SessionID.
func (l *LQC) LoadRabbitVRFDKGLifecycleV1(
	sessionID common.Hash,
) (*RabbitVRFDKGLifecycleV1, error) {
	if l == nil ||
		l.db == nil ||
		sessionID == (common.Hash{}) {
		return nil, ErrInvalidRabbitVRFDKGLifecycleV1
	}

	blob, err := l.db.Get(
		rabbitVRFDKGLifecycleKeyV1(sessionID),
	)
	if err != nil {
		return nil, err
	}

	state, err :=
		decodeRabbitVRFDKGLifecycleV1(blob)
	if err != nil {
		return nil, err
	}

	if state.SessionID != sessionID {
		return nil, ErrInvalidRabbitVRFDKGLifecycleV1
	}

	return state, nil
}

// EnsureRabbitVRFDKGLifecycleV1 creates or resumes the immutable public
// lifecycle record for a canonical bridge.
//
// Different branches may persist different SessionIDs. No global "active
// session" pointer is stored here, so canonicality always remains a property
// of the current chain-derived bridge.
func (l *LQC) EnsureRabbitVRFDKGLifecycleV1(
	bridge RabbitVRFDKGBridgeV1,
) (
	*RabbitVRFDKGLifecycleV1,
	bool,
	error,
) {
	if l == nil || l.db == nil {
		return nil,
			false,
			ErrInvalidRabbitVRFDKGLifecycleV1
	}

	expected, err :=
		newRabbitVRFDKGLifecycleV1(bridge)
	if err != nil {
		return nil, false, err
	}

	rabbitVRFDKGLifecycleMu.Lock()
	defer rabbitVRFDKGLifecycleMu.Unlock()

	key := rabbitVRFDKGLifecycleKeyV1(
		bridge.SessionID,
	)

	has, err := l.db.Has(key)
	if err != nil {
		return nil, false, err
	}

	if has {
		existing, err :=
			l.LoadRabbitVRFDKGLifecycleV1(
				bridge.SessionID,
			)
		if err != nil {
			return nil, false, err
		}

		if !rabbitVRFDKGLifecycleMatchesBridgeV1(
			existing,
			bridge,
		) {
			return nil,
				false,
				ErrRabbitVRFDKGLifecycleConflictV1
		}

		return existing, false, nil
	}

	blob, err :=
		encodeRabbitVRFDKGLifecycleV1(expected)
	if err != nil {
		return nil, false, err
	}

	if err := l.db.Put(key, blob); err != nil {
		return nil, false, err
	}

	return expected, true, nil
}
