# Rabbit Reward Auditor 1.4.0

This version changes only the auditing tool. No line from `consensus/lqc` and
no byte from the frozen genesis is included in the package.

## Diagnosis of report 20260811-183144

- The transaction, partial-outage, return, full-shutdown, and restart tests for
  all 21 nodes passed.
- The old auditor still expected rewards from blocks 1 through 100,000 to be
  locked even though the active rule is immediate liquid credit.
- The old auditor initially knew only the 20 bootstrap participants. Node 21 was
  added to the observed list only after producing its first block.
- At blocks 98 and 99, a committee share of `0.18 RAB` was credited to node 21
  before its first production. The two shares total exactly `0.36 RAB`, which
  is the difference shown in the old report.
- Per-wallet differences also came from reconstructing the committee with the
  static bootstrap list instead of the canonical registry.

## Fixes

1. The registry is reconstructed from `registryProtocolBlock` using header
   envelopes and `RegistrySnapshot.ApplyHeader`.
2. REGISTER, HEARTBEAT, EXIT, missed turns, jail status, the queue, and
   `registryRoot` are validated at every height.
3. A registered address becomes observed in the REGISTER block itself, before
   any committee payment.
4. The committee uses `committeeSize` when explicitly set, or the dynamic 10%
   rule with the `committeeMin` and `committeeMax` limits.
5. Every mining reward is modeled as immediately liquid; legacy vesting storage
   must remain empty and unchanged.

## Expected result

In the current lab, the new auditor must count `1.20 RAB` per block, obtain a
total difference of `0 wei`, and exactly reconstruct the producer and committee
recipients. After this passes, the signature auditor traverses every canonical
block.
