# Rabbit Chain — Basic LQC Header Security 1.0.1

This stage closes only the missing basic header validations in the
`consensus/lqc` consensus implementation. It does not change Rabbit Chain
economics or unblock mainnet.

## Added rules

- reject blocks more than 30 seconds in the future;
- safely reject overflow when calculating the producer or fallback slot;
- reject block numbers that do not fit in `uint64`;
- constrain `gasLimit` and bind it to the parent block's `gasLimit`;
- never allow `gasUsed` to exceed `gasLimit`;
- require `baseFee` and calculate it according to EIP-1559 when London is active;
- prohibit `baseFee` before London;
- reject Shanghai, Cancun, and Prague fields while those forks remain
  unimplemented and unaudited in LQC;
- treat genesis as the hash-committed starting point, preserving the frozen
  Rabbit mainnet `extraData`.

## Unchanged behavior

- immediate mining rewards;
- producer and committee split;
- halvings and eras;
- permissionless queue and registry;
- LightHash;
- mainnet genesis.

## Remaining blocker

Blocks still need a verifiable cryptographic signature from the selected
producer. Until this signature is implemented, tested across multiple nodes,
and audited, Rabbit mainnet remains blocked from launch.

## Validation

Run only:

```bash
cd "$HOME/projects/rabbit-geth" && chmod +x ./scripts/rabbit-devnet/validate-lqc-header-security.sh && ./scripts/rabbit-devnet/validate-lqc-header-security.sh
```

The expected result for this stage is `BASIC HEADER SECURITY: PASS`, followed
by the warning `MAINNET: DO NOT LAUNCH YET`.

## Fix 1.0.1

The expanded suite found two old tests that were incompatible with rules already
present in the code:

- one fixture constructed a header with a zero `gasLimit`; it now inherits the
  parent block's `gasLimit`, as a valid block must;
- the RPC test still expected a fixed 64-block validity period even though the
  protocol advertises and uses `MaxRegistryOperationLifetime = 256`; the
  expectation is now calculated directly from the canonical parameter.

These two changes affect tests only. No header validation was removed or
weakened.
