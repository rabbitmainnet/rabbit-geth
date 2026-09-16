# Rabbit LQC V4: automatic committee participation

This change completes the local-to-network path for delayed committee claims.
It does not choose or change the activation height.

## Consensus behavior

- A selected local WorkSeat computes exactly one RandomX participation hash for
  the canonical target block.
- The owning wallet signs the canonical payload. A locked, missing, offline or
  otherwise unavailable wallet creates no claim and receives no committee
  reward.
- Claims may be included for eight blocks, are capped at 128 participations per
  block and are checked against the same branch-aware context used by consensus.
- Claims already recorded in canonical V4 headers are not proposed again.
- Reorganizations do not create duplicate payment: canonical history is
  rescanned and old-branch signatures are rejected against the new context.
- Producer/fallback authorization and the established reward rules are not
  changed: authorized producer 70%, verified committee participation from the
  30% pool, missing participation unissued, emergency recovery unsubsidized.

## Network and resource bounds

- `lqcw` is upgraded from protocol version 2 to 3 and adds message code 2 for
  compact committee claim groups.
- The message ceiling is 16 KiB and each packet is capped at 128 proofs.
- The in-memory pool is bounded by 128 positions across each of the eight claim
  window blocks (maximum 1,024 entries before pruning).
- Initial peer synchronization sends pending claims, while normal propagation
  uses per-peer known hashes to avoid repeat broadcast.
- While V4 is active, registry operations are capped at 48 in blocks carrying
  the V4 format, preserving the header capacity established by the V4 codec.

## Operational gate

Keep `ConsensusLivenessV3Block` unset or scheduled in the future until the full
tagged regression suite, restart/reorg tests and a multi-node test have passed.
Every node must upgrade together because `lqcw/3` is intentionally incompatible
with the earlier relay handshake.
