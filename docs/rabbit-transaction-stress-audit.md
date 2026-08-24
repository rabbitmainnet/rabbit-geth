# Rabbit Chain — Load, Mempool, and Rejection Audit

The `scripts/rabbit-devnet/run-transaction-stress-audit.sh` runner uses the
already-initialized 20-producer lab. It does not restart the network or modify
genesis or consensus.

The default scenario submits 125 EIP-1559 transfers in five batches. Each batch
is submitted rapidly with consecutive nonces and awaited until canonical
confirmation before the next batch.

The audit verifies every receipt, fee, tip, burn amount, balance, nonce, and
inclusion block. All 20 nodes must have the same sampled blocks and receipts,
and every transaction pool must finish empty.

Finally, it submits four transactions that must be rejected:

1. replay of an already-mined transaction;
2. stale nonce;
3. value exceeding the account balance;
4. incorrect chain ID.

After the load test, the professional reward auditor traverses the entire chain
again to confirm that transactions and fees did not interfere with the Reward
Locker or scheduled issuance.
