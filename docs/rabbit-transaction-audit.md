# Rabbit Chain — Transaction Lab and Audit

This package adds an integration test without changing consensus or modifying
the official genesis files.

## Why the lab needs a liquid balance

Rewards from blocks 1 through 100,000 are correctly sent to the Reward Locker.
Therefore, producer accounts do not have liquid RAB to pay for a transaction
during a short lab run. The lab script creates a temporary genesis copy at
`/tmp/rabbit-20nodes/genesis-runtime.json` and grants 1,000 RAB to the node 20
account only on this disposable network.

The `networks/rabbit-devnet/genesis.json` and
`networks/rabbit-mainnet/genesis.json` files are not changed. The previous lab
is also moved to a backup directory before reinitialization.

## Validated behavior

Using the lab's encrypted keystore, `cmd/rabbit-tx-audit` locally signs an
EIP-1559 transfer of 1 RAB from node 20 to node 2 and submits only the signed
transaction through IPC. It then validates the following against the historical
state of node 1:

- successful receipt and inclusion in the canonical block;
- sender nonce and exact debit;
- exact recipient credit;
- gas used and effective fee;
- the burned portion of the base fee;
- priority fee credited to the correct producer;
- interaction with the locked rewards from the same block;
- the same block and receipt observed by all 20 nodes.

The script then runs the reward auditor again through the chain tip. The JSON
and Markdown reports and reward-auditor data are collected in a single
`rabbit-transaction-audit-result-*.tar.gz` archive.

## Execution

First compile and run the module tests. Then restart the lab with
`scripts/rabbit-devnet/start-rabbit-20producers.sh` and run
`scripts/rabbit-devnet/run-transaction-audit.sh`.

This test is a baseline for the currently active engine. Switching from
`lqcv2` to `lqc` is not part of this package and must occur only after the
LQC readiness audit.
