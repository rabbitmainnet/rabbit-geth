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
				return nil
			}
			if _, err := h.chain.InsertChain(types.Blocks{packet.Block}); err != nil {
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
			h.wakeLQCSync()
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

func (h *ethHandler) requestAndSyncLQCHead(peer *eth.Peer, number uint64, hash common.Hash) {
	resCh := make(chan *eth.Response, 1)
	req, err := peer.RequestHeadersByHash(hash, 1, 0, false, resCh)
	if err != nil {
		peer.Log().Warn("LQC fork-recovery target request failed", "target", number, "hash", hash, "err", err)
		return
	}
	defer req.Close()

	var res *eth.Response
	select {
	case res = <-resCh:
	case <-time.After(5 * time.Second):
		peer.Log().Warn("LQC fork-recovery target request timeout", "target", number, "hash", hash)
		return
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
	h.scheduleLQCSync(peer, header)
}

func (h *ethHandler) startLQCBeaconSync(peer *eth.Peer, header *types.Header) {
	h.scheduleLQCSync(peer, header)
}

func (h *ethHandler) scheduleLQCSync(peer *eth.Peer, header *types.Header) {
	if peer == nil || header == nil || header.Number == nil ||
		!shouldStartLQCSync(h.chain.CurrentBlock(), header.Number.Uint64(), header.Hash()) {
		return
	}
	candidate := &lqcSyncTarget{peer: peer, header: header}

	h.lqcSyncMu.Lock()
	if !h.lqcSyncRunning {
		h.lqcSyncRunning = true
		h.lqcSyncCurrent = candidate
		h.lqcSyncMu.Unlock()
		go h.runLQCSync(candidate)
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

func (h *ethHandler) hasBetterPendingLQCSyncTarget(current *lqcSyncTarget) bool {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	return betterLQCSyncTarget(h.lqcSyncPending, current)
}

func (h *ethHandler) wakeLQCSync() {
	select {
	case h.lqcSyncWake <- struct{}{}:
	default:
	}
}

func (h *ethHandler) runLQCSync(target *lqcSyncTarget) {
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
		if err := h.downloader.BeaconSync(header, nil); err != nil {
			peer.Log().Warn("LQC fork-recovery start failed", "target", header.Number, "hash", header.Hash(), "err", err)
			target = h.takePendingLQCSyncTarget()
			continue
		}

		deadline := time.Now().Add(2 * time.Minute)
		ticker := time.NewTicker(250 * time.Millisecond)
		for {
			if h.chain.GetCanonicalHash(header.Number.Uint64()) == header.Hash() {
				break
			}
			peerRange := peer.BlockRange()
			if !lqcPeerRangeContainsTarget(peerRange, header.Number.Uint64(), header.Hash()) {
				if local := h.chain.CurrentBlock(); local != nil && local.Number != nil {
					if err := h.downloader.BeaconSync(local, nil); err != nil {
						peer.Log().Warn("LQC fork-recovery canonical reanchor failed", "number", local.Number, "hash", local.Hash(), "err", err)
					}
				}
				break
			}
			if h.hasBetterPendingLQCSyncTarget(target) || time.Now().After(deadline) {
				break
			}
			select {
			case <-ticker.C:
			case <-h.lqcSyncWake:
			}
		}
		ticker.Stop()
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
