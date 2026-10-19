package lqc

import (
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const CommitteeClaimBitmapBytesV1 = MaxCommitteeParticipationsV1 / 8

var (
	ErrInvalidCommitteeClaimLedgerV1   = errors.New("invalid lqc committee claim ledger v1")
	ErrCommitteeParticipationClaimedV1 = errors.New("lqc committee participation already claimed v1")
)

var committeeClaimLedgerDomainV1 = []byte("RABBIT-LQC-COMMITTEE-CLAIM-LEDGER-V1")

type CommitteeClaimBitmapV1 struct {
	TargetBlock uint64
	Claimed     [CommitteeClaimBitmapBytesV1]byte
}

// CommitteeClaimLedgerV1 keeps only the eight target blocks that may still be
// claimed. Each block uses one bit per selected committee position.
type CommitteeClaimLedgerV1 struct {
	Windows []CommitteeClaimBitmapV1
}

type committeeClaimLedgerRootPayloadV1 struct {
	Domain  []byte
	Version uint8
	Windows []CommitteeClaimBitmapV1
}

func NewCommitteeClaimLedgerV1() *CommitteeClaimLedgerV1 {
	return &CommitteeClaimLedgerV1{Windows: []CommitteeClaimBitmapV1{}}
}

func (ledger *CommitteeClaimLedgerV1) clone() *CommitteeClaimLedgerV1 {
	if ledger == nil {
		return NewCommitteeClaimLedgerV1()
	}
	return &CommitteeClaimLedgerV1{
		Windows: append([]CommitteeClaimBitmapV1(nil), ledger.Windows...),
	}
}

func committeeClaimBitmapEmptyV1(bitmap [CommitteeClaimBitmapBytesV1]byte) bool {
	for _, value := range bitmap {
		if value != 0 {
			return false
		}
	}
	return true
}

func (ledger *CommitteeClaimLedgerV1) Validate() error {
	if ledger == nil || len(ledger.Windows) > int(CommitteeParticipationClaimWindowV1) {
		return ErrInvalidCommitteeClaimLedgerV1
	}
	var previous uint64
	for index, window := range ledger.Windows {
		if window.TargetBlock == 0 || committeeClaimBitmapEmptyV1(window.Claimed) ||
			(index > 0 && window.TargetBlock <= previous) {
			return ErrInvalidCommitteeClaimLedgerV1
		}
		previous = window.TargetBlock
	}
	return nil
}

func (ledger *CommitteeClaimLedgerV1) Root() (common.Hash, error) {
	if err := ledger.Validate(); err != nil {
		return common.Hash{}, err
	}
	encoded, err := rlp.EncodeToBytes(committeeClaimLedgerRootPayloadV1{
		Domain:  committeeClaimLedgerDomainV1,
		Version: CommitteeParticipationVersionV1,
		Windows: ledger.Windows,
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func committeeClaimBitV1(position uint8) (int, byte) {
	return int(position / 8), byte(1 << (position % 8))
}

func (ledger *CommitteeClaimLedgerV1) IsClaimed(
	targetBlock uint64,
	position uint8,
) bool {
	if ledger == nil || int(position) >= MaxCommitteeParticipationsV1 {
		return false
	}
	byteIndex, mask := committeeClaimBitV1(position)
	for _, window := range ledger.Windows {
		if window.TargetBlock == targetBlock {
			return window.Claimed[byteIndex]&mask != 0
		}
	}
	return false
}

// Apply returns a new ledger. The receiver is never mutated, including when a
// duplicate claim makes the entire block transition invalid.
func (ledger *CommitteeClaimLedgerV1) Apply(
	inclusionBlock uint64,
	groups []CommitteeParticipationClaimGroupV1,
) (*CommitteeClaimLedgerV1, error) {
	if ledger == nil {
		return nil, ErrInvalidCommitteeClaimLedgerV1
	}
	if err := ledger.Validate(); err != nil {
		return nil, err
	}
	canonical, err := CanonicalCommitteeParticipationClaimGroupsV1(inclusionBlock, groups)
	if err != nil {
		return nil, err
	}
	next := ledger.clone()
	kept := next.Windows[:0]
	for _, window := range next.Windows {
		if window.TargetBlock < inclusionBlock &&
			inclusionBlock-window.TargetBlock <= CommitteeParticipationClaimWindowV1 {
			kept = append(kept, window)
		}
	}
	next.Windows = kept

	byTarget := make(map[uint64]int, len(next.Windows)+len(canonical))
	for index := range next.Windows {
		byTarget[next.Windows[index].TargetBlock] = index
	}
	for _, group := range canonical {
		windowIndex, exists := byTarget[group.TargetBlock]
		if !exists {
			next.Windows = append(next.Windows, CommitteeClaimBitmapV1{TargetBlock: group.TargetBlock})
			windowIndex = len(next.Windows) - 1
			byTarget[group.TargetBlock] = windowIndex
		}
		for _, claim := range group.Participations {
			byteIndex, mask := committeeClaimBitV1(claim.Position)
			if next.Windows[windowIndex].Claimed[byteIndex]&mask != 0 {
				return nil, ErrCommitteeParticipationClaimedV1
			}
			next.Windows[windowIndex].Claimed[byteIndex] |= mask
		}
	}
	sort.Slice(next.Windows, func(i, j int) bool {
		return next.Windows[i].TargetBlock < next.Windows[j].TargetBlock
	})
	if err := next.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}
