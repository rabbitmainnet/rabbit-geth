# Rabbit Chain Transaction Auditor

Integration test exclusively for the Rabbit lab. It locally signs an EIP-1559
transfer using node 20's encrypted keystore, submits the raw transaction through
IPC, and verifies:

- inclusion and canonical receipt;
- sender value and nonce;
- recipient balance;
- total gas cost;
- burned base fee;
- tip credited to the block producer;
- block hash and receipt on the selected nodes.

The transaction delta is isolated using `prestateTracer`. Consequently,
immediate producer and committee rewards credited in the same block are not
confused with the transfer value, gas, burn, or tip.

By default, all 20 nodes are verified. The `--verify-nodes` option accepts a
list such as `1,3,4,20` to audit a transaction during a controlled test in
which the remaining nodes are offline.

The `scripts/rabbit-devnet/run-transaction-audit.sh` script runs this program
and then the reward auditor to separate transaction effects, liquid rewards,
and the Reward Locker.

The node 20 account receives 1,000 RAB only in the temporary
`genesis-runtime.json` created by the lab script.
`networks/rabbit-devnet/genesis.json` and the mainnet genesis are not changed.

The `scripts/rabbit-devnet/run-network-resilience-audit.sh` script uses this
option to test transactions during a partial producer outage, after the
producers return, and after a complete restart of all 20 nodes.
