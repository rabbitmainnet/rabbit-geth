# Rabbit Chain — EIP-158 Reward Locker Deletion Fix

Date: 2026-08-05
Live evidence: blocks 1–39 of the restarted chain using the correct binary.
Fix scope: `core/vesting`; no monetary rule was changed.

## Confirmed symptom

The auditor expected 46.8 RAB across 39 blocks, but observed zero in the liquid
balance, Reward Locker, original locked balance, and vesting index. The node 1
executable was the new binary and contained `LQCV2 REWARD`, eliminating the
possibility of an old build.

## Cause

The Reward Locker writes storage at system address
`0x0000000000000000000000000000000000001001`. This address does not exist in
genesis and was created with a zero balance, zero nonce, and empty code.

The devnet activates EIP-158 at block zero. For empty-account cleanup, geth
considers only nonce, balance, and code; storage does not make an account
non-empty. Consequently, `IntermediateRoot(true)` deleted the system account
and all newly written storage at the end of every block.

## Minimal fix

Before the first insertion into the vesting index, the code ensures an internal
nonce of `1` for the system account. A nonce does not create RAB or change
rewards, supply, the 70/30 split, the queue, or the schedule. It only prevents
EIP-158 cleanup from classifying the account as empty.

The `TestLockedRewardSurvivesEIP158Finalization` test credits 1.2 RAB at block
1, runs `IntermediateRoot(true)`, and requires the locked balance, original
balance, index, and recipient to remain present.

## Next validation

Compile the client, restart the temporary lab, and run the auditor from block 1.
The expected result per block is 0.84 RAB locked for the producer and 0.072 RAB
locked for each of the five committee members, totaling exactly 1.2 RAB.
