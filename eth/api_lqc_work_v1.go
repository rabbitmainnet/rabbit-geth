package eth

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

var errLQCWorkV1APIUnavailable = errors.New(
	"lqc Work V1 laboratory transport unavailable",
)

type LQCWorkV1API struct {
	transport *lqcWorkV1Transport
}

type LQCWorkV1ContextResult struct {
	Epoch           hexutil.Uint64             `json:"epoch"`
	DatasetAnchor   common.Hash                `json:"datasetAnchor"`
	ChallengeAnchor common.Hash                `json:"challengeAnchor"`
	Difficulty      *hexutil.Big               `json:"difficulty"`
	Pool            lqc.WorkCommitPoolStatusV1 `json:"pool"`
}

type LQCWorkV1CandidateArgs struct {
	Version     hexutil.Uint64 `json:"version"`
	Epoch       hexutil.Uint64 `json:"epoch"`
	Participant common.Address `json:"participant"`
	Nonce       hexutil.Uint64 `json:"nonce"`
	ProofHash   common.Hash    `json:"proofHash"`
	Signature   hexutil.Bytes  `json:"signature"`
}

type LQCWorkV2ParticipantStatusResult struct {
	Participant    common.Address `json:"participant"`
	SelectionEpoch hexutil.Uint64 `json:"selectionEpoch"`
	SeatCount      hexutil.Uint64 `json:"seatCount"`
	ActiveSeat     bool           `json:"activeSeat"`
	Committed      bool           `json:"committed"`
	LocalPool      bool           `json:"localPool"`
}

func newLQCWorkV1API(
	transport *lqcWorkV1Transport,
) *LQCWorkV1API {
	return &LQCWorkV1API{transport: transport}
}

func (api *LQCWorkV1API) WorkV1Context() (
	LQCWorkV1ContextResult,
	error,
) {
	if api == nil || api.transport == nil {
		return LQCWorkV1ContextResult{}, errLQCWorkV1APIUnavailable
	}
	ctx, err := api.transport.currentContext()
	if err != nil {
		return LQCWorkV1ContextResult{}, err
	}

	difficulty := new(hexutil.Big)
	(*big.Int)(difficulty).Set(ctx.Difficulty)

	return LQCWorkV1ContextResult{
		Epoch:           hexutil.Uint64(ctx.Epoch),
		DatasetAnchor:   ctx.DatasetAnchor,
		ChallengeAnchor: ctx.ChallengeAnchor,
		Difficulty:      difficulty,
		Pool:            api.transport.PoolStatus(),
	}, nil
}

func (api *LQCWorkV1API) SubmitWorkV1Candidate(
	args LQCWorkV1CandidateArgs,
) (common.Hash, error) {
	if api == nil || api.transport == nil {
		return common.Hash{}, errLQCWorkV1APIUnavailable
	}
	if uint64(args.Version) > 255 {
		return common.Hash{}, errors.New("invalid Work V1 version")
	}

	return api.transport.Submit(
		lqc.WorkCommitCandidateV1{
			Signed: lqc.SignedRandomXWorkTicketV1{
				Ticket: lqc.RandomXWorkTicketV1{
					Version:     uint8(args.Version),
					Epoch:       uint64(args.Epoch),
					Participant: args.Participant,
					Nonce:       uint64(args.Nonce),
				},
				Signature: append([]byte(nil), args.Signature...),
			},
			ProofHash: args.ProofHash,
		},
	)
}

func (api *LQCWorkV1API) WorkV1PoolStatus() (
	lqc.WorkCommitPoolStatusV1,
	error,
) {
	if api == nil || api.transport == nil {
		return lqc.WorkCommitPoolStatusV1{},
			errLQCWorkV1APIUnavailable
	}
	return api.transport.PoolStatus(), nil
}

func (api *LQCWorkV1API) PendingWorkV1Candidates() (
	[]lqc.WorkCommitCandidateV1,
	error,
) {
	if api == nil || api.transport == nil {
		return nil, errLQCWorkV1APIUnavailable
	}
	return api.transport.Pending()
}

// WorkV2ParticipantStatus distinguishes local relay acceptance, canonical
// commitment and an active persistent seat. This is the authoritative status
// endpoint for Rabbit Core and operators.
func (api *LQCWorkV1API) WorkV2ParticipantStatus(
	participant common.Address,
) (LQCWorkV2ParticipantStatusResult, error) {
	if api == nil || api.transport == nil || api.transport.seatStatus == nil ||
		participant == (common.Address{}) {
		return LQCWorkV2ParticipantStatusResult{}, errLQCWorkV1APIUnavailable
	}
	selectionEpoch, seatCount, active, committed, err :=
		api.transport.seatStatus(participant)
	if err != nil {
		return LQCWorkV2ParticipantStatusResult{}, err
	}
	localPool := false
	pending, err := api.transport.Pending()
	if err != nil {
		return LQCWorkV2ParticipantStatusResult{}, err
	}
	for _, candidate := range pending {
		if candidate.Signed.Ticket.Participant == participant {
			localPool = true
			break
		}
	}
	return LQCWorkV2ParticipantStatusResult{
		Participant:    participant,
		SelectionEpoch: hexutil.Uint64(selectionEpoch),
		SeatCount:      hexutil.Uint64(seatCount),
		ActiveSeat:     active,
		Committed:      committed,
		LocalPool:      localPool,
	}, nil
}
