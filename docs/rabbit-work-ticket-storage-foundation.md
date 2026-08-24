# Rabbit LQC — Ticket Codec, Pool, and Snapshots

Status: **STORAGE_FOUNDATION_ONLY — mainnet blocked**

This stage adds deterministic storage and validation for sequential tickets. It
does not place tickets in the active header, change producer, fallback, or
committee selection, or modify genesis.

## Canonical envelope

- Binary prefix and explicit version.
- Canonical RLP, at most 16 KiB.
- Up to 64 tickets per batch.
- Commitment to block, epoch, anchor, and post-batch root.
- Ordering independent of network arrival.
- Rejection of duplicates, alternative encodings, and false roots.

## Separate pool

- Does not use the transaction pool or EVM.
- Global capacity of 4,096 tickets.
- Maximum of 64 pending tickets per participant.
- Proof and signature verified before retention.
- Round-based local selection: one deep lane does not occupy the batch before
  other continuous lanes.
- The pool is never the consensus source of truth.

## Snapshots

- Indexed by block hash, isolating forks and reorganizations.
- Root bound to chain ID, epoch, anchor, and the state of every lane.
- Deterministic reconstruction by a new node.
- New participants receive a canonical initial predecessor.
- A departing participant's lane is preserved to prevent reset and replay, but
  tickets from an inactive participant are rejected.
- Persistence is only a cache; corruption and non-canonical encoding are
  rejected.

## Honest limitations

- The envelope is not yet part of the active LQC header.
- Ticket RPC and gossip do not yet exist.
- Work-based selection does not yet exist.
- Epoch and anchor transitions will be defined with lab activation.
- Anti-censorship inclusion with thousands of participants remains pending.
- The current selection's Sybil vulnerability remains until integration and
  repetition of the offensive audits.

The frozen genesis remains byte-for-byte identical.
