package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

var errLQCWorkTicketTransportUnavailable = errors.New("lqc work ticket laboratory transport is unavailable")

// LQCWorkTicketTransportAPI is registered only when the explicit laboratory
// transport gate has constructed a transport for a non-mainnet genesis.
type LQCWorkTicketTransportAPI struct {
	transport *lqcWorkTicketTransport
}

type WorkTicketArgs struct {
	Version     hexutil.Uint64 `json:"version"`
	Epoch       hexutil.Uint64 `json:"epoch"`
	Anchor      common.Hash    `json:"anchor"`
	Participant common.Address `json:"participant"`
	Sequence    hexutil.Uint64 `json:"sequence"`
	Previous    common.Hash    `json:"previous"`
	Proof       common.Hash    `json:"proof"`
	Signature   hexutil.Bytes  `json:"signature"`
}

type WorkTicketResult struct {
	Hash common.Hash `json:"hash"`
	WorkTicketArgs
}

func newLQCWorkTicketTransportAPI(transport *lqcWorkTicketTransport) *LQCWorkTicketTransportAPI {
	return &LQCWorkTicketTransportAPI{transport: transport}
}

func (api *LQCWorkTicketTransportAPI) SubmitWorkTicket(args WorkTicketArgs) (common.Hash, error) {
	if api == nil || api.transport == nil {
		return common.Hash{}, errLQCWorkTicketTransportUnavailable
	}
	if uint64(args.Version) > uint64(^uint8(0)) {
		return common.Hash{}, lqc.ErrInvalidWorkTicketVersion
	}
	ticket := lqc.WorkTicket{
		Version:     uint8(args.Version),
		Epoch:       uint64(args.Epoch),
		Anchor:      args.Anchor,
		Participant: args.Participant,
		Sequence:    uint64(args.Sequence),
		Previous:    args.Previous,
		Proof:       args.Proof,
		Signature:   append([]byte(nil), args.Signature...),
	}
	return api.transport.Submit(ticket)
}

func (api *LQCWorkTicketTransportAPI) WorkTicketPoolStatus() (lqc.WorkTicketPoolStatus, error) {
	if api == nil || api.transport == nil {
		return lqc.WorkTicketPoolStatus{}, errLQCWorkTicketTransportUnavailable
	}
	return api.transport.pool.Status(), nil
}

func (api *LQCWorkTicketTransportAPI) PendingWorkTickets(limit hexutil.Uint64) ([]WorkTicketResult, error) {
	if api == nil || api.transport == nil {
		return nil, errLQCWorkTicketTransportUnavailable
	}
	wanted := uint64(limit)
	if wanted == 0 {
		wanted = uint64(MaxWorkTicketsPerPacket)
	}
	if wanted > uint64(lqcWorkTicketInitialSyncLimit) {
		wanted = uint64(lqcWorkTicketInitialSyncLimit)
	}
	tickets := api.transport.pool.All(int(wanted))
	results := make([]WorkTicketResult, len(tickets))
	for index, ticket := range tickets {
		results[index] = WorkTicketResult{
			Hash: lqc.WorkTicketHash(api.transport.chainID, ticket),
			WorkTicketArgs: WorkTicketArgs{
				Version:     hexutil.Uint64(ticket.Version),
				Epoch:       hexutil.Uint64(ticket.Epoch),
				Anchor:      ticket.Anchor,
				Participant: ticket.Participant,
				Sequence:    hexutil.Uint64(ticket.Sequence),
				Previous:    ticket.Previous,
				Proof:       ticket.Proof,
				Signature:   append(hexutil.Bytes(nil), ticket.Signature...),
			},
		}
	}
	return results, nil
}
