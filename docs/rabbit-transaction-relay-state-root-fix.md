# Rabbit Chain — Transaction Relay and State Root Fix

## Lab evidence

The 20-producer lab continued advancing normally, and every node maintained 19
peers. A correctly signed EIP-1559 transfer was accepted into node 20's
transaction pool but did not appear in the other 19 pools. When node 20 tried to
assemble a block containing the transaction, the client itself rejected the
block with `invalid merkle root`. Every canonical block remained empty.

## Cause 1: transaction relay disabled

The LQC production loop imports blocks directly and does not complete the
traditional Ethereum downloader cycle. Consequently, the ETH protocol kept
`AcceptTxs` disabled and discarded transactions received from peers.

`eth/backend.go` now waits for a recent LQC header different from genesis
before marking post-synchronization services as ready. This enables transaction
reception and propagation without accepting traffic while the database is empty
or an old chain remains stalled.

## Cause 2: different fee recipients

The LQC engine records the canonical producer in `header.Coinbase`. A fallback
node may assemble the block using a different local address. Before the fix, the
builder's EVM credited the priority fee to the local address, while block
re-execution credited it to the header producer. The two states ended with
different roots.

`miner/worker.go` now uses `header.Coinbase` as the fee recipient whenever
the chain has an LQC configuration. Networks without LQC retain the original
geth behavior.

## Regression tests

- `miner/rabbit_lqc_fee_recipient_test.go` validates the LQC recipient and
  preserves behavior for other networks.
- `eth/rabbit_lqc_sync_test.go` validates that genesis, an old header, and an
  invalid future timestamp do not enable relay; a recent LQC header enables it.
- Legacy mocks in `miner/miner_test.go` and `miner/payload_building_test.go`
  now implement `AccountManager()`, which was already part of the production
  interface, allowing the entire `miner` package to compile during tests.

This fix did not change any file in `consensus/lqc`, `consensus/lqcv2`,
`core/vesting`, or `networks`.
