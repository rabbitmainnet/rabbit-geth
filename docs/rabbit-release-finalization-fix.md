# Rabbit Chain — Finalization Parity and Boundary Tests

Date: 2026-08-05
Approved live baseline: 20 blocks, 24 RAB issued and locked, zero difference.

## Problem

In the active `lqcv2` engine, `FinalizeAndAssemble` performed the global
release during production, but `Finalize` did not perform it during import and
validation. At the first release, the producer and validator could derive
different states.

In addition, `Finalize` applied rewards only when `vm.StateDB` was exactly
`*state.StateDB`. When geth wrapped the state with tracing hooks, the type
assertion failed and rewards and releases were silently skipped.

## Fix

- Global release and distribution now occur in `Finalize`, the common path for
  both flows.
- `FinalizeAndAssemble` calls `Finalize` exactly once and calculates the root
  afterward.
- The locker and distributor accept `vm.StateDB`, including the tracing wrapper.
- Monetary rules, the queue, committee, supply, and heights were not changed.

## Added tests

- locked reward at block 100,000;
- liquid reward at block 100,001;
- exact 25%, 50%, 75%, and 100% releases, including the remainder;
- exact halvings at the boundaries of eras 1, 2, and 3;
- release through a StateDB with tracing hooks;
- locker persistence after EIP-158 cleanup.

The release test uses small values with a remainder to prove that the last stage
releases the entire balance and leaves no wei locked.
