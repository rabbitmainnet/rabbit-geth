package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	WorkTicketEnvelopeVersion uint8 = 1
	MaxWorkTicketEnvelopeSize       = 16 * 1024
)

var (
	workTicketEnvelopeMagic = []byte{'L', 'Q', 'T', 0}

	ErrInvalidWorkTicketEnvelope     = errors.New("invalid lqc work ticket envelope")
	ErrUnsupportedWorkTicketEnvelope = errors.New("unsupported lqc work ticket envelope version")
	ErrWorkTicketBlockMismatch       = errors.New("lqc work ticket envelope block mismatch")
	ErrWorkTicketEpochMismatch       = errors.New("lqc work ticket envelope epoch mismatch")
	ErrInvalidWorkTicketStateRoot    = errors.New("invalid lqc work ticket state root")
	ErrDuplicateWorkTicket           = errors.New("duplicate lqc work ticket")
)

// WorkTicketEnvelope is the standalone canonical representation used by the
// storage foundation. It is deliberately not embedded in an active header.
type WorkTicketEnvelope struct {
	Version     uint8
	BlockNumber uint64
	Epoch       uint64
	Anchor      common.Hash
	StateRoot   common.Hash
	Tickets     []WorkTicket
}

func IsWorkTicketEnvelope(blob []byte) bool {
	return len(blob) >= len(workTicketEnvelopeMagic) && bytes.Equal(blob[:len(workTicketEnvelopeMagic)], workTicketEnvelopeMagic)
}

func validateWorkTicketEnvelopeTickets(tickets []WorkTicket) error {
	if len(tickets) > MaxWorkTicketsPerBlock {
		return ErrTooManyWorkTickets
	}
	for index, ticket := range tickets {
		if err := validateWorkTicketStructure(ticket, true); err != nil {
			return fmt.Errorf("ticket %d: %w", index, err)
		}
		if index > 0 && tickets[index-1].Participant == ticket.Participant && tickets[index-1].Sequence == ticket.Sequence {
			return fmt.Errorf("ticket %d: %w", index, ErrDuplicateWorkTicket)
		}
	}
	return nil
}

// EncodeWorkTicketEnvelope canonicalizes tickets before producing bounded RLP.
func EncodeWorkTicketEnvelope(blockNumber, epoch uint64, anchor, stateRoot common.Hash, tickets []WorkTicket) ([]byte, error) {
	if blockNumber == 0 || epoch == 0 || anchor == (common.Hash{}) || stateRoot == (common.Hash{}) {
		return nil, ErrInvalidWorkTicketEnvelope
	}
	canonical := CanonicalWorkTickets(tickets)
	if err := validateWorkTicketEnvelopeTickets(canonical); err != nil {
		return nil, err
	}
	envelope := WorkTicketEnvelope{
		Version:     WorkTicketEnvelopeVersion,
		BlockNumber: blockNumber,
		Epoch:       epoch,
		Anchor:      anchor,
		StateRoot:   stateRoot,
		Tickets:     canonical,
	}
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkTicketEnvelope, err)
	}
	blob := make([]byte, 0, len(workTicketEnvelopeMagic)+len(payload))
	blob = append(blob, workTicketEnvelopeMagic...)
	blob = append(blob, payload...)
	if len(blob) > MaxWorkTicketEnvelopeSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrInvalidWorkTicketEnvelope, len(blob), MaxWorkTicketEnvelopeSize)
	}
	return blob, nil
}

// DecodeWorkTicketEnvelope rejects non-canonical and oversized encodings.
func DecodeWorkTicketEnvelope(blob []byte) (WorkTicketEnvelope, error) {
	if !IsWorkTicketEnvelope(blob) || len(blob) > MaxWorkTicketEnvelopeSize {
		return WorkTicketEnvelope{}, ErrInvalidWorkTicketEnvelope
	}
	var envelope WorkTicketEnvelope
	if err := rlp.DecodeBytes(blob[len(workTicketEnvelopeMagic):], &envelope); err != nil {
		return WorkTicketEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidWorkTicketEnvelope, err)
	}
	if envelope.Version != WorkTicketEnvelopeVersion {
		return WorkTicketEnvelope{}, ErrUnsupportedWorkTicketEnvelope
	}
	if envelope.BlockNumber == 0 || envelope.Epoch == 0 || envelope.Anchor == (common.Hash{}) || envelope.StateRoot == (common.Hash{}) {
		return WorkTicketEnvelope{}, ErrInvalidWorkTicketEnvelope
	}
	if err := validateWorkTicketEnvelopeTickets(envelope.Tickets); err != nil {
		return WorkTicketEnvelope{}, err
	}
	canonical := CanonicalWorkTickets(envelope.Tickets)
	if !workTicketsEqual(canonical, envelope.Tickets) {
		return WorkTicketEnvelope{}, ErrNonCanonicalWorkTickets
	}
	reencoded, err := EncodeWorkTicketEnvelope(envelope.BlockNumber, envelope.Epoch, envelope.Anchor, envelope.StateRoot, envelope.Tickets)
	if err != nil || !bytes.Equal(reencoded, blob) {
		return WorkTicketEnvelope{}, ErrInvalidWorkTicketEnvelope
	}
	return envelope, nil
}

// ValidateWorkTicketEnvelope performs context, cryptography, lane and state
// root checks without mutating the supplied states.
func ValidateWorkTicketEnvelope(chainID *big.Int, blockNumber, epoch uint64, anchor common.Hash, states map[common.Address]WorkTicketLaneState, blob []byte) (WorkTicketEnvelope, map[common.Address]WorkTicketLaneState, error) {
	envelope, err := DecodeWorkTicketEnvelope(blob)
	if err != nil {
		return WorkTicketEnvelope{}, nil, err
	}
	if envelope.BlockNumber != blockNumber {
		return WorkTicketEnvelope{}, nil, ErrWorkTicketBlockMismatch
	}
	if envelope.Epoch != epoch {
		return WorkTicketEnvelope{}, nil, ErrWorkTicketEpochMismatch
	}
	if envelope.Anchor != anchor {
		return WorkTicketEnvelope{}, nil, ErrInvalidWorkTicketAnchor
	}
	next, err := ValidateWorkTicketBatch(chainID, anchor, epoch, states, envelope.Tickets)
	if err != nil {
		return WorkTicketEnvelope{}, nil, err
	}
	root, err := WorkTicketStateRoot(chainID, anchor, epoch, next)
	if err != nil {
		return WorkTicketEnvelope{}, nil, err
	}
	if root != envelope.StateRoot {
		return WorkTicketEnvelope{}, nil, ErrInvalidWorkTicketStateRoot
	}
	return envelope, next, nil
}
