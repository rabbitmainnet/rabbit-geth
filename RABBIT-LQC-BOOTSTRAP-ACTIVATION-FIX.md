# Rabbit Chain — Permissionless Bootstrap Activation 1.0.0

## Observed failure

The lab started 21 nodes and established all peer connections, but remained at
height zero. The 20 bootstrap producers were classified outside the queue for
block 1.

## Cause

`activationDelay` protects participants that enter through a `REGISTER`
operation. The genesis bootstrap participants were also receiving this delay.
With the protocol activated at block 1 and a delay of 2, the chain had no
eligible producer able to create the first block.

## Fix

Bootstrap participants are canonically identified by sequence zero. Signed
operations can never use sequence zero. Genesis bootstrap participants are
therefore immediately eligible, while participants admitted through `REGISTER`
continue to follow `N + 1 + activationDelay`.

## Scope

- Does not change the block reward.
- Does not change the committee or the 70/30 split.
- Does not change the reward locker, vesting, releases, or halving.
- Does not remove LightHash, signatures, sequences, heartbeat, or `EXIT`.
- Adds deterministic regressions for activation at block 1 with a delay of 2.
