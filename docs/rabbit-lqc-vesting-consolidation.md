# Rabbit Chain — Vesting Consolidation in consensus/lqc

Patch version: 1.0.0
Scope: Reward Locker and `consensus/lqc` finalization.

## Changes

- `lqc.distributeRewards` uses `core/vesting.CreditReward` directly;
- `lqc.Finalize` runs `core/vesting.ReleaseAllUnlockedRewards` before rewards;
- `FinalizeAndAssemble` reuses `Finalize`, avoiding different production and
  import paths;
- `consensus/lqc/rewardlocker.go` no longer maintains separate state and
  forwards the legacy API to `core/vesting`;
- the factory continues to use `consensus/lqcv2` during this stage.

## Added tests

- boundaries of all four eras;
- 70% for the producer and 30% for the configured committee;
- 100% for the producer when no committee exists;
- conservation of the remainder and every wei;
- creation and persistence of the canonical vesting index under EIP-158;
- compatibility of the legacy locker API;
- release performed by `Finalize` with `vm.StateDB` wrapped by tracing.

## Installation and validation

The lab may continue running because the active engine is still `lqcv2`.
After extracting the package, run:

```bash
go test ./core/vesting ./consensus/lqc ./consensus/lqcv2
```

Do not rebuild or restart the lab before evaluating these test results. The
factory switch will be a separate patch and will occur only after this
validation.
