// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
)

// ethHandler implements the eth.Backend interface to handle the various network
// packets that are sent as replies or broadcasts.
type ethHandler handler

func (h *ethHandler) Chain() *core.BlockChain { return h.chain }
func (h *ethHandler) TxPool() eth.TxPool      { return h.txpool }
func (h *ethHandler) BlobPool() eth.BlobPool  { return h.blobpool }

// RunPeer is invoked when a peer joins on the `eth` protocol.
func (h *ethHandler) RunPeer(peer *eth.Peer, hand eth.Handler) error {
	return (*handler)(h).runEthPeer(peer, hand)
}

// PeerInfo retrieves all known `eth` information about a peer.
func (h *ethHandler) PeerInfo(id enode.ID) interface{} {
	if p := h.peers.peer(id.String()); p != nil {
		return p.info()
	}
	return nil
}

// AcceptTxs retrieves whether transaction processing is enabled on the node
// or if inbound transactions should simply be dropped.
func (h *ethHandler) AcceptTxs() bool {
	return h.synced.Load()
}

// Handle is invoked from a peer's message handler when it receives a new remote
// message that the handler couldn't consume and serve itself.
func (h *ethHandler) Handle(peer *eth.Peer, packet eth.Packet) error {
	// Consume any broadcasts and announces, forwarding the rest to the downloader
	switch packet := packet.(type) {
	case *eth.NewPooledTransactionHashesPacket72:
		hashes, err := h.txFetcher.Notify(peer.ID(), packet.Types, packet.Sizes, packet.Hashes)
		if err != nil {
			return err
		}
		if len(hashes) != 0 {
			return h.blobFetcher.Notify(peer.ID(), hashes, packet.Mask)
		}
		return nil

	case *eth.NewPooledTransactionHashesPacket71:
		_, err := h.txFetcher.Notify(peer.ID(), packet.Types, packet.Sizes, packet.Hashes)
		return err

	case *eth.TransactionsPacket:
		txs, err := packet.Items()
		if err != nil {
			return fmt.Errorf("Transactions: %v", err)
		}
		if err := handleTransactions(peer, txs, true); err != nil {
			return fmt.Errorf("Transactions: %v", err)
		}
		return h.txFetcher.Enqueue(peer.ID(), peer.Version(), txs, false)

	case *eth.PooledTransactionsPacket:
		txs, err := packet.List.Items()
		if err != nil {
			return fmt.Errorf("PooledTransactions: %v", err)
		}
		if err := handleTransactions(peer, txs, false); err != nil {
			return fmt.Errorf("PooledTransactions: %v", err)
		}
		return h.txFetcher.Enqueue(peer.ID(), peer.Version(), txs, true)

	case *eth.CellsResponse:
		outer, err := packet.Cells.Items()
		if err != nil {
			return fmt.Errorf("Cells: %v", err)
		}
		cells := make([][]kzg4844.Cell, len(outer))
		for i := range outer {
			if outer[i].Len() > params.BlobTxMaxBlobs*kzg4844.CellsPerBlob {
				return fmt.Errorf("Cells: cells per tx exceeded the possible maximum")
			}
			if cells[i], err = outer[i].Items(); err != nil {
				return fmt.Errorf("Cells: %v", err)
			}
		}
		return h.blobFetcher.Enqueue(peer.ID(), packet.Hashes, cells, packet.Mask)

	case *eth.NewBlockPacket:
		if packet == nil || packet.Block == nil {
			return nil
		}
		if cfg := h.chain.Config(); cfg != nil && cfg.LQC != nil {
			head := h.chain.CurrentBlock()
			localNumber := uint64(0)
			if head != nil && head.Number != nil {
				localNumber = head.Number.Uint64()
			}
			peer.Log().Info("LQC live NewBlock received",
				"local", localNumber,
				"remote", packet.Block.NumberU64(),
				"hash", packet.Block.Hash(),
			)

			if packet.Block.NumberU64() == localNumber && head != nil &&
				shouldStartLQCSync(head, packet.Block.NumberU64(), packet.Block.Hash()) {
				peer.Log().Info("LQC same-height NewBlock applying deterministic fork choice",
					"local", localNumber,
					"localHash", head.Hash(),
					"remoteHash", packet.Block.Hash(),
				)
				if _, err := h.chain.InsertChain(types.Blocks{packet.Block}); err != nil {
					peer.Log().Warn("LQC same-height NewBlock direct import failed; scheduling fork recovery",
						"number", packet.Block.NumberU64(),
						"hash", packet.Block.Hash(),
						"err", err,
					)
					h.scheduleLQCSync(peer, packet.Block.Header())
					return nil
				}
				return nil
			}
			if packet.Block.NumberU64() <= localNumber {
				return nil
			}
			if head != nil && packet.Block.NumberU64() == localNumber+1 && packet.Block.ParentHash() != head.Hash() {
				peer.Log().Warn("LQC live NewBlock parent mismatch; scheduling fork recovery",
					"local", localNumber,
					"localHash", head.Hash(),
					"remote", packet.Block.NumberU64(),
					"remoteHash", packet.Block.Hash(),
				)
				h.scheduleLQCSync(peer, packet.Block.Header())
				return nil
			}
			if packet.Block.NumberU64() > localNumber+1 {
				// The complete block already supplies the recovery target header.
				// Do not depend on a separate range announcement/header response:
				// that response may be delayed behind live imports on this peer.
				// Normal downloader validation still checks the missing ancestors.
				h.scheduleLQCSync(peer, packet.Block.Header())
				return nil
			}
			if _, err := h.chain.InsertChain(types.Blocks{packet.Block}); err != nil {
				if rabbitQueueFutureLiveBlock(h, packet.Block, err) {
					return nil
				}
				peer.Log().Warn("LQC live NewBlock insert failed",
					"number", packet.Block.NumberU64(),
					"hash", packet.Block.Hash(),
					"err", err,
				)
			}
		}
		return nil

	case *eth.BlockRangeUpdatePacket:
		if cfg := h.chain.Config(); cfg != nil && cfg.LQC != nil {
			localHead := h.chain.CurrentBlock()
			localNumber := uint64(0)
			if localHead != nil && localHead.Number != nil {
				localNumber = localHead.Number.Uint64()
			}
			peer.Log().Info("LQC live block range received",
				"local", localNumber,
				"remote", packet.LatestBlock,
				"hash", packet.LatestBlockHash,
			)
			if !shouldStartLQCSync(localHead, packet.LatestBlock, packet.LatestBlockHash) {
				return nil
			}
			// Claim synchronously, before launching the goroutine. A burst of
			// identical range updates from one peer must not spawn multiple
			// concurrent requests. Other peers remain available for failover.
			if !h.claimLQCHeaderRequest(peer.ID(), packet.LatestBlock, packet.LatestBlockHash) {
				return nil
			}
			h.wakeLQCSync()
			go h.requestAndSyncLQCHead(peer, packet.LatestBlock, packet.LatestBlockHash)
		}
		return nil

	default:
		return fmt.Errorf("unexpected eth packet type: %T", packet)
	}
}

// shouldStartLQCSync selects a strictly higher remote chain. If both chains
// have the same height and different heads, the lower hash is the deterministic
// tie-breaker so every node makes the same choice.
func shouldStartLQCSync(local *types.Header, remoteNumber uint64, remoteHash common.Hash) bool {
	if remoteHash == (common.Hash{}) {
		return false
	}
	if local == nil || local.Number == nil {
		return remoteNumber > 0
	}
	localNumber := local.Number.Uint64()
	if remoteNumber > localNumber {
		return true
	}
	if remoteNumber < localNumber {
		return false
	}
	localHash := local.Hash()
	if remoteHash == localHash {
		return false
	}
	return bytes.Compare(remoteHash[:], localHash[:]) < 0
}

type lqcSyncTarget struct {
	peer   *eth.Peer
	header *types.Header
	peerID string // Source of the validated header, for bounded alternate lookup.
}

func betterLQCSyncTarget(candidate, current *lqcSyncTarget) bool {
	if candidate == nil || candidate.header == nil || candidate.header.Number == nil {
		return false
	}
	if current == nil || current.header == nil || current.header.Number == nil {
		return true
	}
	candidateNumber := candidate.header.Number.Uint64()
	currentNumber := current.header.Number.Uint64()
	if candidateNumber != currentNumber {
		return candidateNumber > currentNumber
	}
	candidateHash := candidate.header.Hash()
	currentHash := current.header.Hash()
	return candidateHash != currentHash && bytes.Compare(candidateHash[:], currentHash[:]) < 0
}

func lqcPeerRangeContainsTarget(peerRange *eth.BlockRangeUpdatePacket, targetNumber uint64, targetHash common.Hash) bool {
	if peerRange == nil {
		return true
	}
	if peerRange.LatestBlock != targetNumber {
		return peerRange.LatestBlock > targetNumber
	}
	return peerRange.LatestBlockHash == targetHash
}

// lqcSyncAlreadyHasTarget reports whether a validated recovery target is already
// queued or being processed. It deliberately compares the full hash and height:
// a competing fork at the same height must never be suppressed.
func lqcSyncAlreadyHasTarget(current, pending *lqcSyncTarget, number uint64, hash common.Hash) bool {
	for _, target := range []*lqcSyncTarget{current, pending} {
		if target != nil && target.header != nil && target.header.Number != nil &&
			target.header.Number.IsUint64() && target.header.Number.Uint64() == number &&
			target.header.Hash() == hash {
			return true
		}
	}
	return false
}

func (h *ethHandler) lqcSyncAlreadyHasTarget(number uint64, hash common.Hash) bool {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	return lqcSyncAlreadyHasTarget(h.lqcSyncCurrent, h.lqcSyncPending, number, hash)
}

// lqcHeaderBackupNeeded permits a single independent header lookup while the
// exact head is being recovered. Previously the generic duplicate check also
// suppressed every other peer, so BlockRange-only peers could never become
// pending alternatives. Do not start a backup for missing source IDs, an
// existing pending target, another fork, or a different height.
func lqcHeaderBackupNeeded(current, pending *lqcSyncTarget, peerID string, number uint64, hash common.Hash) bool {
	if current == nil || pending != nil || current.peerID == "" || peerID == "" ||
		current.peerID == peerID || current.header == nil || current.header.Number == nil ||
		!current.header.Number.IsUint64() || current.header.Number.Uint64() != number {
		return false
	}
	return current.header.Hash() == hash
}

// lqcHeaderAlreadyCoveredForPeer is used while a claimed request waits for
// a reply. An accepted alternate must not cancel itself merely because the
// primary is now in the sync queue.
func (h *ethHandler) lqcHeaderAlreadyCoveredForPeer(peerID string, number uint64, hash common.Hash) bool {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	if !lqcSyncAlreadyHasTarget(h.lqcSyncCurrent, h.lqcSyncPending, number, hash) {
		return false
	}
	return !lqcHeaderBackupNeeded(h.lqcSyncCurrent, h.lqcSyncPending, peerID, number, hash)
}

// lqcHeaderRequestKey identifies a single peer's in-flight hash lookup.
// Do not deduplicate across peers: one silent peer must never block failover.
type lqcHeaderRequestKey struct {
	peerID string
	number uint64
	hash   common.Hash
}

func (h *ethHandler) claimLQCHeaderRequest(peerID string, number uint64, hash common.Hash) bool {
	if peerID == "" || hash == (common.Hash{}) {
		return false
	}
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	if lqcSyncAlreadyHasTarget(h.lqcSyncCurrent, h.lqcSyncPending, number, hash) {
		if !lqcHeaderBackupNeeded(h.lqcSyncCurrent, h.lqcSyncPending, peerID, number, hash) {
			return false
		}
		// Do not fan out one additional lookup per peer; exactly one
		// backup request may be outstanding for the current head.
		for inFlight := range h.lqcHeaderRequests {
			if inFlight.number == number && inFlight.hash == hash && inFlight.peerID != h.lqcSyncCurrent.peerID {
				return false
			}
		}
	}
	key := lqcHeaderRequestKey{peerID: peerID, number: number, hash: hash}
	if h.lqcHeaderRequests == nil {
		h.lqcHeaderRequests = make(map[lqcHeaderRequestKey]struct{})
	}
	if _, exists := h.lqcHeaderRequests[key]; exists {
		return false
	}
	h.lqcHeaderRequests[key] = struct{}{}
	return true
}

func (h *ethHandler) releaseLQCHeaderRequest(peerID string, number uint64, hash common.Hash) {
	h.lqcSyncMu.Lock()
	delete(h.lqcHeaderRequests, lqcHeaderRequestKey{peerID: peerID, number: number, hash: hash})
	h.lqcSyncMu.Unlock()
}

func (h *ethHandler) requestAndSyncLQCHead(peer *eth.Peer, number uint64, hash common.Hash) {
	defer h.releaseLQCHeaderRequest(peer.ID(), number, hash)
	// A queued NewBlock makes same-peer lookups redundant, but one
	// independent peer may still supply a backup for a stalled recovery.
	if h.lqcHeaderAlreadyCoveredForPeer(peer.ID(), number, hash) {
		return
	}
	resCh := make(chan *eth.Response, 1)
	req, err := peer.RequestHeadersByHash(hash, 1, 0, false, resCh)
	if err != nil {
		peer.Log().Warn("LQC fork-recovery target request failed", "target", number, "hash", hash, "err", err)
		return
	}
	defer req.Close()
	// Retire an outstanding request as soon as its target is no longer needed.
	// This also avoids logging a timeout for a block imported by another peer.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var res *eth.Response
waitHeader:
	for {
		select {
		case res = <-resCh:
			break waitHeader
		case <-ticker.C:
			// Stop this duplicate request if NewBlock already scheduled its target.
			if h.lqcHeaderAlreadyCoveredForPeer(peer.ID(), number, hash) {
				return
			}
			if !shouldStartLQCSync(h.chain.CurrentBlock(), number, hash) {
				peer.Log().Debug("LQC target already reached while awaiting header",
					"target", number, "hash", hash)
				return
			}
		case <-timer.C:
			if h.lqcHeaderAlreadyCoveredForPeer(peer.ID(), number, hash) {
				return
			}
			if shouldStartLQCSync(h.chain.CurrentBlock(), number, hash) {
				peer.Log().Warn("LQC fork-recovery target request timeout",
					"target", number, "hash", hash)
			}
			return
		case <-h.quitSync:
			return
		}
	}
	if res == nil {
		return
	}
	headers, ok := res.Res.(*eth.BlockHeadersRequest)
	if res.Done != nil {
		res.Done <- nil
	}
	if !ok || headers == nil || len(*headers) != 1 || (*headers)[0] == nil {
		peer.Log().Warn("LQC fork-recovery invalid target response", "target", number, "hash", hash)
		return
	}
	header := (*headers)[0]
	if header.Number == nil || header.Number.Uint64() != number || header.Hash() != hash {
		peer.Log().Warn("LQC fork-recovery target mismatch", "wantNumber", number, "haveNumber", header.Number, "wantHash", hash, "haveHash", header.Hash())
		return
	}
	// BlockRangeUpdate normally precedes NewBlock. Give a direct extension a
	// bounded opportunity to finish live import before marking recovery busy.
	if !waitForLQCLiveImport(h.chain.CurrentBlock, header, h.quitSync, 500*time.Millisecond) {
		return
	}
	h.scheduleLQCSync(peer, header)
}

// waitForLQCLiveImport suppresses recovery when normal import reaches the target.
// Gaps and competing parents bypass the grace period; an absent live block still
// falls back to the downloader after the bounded wait.
func waitForLQCLiveImport(current func() *types.Header, target *types.Header, quit <-chan struct{}, grace time.Duration) bool {
	if target == nil || target.Number == nil {
		return false
	}
	local := current()
	if !shouldStartLQCSync(local, target.Number.Uint64(), target.Hash()) {
		return false
	}
	if local == nil || local.Number == nil ||
		target.Number.Uint64() != local.Number.Uint64()+1 || target.ParentHash != local.Hash() {
		return true
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-quit:
			return false
		case <-timer.C:
			local = current()
			return shouldStartLQCSync(local, target.Number.Uint64(), target.Hash())
		case <-ticker.C:
			local = current()
			if !shouldStartLQCSync(local, target.Number.Uint64(), target.Hash()) {
				return false
			}
			if local == nil || local.Number == nil ||
				target.Number.Uint64() != local.Number.Uint64()+1 || target.ParentHash != local.Hash() {
				return true
			}
		}
	}
}

// lqcSameHeadAlternatePeer allows a different peer to back up a stalled
// recovery of the exact same header. It never displaces a pending higher head
// or a competing fork, and never changes consensus validity.
func lqcSameHeadAlternatePeer(candidate, current, pending *lqcSyncTarget) bool {
	if candidate == nil || current == nil || pending != nil ||
		candidate.peer == nil || current.peer == nil || candidate.peer == current.peer ||
		candidate.header == nil || current.header == nil ||
		candidate.header.Number == nil || current.header.Number == nil {
		return false
	}
	return candidate.header.Number.Cmp(current.header.Number) == 0 &&
		candidate.header.Hash() == current.header.Hash()
}

func (h *ethHandler) scheduleLQCSync(peer *eth.Peer, header *types.Header) {
	if peer == nil || header == nil || header.Number == nil ||
		!shouldStartLQCSync(h.chain.CurrentBlock(), header.Number.Uint64(), header.Hash()) {
		return
	}
	candidate := &lqcSyncTarget{peer: peer, header: header, peerID: peer.ID()}

	h.lqcSyncMu.Lock()
	if !h.lqcSyncRunning {
		h.lqcSyncRunning = true
		h.lqcSyncCurrent = candidate
		h.lqcSyncMu.Unlock()
		go h.runLQCSync(candidate)
		return
	}
	// Preserve an independent serving peer for the same validated header.
	// If the current peer stalls or disconnects, the worker can use this
	// pending source rather than restarting without an alternative.
	if lqcSameHeadAlternatePeer(candidate, h.lqcSyncCurrent, h.lqcSyncPending) {
		h.lqcSyncPending = candidate
		h.lqcSyncMu.Unlock()
		return
	}
	if current := h.lqcSyncCurrent; current != nil && current.peer == peer &&
		current.header != nil && current.header.Number != nil &&
		!lqcPeerRangeContainsTarget(peer.BlockRange(), current.header.Number.Uint64(), current.header.Hash()) {
		if h.lqcSyncPending == nil || betterLQCSyncTarget(candidate, h.lqcSyncPending) {
			h.lqcSyncPending = candidate
		}
		h.lqcSyncMu.Unlock()
		h.wakeLQCSync()
		return
	}
	best := h.lqcSyncCurrent
	if betterLQCSyncTarget(h.lqcSyncPending, best) {
		best = h.lqcSyncPending
	}
	if betterLQCSyncTarget(candidate, best) {
		h.lqcSyncPending = candidate
	}
	h.lqcSyncMu.Unlock()
}

func (h *ethHandler) takePendingLQCSyncTarget() *lqcSyncTarget {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	next := h.lqcSyncPending
	h.lqcSyncPending = nil
	h.lqcSyncCurrent = next
	if next == nil {
		h.lqcSyncRunning = false
	}
	return next
}

func (h *ethHandler) wakeLQCSync() {
	select {
	case h.lqcSyncWake <- struct{}{}:
	default:
	}
}

// extendOrReplaceLQCSyncV1 first attempts a compatible extension.
// An incompatible but still-needed head may replace the skeleton target.
// Consensus validation remains the responsibility of the downloader.
func (h *ethHandler) extendOrReplaceLQCSyncV1(next *lqcSyncTarget) error {
	if next == nil || next.peer == nil || next.header == nil || next.header.Number == nil {
		return fmt.Errorf("invalid LQC replacement target")
	}
	err := h.downloader.BeaconExtend(next.header)
	if err == nil {
		return nil
	}
	if !downloader.IsLQCBeaconHeadConflict(err) {
		return err
	}

	number := next.header.Number.Uint64()
	hash := next.header.Hash()
	if !lqcForkRecoveryTargetNeededV1(
		h.chain.CurrentBlock(), next.header, h.chain.GetCanonicalHash(number),
	) || !lqcPeerRangeContainsTarget(next.peer.BlockRange(), number, hash) {
		return err
	}
	next.peer.Log().Info("LQC replacing incompatible recovery target",
		"target", number, "hash", hash, "extensionError", err)
	return h.downloader.BeaconSync(next.header, nil)
}

// lqcRecoveryStallBudget bounds a stalled near-tip fork repair. The two-minute
// budget remains for larger gaps, where full backfill can legitimately take
// longer. This only controls the recovery worker's wait; it never selects or
// accepts a block without downloader and consensus validation.
func lqcRecoveryStallBudget(local, target *types.Header) time.Duration {
	const nearTipBudget = 30 * time.Second
	const backfillBudget = 2 * time.Minute
	if local == nil || local.Number == nil || target == nil || target.Number == nil {
		return backfillBudget
	}
	// Equal-height forks and gaps of up to two blocks should fail over promptly.
	if target.Number.Uint64() <= local.Number.Uint64()+2 {
		return nearTipBudget
	}
	return backfillBudget
}

// lqcBackupNeedsForcedSync permits a forced downloader start only when the
// skeleton explicitly reports that no session has started. Other failures
// must not tear down an existing recovery cycle.
func lqcBackupNeedsForcedSync(err error) bool {
	return errors.Is(err, downloader.ErrLQCBeaconNeedsStart)
}

// lqcWorkerBackupAfterStall limits a same-hash worker handoff to a stalled
// near-tip recovery. The downloader still owns peer choice and validation.
func lqcWorkerBackupAfterStall(local, target *types.Header, stalled time.Duration) bool {
	if local == nil || local.Number == nil || target == nil || target.Number == nil ||
		!local.Number.IsUint64() || !target.Number.IsUint64() || stalled < 8*time.Second {
		return false
	}
	localNumber, targetNumber := local.Number.Uint64(), target.Number.Uint64()
	return targetNumber <= localNumber || targetNumber-localNumber <= 2
}

// takeSameHeadPendingLQCSyncTarget selects only a validated alternative source
// for exactly the current header; a higher head or competing fork is retained.
func (h *ethHandler) takeSameHeadPendingLQCSyncTarget(current *lqcSyncTarget) *lqcSyncTarget {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	pending := h.lqcSyncPending
	if !lqcSameHeadAlternatePeer(pending, current, nil) ||
		!lqcPeerRangeContainsTarget(pending.peer.BlockRange(), pending.header.Number.Uint64(), pending.header.Hash()) {
		return nil
	}
	h.lqcSyncPending = nil
	h.lqcSyncCurrent = pending
	return pending
}

// trySameHeadPendingLQCSyncTarget commits a backup only after the downloader
// accepts its peer preference. An error leaves both queue entries untouched.
// The callback never runs while lqcSyncMu is held.
func (h *ethHandler) trySameHeadPendingLQCSyncTarget(current *lqcSyncTarget, prefer func(*lqcSyncTarget) error) (*lqcSyncTarget, error) {
	h.lqcSyncMu.Lock()
	pending := h.lqcSyncPending
	eligible := h.lqcSyncCurrent == current && lqcSameHeadAlternatePeer(pending, current, nil) &&
		lqcPeerRangeContainsTarget(pending.peer.BlockRange(), pending.header.Number.Uint64(), pending.header.Hash())
	h.lqcSyncMu.Unlock()
	if !eligible {
		return nil, nil
	}
	if err := prefer(pending); err != nil {
		return nil, err
	}
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	// A concurrent higher head or fork may have replaced the pending target.
	// Do not overwrite a newer decision, even if the old hint was accepted.
	if h.lqcSyncCurrent != current || h.lqcSyncPending != pending ||
		!lqcSameHeadAlternatePeer(pending, current, nil) {
		return nil, nil
	}
	h.lqcSyncPending = nil
	h.lqcSyncCurrent = pending
	return pending, nil
}

func (h *ethHandler) runLQCSync(target *lqcSyncTarget) {
	syncEvents := make(chan downloader.SyncEvent, 16)
	subscription := h.downloader.SubscribeSyncEvents(syncEvents)
	defer subscription.Unsubscribe()
	retries := 0
	// One worker-level same-hash handoff per head. Never ping-pong peers.
	backupTried := make(map[common.Hash]bool)
	var backupHash common.Hash
	var backupDeadline time.Time
	for target != nil {
		peer := target.peer
		header := target.header
		if peer == nil || header == nil || header.Number == nil {
			target = h.takePendingLQCSyncTarget()
			continue
		}
		if !shouldStartLQCSync(h.chain.CurrentBlock(), header.Number.Uint64(), header.Hash()) {
			target = h.takePendingLQCSyncTarget()
			continue
		}
		if !lqcPeerRangeContainsTarget(peer.BlockRange(), header.Number.Uint64(), header.Hash()) {
			target = h.takePendingLQCSyncTarget()
			continue
		}
		peer.Log().Info("LQC fork-recovery started", "target", header.Number, "hash", header.Hash())
		// A same-hash alternate source does not change the download target.
		// The skeleton already retries stalled header requests with other peers;
		// forcing BeaconSync here can restart that in-flight recovery cycle.
		// Keep the existing downloader session and original stall deadline.
		if header.Hash() == backupHash && !backupDeadline.IsZero() {
			// Pass the already-range-checked backup to the downloader's
			// scheduler. The hint cannot authorize an invalid block or fork.
			// Peer preference was accepted before the worker committed this backup.
			// Do not send a duplicate hint or accidentally reset its scheduler.
			// An identical head should not tear down an active skeleton session.
			// However, the skeleton might have terminated since the backup was queued.
			// BeaconExtend is non-forcing during an active session; when no session
			// exists it fails, and a forced start is then necessary for recovery.
			if err := h.downloader.BeaconExtend(header); err != nil {
				if !lqcBackupNeedsForcedSync(err) {
					// Do not force a skeleton restart for transient/reorging errors.
					// The active session can continue its own peer handoff.
					peer.Log().Warn("LQC backup extension deferred; preserving downloader session",
						"target", header.Number, "hash", header.Hash(), "err", err)
				} else {
					peer.Log().Info("LQC backup session inactive; starting downloader",
						"target", header.Number, "hash", header.Hash())
					if err = h.downloader.BeaconSync(header, nil); err != nil {
						peer.Log().Warn("LQC backup recovery restart failed", "target", header.Number, "err", err)
						target = h.takePendingLQCSyncTarget()
						continue
					}
				}
			}
		} else if err := h.downloader.BeaconSync(header, nil); err != nil {
			peer.Log().Warn("LQC fork-recovery start failed", "target", header.Number, "hash", header.Hash(), "err", err)
			target = h.takePendingLQCSyncTarget()
			continue
		}

		deadline := time.Now().Add(lqcRecoveryStallBudget(h.chain.CurrentBlock(), header))
		if header.Hash() == backupHash && !backupDeadline.IsZero() && backupDeadline.Before(deadline) {
			deadline = backupDeadline
		}
		var progressNumber uint64
		if local := h.chain.CurrentBlock(); local != nil && local.Number != nil {
			progressNumber = local.Number.Uint64()
		}
		ticker := time.NewTicker(250 * time.Millisecond)
		failed := false
		handedOff := false
		lastProgress := time.Now()
		failedExtension := false
		var failedHash common.Hash
		var failedProgress uint64
	waitRecovery:
		for {
			// Direct block import can resolve the fork after BeaconSync starts.
			// A losing equal-height target can never become canonical while
			// this head is preferred. Retire it and drain the pending target
			// instead of holding all newer recovery requests for two minutes.
			if !lqcForkRecoveryTargetNeededV1(h.chain.CurrentBlock(), header,
				h.chain.GetCanonicalHash(header.Number.Uint64())) {
				break
			}
			// Keep the current backfill alive for direct descendants. BeaconExtend
			// integrates them without cancelling downloads. If the newest queued
			// target requires a fork or gap repair, let the outer loop force that
			// target now instead of ignoring it until the two-minute deadline.

			if next := h.takeBetterPendingLQCSyncTargetV1(target); next != nil {
				nextHash := next.header.Hash()
				if lqcShouldDeferFailedExtensionV1(
					failedExtension, failedHash, nextHash,
					failedProgress, progressNumber,
				) {
					h.restorePendingLQCSyncTargetV1(next)
				} else if err := h.extendOrReplaceLQCSyncV1(next); err != nil {
					h.restorePendingLQCSyncTargetV1(next)
					failedExtension = true
					failedHash = nextHash
					failedProgress = progressNumber
					peer.Log().Info("LQC fork-recovery deferred incompatible extension",
						"from", header.Number, "to", next.header.Number, "reason", err)
				} else {
					failedExtension = false
					target = next
					peer, header = next.peer, next.header
					h.lqcSyncMu.Lock()
					h.lqcSyncCurrent = next
					h.lqcSyncMu.Unlock()
					// A newly selected recovery target needs its own stall budget.
					// Do not inherit an almost-expired deadline from an older fork.
					deadline = time.Now().Add(lqcRecoveryStallBudget(h.chain.CurrentBlock(), header))
				}
			}
			peerRange := peer.BlockRange()
			if !lqcPeerRangeContainsTarget(peerRange, header.Number.Uint64(), header.Hash()) {
				peer.Log().Info("LQC recovery target no longer advertised; ending current wait",
					"target", header.Number, "hash", header.Hash())
				break
			}
			// Keep an advancing import alive. Only a stalled import times out.
			if local := h.chain.CurrentBlock(); local != nil && local.Number != nil {
				if number := local.Number.Uint64(); number > progressNumber {
					lastProgress = time.Now()
					progressNumber = number
					deadline = time.Now().Add(lqcRecoveryStallBudget(local, header))
				}
			}
			// Preserve the existing stall deadline: a same-hash backup must
			// not double the 30s/120s maximum wait. Only one try per hash.
			if !backupTried[header.Hash()] &&
				lqcWorkerBackupAfterStall(h.chain.CurrentBlock(), header, time.Since(lastProgress)) {
				alternate, hintErr := h.trySameHeadPendingLQCSyncTarget(target, func(candidate *lqcSyncTarget) error {
					err := h.downloader.PreferLQCRecoveryPeer(header, candidate.peerID)
					if lqcBackupNeedsForcedSync(err) {
						// Skeleton is idle; a forced start is safe and required before preferring.
						if err = h.downloader.BeaconSync(header, nil); err == nil {
							err = h.downloader.PreferLQCRecoveryPeer(header, candidate.peerID)
						}
					}
					return err
				})
				if hintErr != nil {
					peer.Log().Debug("LQC backup peer preference rejected; leaving backup pending", "err", hintErr)
				}
				if alternate != nil {
					backupTried[header.Hash()] = true
					backupHash, backupDeadline = header.Hash(), deadline
					peer.Log().Warn("LQC worker handing stalled target to alternate source",
						"target", header.Number, "hash", header.Hash(),
						"alternatePeer", alternate.peerID)
					target = alternate
					handedOff = true
					break waitRecovery
				}
			}
			if time.Now().After(deadline) {
				peer.Log().Warn("LQC fork-recovery stalled",
					"target", header.Number, "imported", progressNumber)
				break
			}
			switch lqcWaitRecoverySignal(ticker.C, h.lqcSyncWake, syncEvents, h.quitSync, subscription.Err()) {
			case 1:
				peer.Log().Warn("LQC fork-recovery download failed; retry scheduled",
					"target", header.Number, "hash", header.Hash())
				failed = true
				break waitRecovery
			case 2:
				ticker.Stop()
				h.lqcSyncMu.Lock()
				h.lqcSyncRunning = false
				h.lqcSyncCurrent = nil
				h.lqcSyncPending = nil
				h.lqcSyncMu.Unlock()
				return
			}
		}
		ticker.Stop()
		if handedOff {
			continue
		}
		if failed && retries < 3 {
			retries++
			pause := time.NewTimer(time.Second)
			select {
			case <-pause.C:
			case <-h.quitSync:
				pause.Stop()
				h.lqcSyncMu.Lock()
				h.lqcSyncRunning = false
				h.lqcSyncCurrent = nil
				h.lqcSyncPending = nil
				h.lqcSyncMu.Unlock()
				return
			}
			// Keep ownership of this recovery worker while choosing its next target.
			h.lqcSyncMu.Lock()
			if h.lqcSyncPending != nil {
				target = h.lqcSyncPending
				h.lqcSyncPending = nil
			}
			h.lqcSyncCurrent = target
			h.lqcSyncMu.Unlock()
			continue
		}
		retries = 0
		target = h.takePendingLQCSyncTarget()
	}
}

// handleTransactions marks all given transactions as known to the peer
// and performs basic validations.
func handleTransactions(peer *eth.Peer, list []*types.Transaction, directBroadcast bool) error {
	seen := make(map[common.Hash]struct{}, len(list))
	for _, tx := range list {
		if tx.Type() == types.BlobTxType {
			if directBroadcast {
				return errors.New("disallowed broadcast blob transaction")
			} else {
				// If we receive any blob transactions missing sidecars, or with
				// sidecars that don't correspond to the versioned hashes reported
				// in the header, disconnect from the sending peer.
				if tx.BlobTxSidecar() == nil {
					return errors.New("received sidecar-less blob transaction")
				}
				if err := tx.BlobTxSidecar().ValidateBlobCommitmentHashes(tx.BlobHashes()); err != nil {
					return err
				}
			}
		}

		// Check for duplicates.
		hash := tx.Hash()
		if _, exists := seen[hash]; exists {
			return fmt.Errorf("multiple copies of the same hash %v", hash)
		}
		seen[hash] = struct{}{}

		// Mark as known.
		peer.MarkTransaction(hash)
	}
	return nil
}

// lqcWaitRecoverySignal distinguishes a failed asynchronous download from
// ordinary progress. A failure never authorizes production by itself.
func lqcWaitRecoverySignal(
	tick <-chan time.Time,
	wake <-chan struct{},
	events <-chan downloader.SyncEvent,
	quit <-chan struct{},
	subscriptionErr <-chan error,
) int {
	select {
	case <-tick:
		return 0
	case <-wake:
		return 0
	case event := <-events:
		if event.Type == downloader.SyncFailed {
			return 1
		}
		return 0
	case <-quit:
		return 2
	case <-subscriptionErr:
		return 2
	}
}
