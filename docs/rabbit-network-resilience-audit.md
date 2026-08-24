# Rabbit Chain — Lab Resilience Audit

The `scripts/rabbit-devnet/run-network-resilience-audit.sh` script tests the
initialized lab without changing the official genesis files or consensus.

It performs these steps in order:

1. verifies initial convergence of all 20 nodes;
2. submits three sequential EIP-1559 transactions;
3. shuts down seven producers;
4. produces blocks and submits a transaction with 13 producers online;
5. restarts and synchronizes the seven producers;
6. submits a transaction after their return;
7. cleanly shuts down all 20 nodes;
8. restarts the same databases and validates the previous checkpoint;
9. resumes block production and submits another transaction;
10. performs a final reward and Reward Locker audit.

On any error or interruption, the exit handler attempts to restart and reconnect
all 20 nodes. The result is saved in `audit-reports` and packaged as a single
`rabbit-network-resilience-result-*.tar.gz` archive.

The test is exclusive to the `/tmp/rabbit-20nodes` lab. Its accounts, test net
balance, and runtime genesis remain outside the official Rabbit Chain
configuration.

Since version 1.0.1, the test also validates fork recovery through the client's
full downloader. Recovery finds the common ancestor and transfers complete
blocks, including transactions and receipts; it does not create empty blocks
from remote headers.
