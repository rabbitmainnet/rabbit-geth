# Rabbit Chain Monetary Boundary Audit

The `scripts/rabbit-devnet/run-reward-boundary-audit.sh` runner validates
monetary boundary heights without producing millions of blocks. Each scenario
creates an isolated StateDB and directly calls the real vesting and `Finalize`
implementations.

Coverage:

- locked block 100,000 and liquid block 100,001;
- 25%, 50%, 75%, and 100% releases at blocks 3,253,600, 4,042,000,
  4,830,400, and 5,618,800;
- the block before, at, and after each of the three halvings;
- parity between `consensus/lqc` and `consensus/lqcv2`;
- 70% producer reward, 30% committee reward, and the no-reward fallback;
- remainder conservation with a seven-member committee;
- idempotency, late catch-up, and global release;
- exact cumulative issuance in wei;
- Reward Locker persistence after EIP-158 finalization;
- comparison between configured and observed selection sizes.

The auditor does not modify genesis files, databases, or consensus code.
