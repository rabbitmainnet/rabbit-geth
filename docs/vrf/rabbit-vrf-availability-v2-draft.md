# Rabbit VRF availability V2 — development draft

This draft is NOT a release and has no public activation height. Public genesis,
WorkSeats, mining selection/rewards, existing DKG sessions and contract bytecode
are unchanged. The default new configuration field is zero (disabled).

## Evidence

Public session 3b2795ff60eeeabd9f575c7f586bda278b47872c0183c0de08ad4997d49b5bbe
had 32 canonical members, threshold 22 and only four polynomial commitments.
The 28 absent members included inactive/missing registry entries and nominally
active entries whose last heartbeat was tens of thousands of blocks old.
Windows persistence was already fixed in 56db31a4fcfe3a93297fb633ccb41a6cb14167e8.

## Integration in this patch

- `vrfAvailabilityV2Block` is an optional future preparation-boundary fork.
- A ceremony whose preparation started before the fork remains V1 throughout.
- V2 freezes availability at preparationStartBlock - 1, on the requested branch.
- The runtime loads the historical canonical registry, not local peer presence.
- The registry commitment is validated; the candidate set, reference number/hash,
  registry root, source snapshot root, heartbeat window, cap and entropy are
  committed into the seed. Lifecycle and replay retain that exact seed/session.
- Candidates require a persistent WorkSeat plus active registry status, no active
  jail and a nonzero heartbeat no older than HeartbeatWindow + HeartbeatGrace.
- No candidate produces no new keyset/session, rather than a fabricated fallback.
- Nonempty sets of 1..3 are supported algebraically without Byzantine fault
  tolerance. One participant supplies no independent distributed randomness.
- Committee sizing uses the existing dynamic policy with minimum target 32,
  clamped to available population and hard cap 128. WorkSeats are not removed.

## Unresolved production requirements

This patch does NOT implement qualified-dealer DKG, complaint resolution,
reliable broadcast, or a consensus certificate for excluding a dealer during an
already frozen session. The existing DKG still requires every selected dealer.
Do not accept the first locally seen threshold of commitments: nodes could
form different keys. Failure during DKG must fail closed. A later epoch has a
new session and fresh canonical availability; this helps when an absent dealer
stops heartbeating, but does not guarantee liveness against a dealer that keeps
heartbeating while withholding evaluations. That requires a separately designed
and reviewed qualification/timeout protocol.

Pending request retry remains existing code and must be tested end-to-end over
failed ceremonies, later successful keysets, reorgs and restarts. This patch
has not proved on-chain fulfillment, callback delivery, fair mining, committee
reward settlement, million-node performance or deployment compatibility.

## Validation

No Go toolchain/full repository is available in the assistant execution host.
Run the included apply script in the full recovery checkout. It formats changed
files and runs targeted params, crypto and production-tagged consensus/eth tests.
A passing result is development validation, NOT release approval. Complete the
repository AGENTS.md pre-commit checks before any commit/release. Do not schedule
activation or update site downloads until the unresolved requirements are handled.
