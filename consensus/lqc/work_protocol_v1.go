package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

const RandomXWorkProtocolVersion uint8 = 1

var (
	ErrInvalidRandomXWorkTicket   = errors.New("invalid lqc randomx work ticket")
	ErrDuplicateRandomXWorkHash   = errors.New("duplicate lqc randomx work hash")
	ErrDuplicateWorkParticipantV1 = errors.New("duplicate lqc work participant v1")
	ErrInvalidWorkSeat            = errors.New("invalid lqc work seat")
	ErrInvalidWorkSeatReward      = errors.New("invalid lqc work seat reward")
)

// RandomXWorkTicketV1 is the minimal wire identity of one future RandomX proof.
//
// The RandomX output itself is deliberately not required in the wire object:
// a verifier will recompute it from the canonical challenge. RandomX
// verification is NOT wired into active consensus by this file.
type RandomXWorkTicketV1 struct {
	Version     uint8
	Epoch       uint64
	Participant common.Address
	Nonce       uint64
}

// VerifiedRandomXWorkTicketV1 is an internal result produced only after a
// future RandomX verifier has recomputed and accepted the proof hash.
type VerifiedRandomXWorkTicketV1 struct {
	Ticket RandomXWorkTicketV1
	Hash   common.Hash
}

// WorkSeatV1 is one unit of consensus eligibility. A canonical work epoch may
// contain at most one seat for each participant.
type WorkSeatV1 struct {
	TicketHash  common.Hash
	Participant common.Address
}

// WorkSeatRewardV1 is an address payout after reward weight has been calculated
// across unique eligible wallets.
type WorkSeatRewardV1 struct {
	Address common.Address
	Amount  *uint256.Int
}

func validateRandomXWorkTicketV1(ticket RandomXWorkTicketV1) error {
	if ticket.Version != RandomXWorkProtocolVersion ||
		ticket.Epoch == 0 ||
		ticket.Participant == (common.Address{}) {
		return ErrInvalidRandomXWorkTicket
	}
	return nil
}

// CanonicalWorkSeatsV1 converts verified RandomX results into a canonical set
// of work seats. Duplicate participants are rejected, never silently removed.
func CanonicalWorkSeatsV1(
	verified []VerifiedRandomXWorkTicketV1,
) ([]WorkSeatV1, error) {
	seats := make([]WorkSeatV1, 0, len(verified))
	seenHashes := make(map[common.Hash]struct{}, len(verified))
	seenParticipants := make(map[common.Address]struct{}, len(verified))

	for _, item := range verified {
		if err := validateRandomXWorkTicketV1(item.Ticket); err != nil {
			return nil, err
		}
		if item.Hash == (common.Hash{}) {
			return nil, ErrInvalidRandomXWorkTicket
		}
		if _, exists := seenHashes[item.Hash]; exists {
			return nil, ErrDuplicateRandomXWorkHash
		}
		seenHashes[item.Hash] = struct{}{}
		if _, exists := seenParticipants[item.Ticket.Participant]; exists {
			return nil, ErrDuplicateWorkParticipantV1
		}
		seenParticipants[item.Ticket.Participant] = struct{}{}

		seats = append(seats, WorkSeatV1{
			TicketHash:  item.Hash,
			Participant: item.Ticket.Participant,
		})
	}

	sort.Slice(seats, func(i, j int) bool {
		if order := bytes.Compare(
			seats[i].TicketHash.Bytes(),
			seats[j].TicketHash.Bytes(),
		); order != 0 {
			return order < 0
		}
		return seats[i].Participant.Cmp(seats[j].Participant) < 0
	})
	return seats, nil
}

// AggregateWorkSeatRewardsV1 divides reward among unique canonical seats.
//
// The integer remainder is assigned to the first canonical seat so every wei
// is conserved deterministically.
func AggregateWorkSeatRewardsV1(
	total *uint256.Int,
	seats []WorkSeatV1,
) ([]WorkSeatRewardV1, error) {
	if total == nil {
		return nil, ErrInvalidWorkSeatReward
	}
	if len(seats) == 0 || total.IsZero() {
		return nil, nil
	}

	seenParticipants := make(map[common.Address]struct{}, len(seats))
	for _, seat := range seats {
		if seat.TicketHash == (common.Hash{}) ||
			seat.Participant == (common.Address{}) {
			return nil, ErrInvalidWorkSeat
		}
		if _, exists := seenParticipants[seat.Participant]; exists {
			return nil, ErrDuplicateWorkParticipantV1
		}
		seenParticipants[seat.Participant] = struct{}{}
	}

	perSeat := new(uint256.Int).Set(total)
	perSeat.Div(perSeat, uint256.NewInt(uint64(len(seats))))

	allocated := new(uint256.Int).Set(perSeat)
	allocated.Mul(allocated, uint256.NewInt(uint64(len(seats))))

	remainder := new(uint256.Int).Set(total)
	remainder.Sub(remainder, allocated)

	amounts := make(map[common.Address]*uint256.Int)
	for index, seat := range seats {
		amount := new(uint256.Int).Set(perSeat)
		if index == 0 {
			amount.Add(amount, remainder)
		}

		current := amounts[seat.Participant]
		if current == nil {
			current = uint256.NewInt(0)
			amounts[seat.Participant] = current
		}
		current.Add(current, amount)
	}

	addresses := make([]common.Address, 0, len(amounts))
	for address := range amounts {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Cmp(addresses[j]) < 0
	})

	out := make([]WorkSeatRewardV1, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, WorkSeatRewardV1{
			Address: address,
			Amount:  new(uint256.Int).Set(amounts[address]),
		})
	}
	return out, nil
}

// WorkControllerRewardTotalV1 is a test/lab helper for comparing one address
// against a controller that splits the same seats across many addresses.
// It is deliberately not part of active consensus wiring.
func WorkControllerRewardTotalV1(
	rewards []WorkSeatRewardV1,
	addresses map[common.Address]struct{},
) *uint256.Int {
	total := uint256.NewInt(0)
	for _, reward := range rewards {
		if _, ok := addresses[reward.Address]; ok && reward.Amount != nil {
			total.Add(total, reward.Amount)
		}
	}
	return total
}

// keep math/big in this foundation because the future RandomX challenge will
// bind the chain ID without depending on mutable process-local state.
var _ *big.Int
