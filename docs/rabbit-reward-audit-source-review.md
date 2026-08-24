# Rabbit Chain — Reward Audit and Static Review

Updated: 2026-08-06
Scope: `consensus/lqc`, Reward Locker, issuance, transactions, fork recovery,
and engine consolidation.

## Evidence completed before consolidation

The 20-producer lab passed:

- 273 of 273 reward blocks, with `327.6 RAB` expected and observed;
- a total issuance difference of `0 wei`;
- the 70/30 split, committee, locker, and vesting index;
- 125 of 125 EIP-1559 transactions across five blocks and 20 of 20 nodes;
- tips, base fee, burn, balances, and invalid-transaction rejections;
- shutdown and return of seven producers;
- shutdown and restart of all 20 nodes while preserving the chain;
- deterministic fork recovery;
- 120 boundary checks for locking, releases, and halvings.

This evidence was produced while the factory still started `consensus/lqcv2`.
It validated the shared monetary flow, but also revealed that the active engine
was not the canonical implementation selected for the project.

## Canonical consolidation

`eth/ethconfig.CreateConsensusEngine` now instantiates `consensus/lqc` for
every genesis with `config.lqc`. The `consensus/lqcv2` implementation remains
in the repository only as a legacy implementation and does not participate in
the active chain.

Before activation, `lqc` was consolidated with the already-approved fixes:

- `core/vesting.CreditReward` and `ReleaseAllUnlockedRewards` in the common
  finalization flow;
- EIP-158 protection for the locker system account;
- local producer resolution using the next block's queue;
- the actual producer or fallback position returned to the miner;
- validation of parents present in the same header batch during synchronization;
- fork recovery and preserved downloading of bodies, transactions, and receipts;
- `fallbackCount` and `committeeSize` from genesis used by the engine and
  auditor.

The first halving was kept exactly at block `8,409,600`.
`RabbitChainConfig`, `RabbitDevnetChainConfig`, the Rabbit mainnet genesis,
and the devnet genesis are aligned at this same height.

## Required validation after package installation

A database produced by the previous engine must not be reused as evidence for
the new engine. The consolidated implementation must be compiled and then
validated in a fresh 20-producer lab. The same reward, transaction, load,
node-loss, return, and complete-restart tests must be repeated before the
architecture can be considered approved.

The first lab using the canonical engine revealed an important regression:
`Prepare` replaced the real timestamp with the slot minimum. Because genesis
had a zero timestamp, every fallback window appeared expired, nodes rapidly
produced different forks, and the old script reported a false success.
Preparation now preserves the real timestamp when it already meets the minimum,
a dedicated regression test exists, and the script passes only after checking
connectivity, a maximum one-block height difference, a common hash, and distinct
producers.

## Remaining items

- The terminal reward of `0.15 RAB` continues indefinitely under the Era 3+
  rule.
- Because genesis receives no reward, Era 0 pays blocks 1 through 8,409,599.
  This behavior was confirmed and retained.
- Global release still traverses the vesting index. Its linear cost must be
  measured before claiming support for millions of miners.
- The definitive public registry requires a dedicated audit before the official
  genesis; the bootstrap lab does not prove permissionless admission at public
  scale.
