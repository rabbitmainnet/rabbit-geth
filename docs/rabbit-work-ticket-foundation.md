# Rabbit LQC — Portable Sequential Ticket Foundation

Status: **FOUNDATION_ONLY — mainnet blocked**

This stage defines and tests the portable cryptographic proof. It does not
modify producer selection, fallbacks, the committee, rewards, headers, or
snapshots.

## Candidate parameters

- Algorithm: Argon2id v1.3 from `golang.org/x/crypto/argon2`.
- Memory: 8 MiB per proof.
- Iterations: 1.
- Internal proof parallelism: 1.
- Output: 32 bytes.
- Candidate maximum: 64 tickets per block.
- Independent verification: at most 2 workers and 16 MiB in use concurrently.

## Security bindings

Each proof commits to:

- Rabbit LQC domain and version;
- chain ID;
- epoch and anchor hash;
- participant address;
- lane sequence;
- the previous proof from the same lane.

The complete ticket is signed using recoverable low-S secp256k1. This prevents
replay across networks, epochs, and anchors; copying a proof to another
identity; and skipping a sequence. Batch validation is canonical, bounded, and
atomic.

## Honest limitations of this stage

- Argon2id is not a cryptographic VDF.
- Additional hardware still produces proportionally more work.
- There is no guarantee of one person per address without external identity.
- The rule that converts tickets into producers, fallbacks, and committee
  members is not yet implemented; therefore, the active engine's Sybil
  vulnerability still exists.
- The pure Go reference is portable but still allocates memory for each proof.
  The reusable optimization must preserve byte-for-byte equivalence.

## Next gate

Integrate the ticket pool, codec, and snapshots in a lab-only activation. Only
then repeat Sybil attacks with up to 100,000 identities, forks, restarts, load,
reward, and signature tests.

The frozen genesis remains byte-for-byte identical.
