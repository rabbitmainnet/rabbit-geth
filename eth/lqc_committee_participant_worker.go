//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/log"
)

func startLQCCommitteeParticipantWorker(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
	engine *lqc.LQC,
) error {
	if backend == nil || backend.blockchain == nil || backend.accountManager == nil ||
		transport == nil || engine == nil || transport.claimDone != nil {
		return errLQCWorkV1Context
	}
	headCh := make(chan core.ChainHeadEvent, 16)
	sub := backend.blockchain.SubscribeChainHeadEvent(headCh)
	transport.claimDone = make(chan struct{})
	go func() {
		defer close(transport.claimDone)
		defer sub.Unsubscribe()
		var last common.Hash
		process := func() {
			header := backend.blockchain.CurrentHeader()
			if header == nil || header.Number == nil || !header.Number.IsUint64() ||
				header.Hash() == last {
				return
			}
			last = header.Hash()
			if err := produceLocalLQCCommitteeClaims(
				backend, transport, engine, header.Number.Uint64(), header.Hash(),
			); err != nil && !errors.Is(err, lqc.ErrInvalidCommitteeParticipationVerificationV1) {
				log.Debug("LQC committee participation skipped", "block", header.Number, "err", err)
			}
		}
		process()
		for {
			select {
			case <-transport.claimStop:
				return
			case <-sub.Err():
				return
			case <-headCh:
				process()
			}
		}
	}()
	return nil
}

func produceLocalLQCCommitteeClaims(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
	engine *lqc.LQC,
	blockNumber uint64,
	blockHash common.Hash,
) error {
	ctx, err := engine.WorkV1EngineLabCommitteeContext(
		backend.blockchain, blockNumber, blockHash,
	)
	if err != nil {
		return err
	}
	local := make(map[common.Address]accounts.Wallet)
	for _, wallet := range backend.accountManager.Wallets() {
		for _, account := range wallet.Accounts() {
			local[account.Address] = wallet
		}
	}
	for position, seat := range ctx.Committee {
		wallet := local[seat.Participant]
		if wallet == nil {
			continue
		}
		proof := lqc.CommitteeParticipationV1{
			Version:       lqc.CommitteeParticipationVersionV1,
			BlockNumber:   blockNumber,
			ParentHash:    ctx.ParentHash,
			SelectionRoot: ctx.SelectionRoot,
			TicketHash:    seat.TicketHash,
			Participant:   seat.Participant,
		}
		input, err := lqc.CommitteeParticipationInputV1(ctx.ChainID, proof)
		if err != nil {
			return err
		}
		proofHash, err := ctx.Hasher(ctx.DatasetKey, input)
		if err != nil {
			return err
		}
		signingData, err := lqc.CommitteeParticipationSigningDataV1(
			ctx.ChainID, proof, proofHash,
		)
		if err != nil {
			return err
		}
		account := accounts.Account{Address: seat.Participant}
		proof.Signature, err = wallet.SignData(
			account, accounts.MimetypeClique, signingData,
		)
		if err != nil {
			// Locked/external/unavailable wallets do not participate and receive
			// no committee reward. Other local seats remain independent.
			log.Debug("LQC committee wallet did not sign", "participant", seat.Participant, "err", err)
			continue
		}
		if err := lqc.VerifyCommitteeParticipationV1(
			ctx.ChainID, seat, proof, proofHash,
		); err != nil {
			return err
		}
		group := lqc.CommitteeParticipationClaimGroupV1{
			TargetBlock: blockNumber,
			Participations: []lqc.CompactCommitteeParticipationV1{{
				Position: uint8(position), Signature: append([]byte(nil), proof.Signature...),
			}},
		}
		if err := transport.SubmitCommitteeClaim(group); err != nil &&
			!errors.Is(err, lqc.ErrDuplicateCommitteeParticipationClaimV1) {
			return err
		}
	}
	return nil
}
