# Rabbit Chain — Canonical Permissionless Registry

Specification version: `1.3.0-draft`

Status: engine, pool, RPC, and gossip implemented behind explicit block
activation; disabled by default and not yet enabled in the lab genesis.

## Goals

- anyone can request admission without owning or locking RAB;
- no administrator wallet approves or removes participants;
- every node derives the same queue from the canonical chain;
- a new node reconstructs the registry from headers alone;
- registration, heartbeat, and exit are verifiable without a smart contract;
- every active participant retains equal weight in the queue.

## Registration

A candidate creates a `REGISTER` operation containing a version, address,
sequence, validity limit, and proof nonce. The candidate searches for a nonce
whose Keccak-256 hash is below the target defined by `proofDifficulty`, then
signs the operation with its own secp256k1 key.

The proof exists only to limit registration spam. It does not increase weight,
priority, or reward. No token deposit is required.

## Canonical inclusion

Operations are propagated through a dedicated LQC pool, without fees or the
EVM. An active producer includes a limited set of operations in the header's
`Extra` field. The header also contains the resulting `registryRoot`. Every node:

1. verifies the signature, sequence, validity, and LightHash;
2. applies the operations to the parent header snapshot;
3. recalculates the `registryRoot`;
4. rejects the block if any byte differs.

An entry included in block `N` participates in selection only from
`N + 1 + activationDelay`. The producer of a block can therefore never
authorize itself in that same block.

## Canonical header format

The binary envelope begins with the four bytes `LQC\\x00`, followed by
canonical RLP containing:

1. envelope version (`2`);
2. block number;
3. post-block `registryRoot`;
4. list of signed operations.

The codec limits `Extra` to 16 KiB and accepts at most 64 operations per block.
Operations are ordered by address and sequence using deterministic tie-breakers.
A duplicate address/sequence pair, zero root, incorrectly sized signature,
non-canonical order, malformed RLP, or unknown version is rejected.
Cryptographic validation uses the `chainId`, height, and LightHash difficulty.
The snapshot layer verifies `registryRoot` before the engine accepts a header.

The in-memory pool accepts at most 4,096 operations and retains one pending
operation per address. A higher sequence replaces the previous operation only
after it is valid against the current canonical snapshot. Expired operations
are pruned. Included operations may remain retained until expiry to tolerate a
reorganization, but they never return to a header without being revalidated
against the new parent.

The `lqc_submitRegistryOperation` RPC accepts only operations that are already
signed; the node never receives a private key and never signs for the user.
Status and pending-operation queries use the same namespace. The separate
`lqcr/1` P2P protocol verifies version, network ID, and genesis before
propagating bounded batches. Each peer maintains a bounded set of known hashes
to stop gossip loops.

## Heartbeat and exit

- producing a block automatically updates the heartbeat;
- a participant that has not yet produced sends a signed `HEARTBEAT` operation;
- `EXIT` is signed by the participant itself;
- inactivity beyond `heartbeatWindow + heartbeatGrace` automatically removes
  the participant from the queue without deleting its history;
- re-entry requires a new sequence and a new LightHash proof.

## Reconstruction, reorganization, and checkpoints

The registry is a snapshot derived from headers. Snapshots are indexed by block
hash, never by height alone. A reorganization selects the snapshot belonging to
the new parent. Periodic checkpoints may be stored locally to accelerate
synchronization, but the local cache is never a source of consensus truth.

Each snapshot stores the height, block hash, registry root, and participants in
canonical order. When applying a child, the node verifies continuity, producer
eligibility in the parent snapshot, the envelope, operations, and post-block
root. Block production updates the heartbeat before applying operations. A new
node begins reconstruction from the deterministic transition base snapshot and
reapplies canonical headers.

## Engine activation

The `registryProtocolBlock` field controls the transition. A zero value retains
the legacy format. Before the configured height, the engine requires `LQC:1:N`;
from that height onward, it requires the binary envelope and derives selection,
committee, and heartbeat from the parent snapshot. The canonical path does not
consult `RuntimeRegistry`, does not require a bond, and has no fallback that
automatically authorizes the header coinbase.

At the activation boundary, the base snapshot is created deterministically
from the configured bootstrap participants and the parent hash. Complete local
checkpoints are written only every `epochLength`; other recent snapshots remain
in a bounded LRU cache. A missing or invalid cache is reconstructed from
headers.

An activated configuration without LightHash difficulty or a valid bootstrap
set is rejected. After either conflicting height has been reached, a node also
rejects a local change to `registryProtocolBlock`, preventing a fork caused by
a late genesis/configuration change.

## Mandatory limits

- maximum `Extra` size;
- maximum operations per block;
- maximum future validity of an operation;
- one operation per address and sequence;
- canonical operation ordering before encoding;
- signature domain includes the `chainId`;
- difficulty can never be zero.

## Genesis

An empty chain cannot safely select its first producer. Before mainnet, there
will be a public genesis ceremony: candidates publish LightHash proofs, the
list is ordered by a verifiable rule, and the result is inserted into the
genesis. After block zero, the registry remains permanently open and no key has
administrative power.

## Integration phases

1. mathematical core and isolated tests — completed;
2. versioned envelope codec and `registryRoot` — completed, not activated;
3. hash-indexed snapshots and header-based reconstruction — completed, not activated;
4. controlled engine integration — completed behind activation;
5. operation pool/gossip/RPC — implemented behind activation and under validation;
6. lab with bootstrap participants plus unknown producer 21;
7. exit, expiry, return, reorganization, and new-node synchronization;
8. repeat reward, resilience, load, and boundary tests.
