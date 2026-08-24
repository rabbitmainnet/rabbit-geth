# Rabbit LQC — Sequential Ticket Research v1

Status: mathematical research model. Not implemented in consensus.

## Result of the previous prototype

The `rabbit-lowend-accessibility-benchmark/1.0.2` benchmark separated continuous execution from isolated operations. The current use of `argon2.IDKey` showed severe allocation/GC pauses and instability at larger memory sizes. The prototype was rejected and no parameter was selected.

## Requirement that remains

Creating additional addresses without additional work must not increase producer, fallback, or committee selection. A low-end PC must be able to produce one valid unit of work without a GPU, stake, or RAB balance.

No permissionless mechanism can prove that two keys belong to the same person. The technically verifiable goal is one chance per real unit of resource, not one chance per human.

## Sequential model

A lane executes a sequence bound to the canonical challenge and produces at most one eligible ticket per epoch. Splitting the same lane among 1 or 5,000 identities creates no additional tickets. Parallel production requires real additional lanes.

A future implementation may investigate a Verifiable Delay Function. The [original work on VDFs](https://eprint.iacr.org/2018/601) defines functions that require a number of sequential steps to produce a unique output with efficient public verification, and cites leader election as an application. This reference does not constitute an approved implementation.

## Fundamental limit

Sequential work neutralizes free identities, but not real resources. An attacker with 64 lanes against 20 honest lanes controls approximately 76% of selection. Against 1,000 honest lanes, the same 64 represent approximately 6%.

Security therefore also depends on a broad honest base. Mainnet cannot treat 20 processes on one computer as 20 independent participants.

## Alternatives examined

- **RandomX:** the official implementation reports 2,080 MiB for fast mining mode; light mode uses 256 MiB but is significantly slower and intended for verification. This conflicts with the 4 GiB low-end profile.
- **Cuckoo Cycle:** the official project describes strongly memory-bound work and instant verification. It remains a research candidate but still requires benchmarking of RAM, miners, and specialized hardware.
- **Argon2:** RFC 9106 covers proof-of-work applications, but the measured Go implementation did not pass the stability gate. An implementation with reusable memory or Argon2d would require new code, official vectors, and independent review.
- **VDF/sequential work:** conceptually better aligned with a single lane and efficient verification, but cryptographically more complex and not to be written from scratch without an audited implementation.

## Gates before any integration

1. Simulate fixed identities and increasing resources.
2. Demonstrate affordable cost on a real low-end PC.
3. Use a recognized cryptographic implementation and public vectors.
4. Audit uniqueness, sequentiality, replay, grinding, and parallelization.
5. Limit proofs per block and the cost of invalid inputs.
6. Repeat the Sybil attack, committee capture, reorganization, and resilience tests.
7. Run a public testnet with independent computers and operators.
8. Keep mainnet blocked until external review.

