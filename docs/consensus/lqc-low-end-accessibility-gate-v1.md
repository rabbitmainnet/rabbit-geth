# Rabbit LQC — Low-End PC Accessibility Gate v1

Status: benchmark protocol. It is not an algorithm choice and is not implemented in consensus.

## Why this gate exists

The Sybil test confirmed that one position per address lets a controller turn thousands of keys into nearly all producer, fallback, and committee power. The continuous-ticket simulation removed this free gain when total work remained fixed.

That result does not prove the defense is accessible. Before any implementation, Rabbit Chain must measure whether a person with a basic computer can participate without a GPU, stake, RAB balance, or specialized equipment.

## Available guarantee

A permissionless blockchain cannot guarantee "one chance per person" without external identity. IP, MAC, fingerprints, and device serial numbers do not solve this problem: they can be shared, changed, forged, or controlled by VPNs, cloud platforms, and manufacturers.

The intended technical guarantee is:

> Splitting the same work among many identities does not increase the chance; every additional unit of chance requires additional verifiable work.

IP and connection reputation may rate-limit abuse in P2P transport, but can never decide producer, fallback, committee, or reward.

## Reference low-end profile

The first gate uses this conservative reference:

- 2 available CPU cores;
- 4 GiB total RAM;
- no GPU requirement;
- one mining worker;
- at most 128 MiB of proof memory;
- a computer estimated to be four times slower than the lab machine.

This estimate does not replace physical testing. The result remains `PROVISIONAL` until execution on at least three real hardware classes, including the low-end profile.

## Measurable prototype

The benchmark uses Argon2id v1.3 only to measure CPU and memory cost. It does not propose activating Argon2id in consensus. Version 1.0.3 interleaves profiles in ascending and descending order, warms each profile, runs five rounds, and uses p95 for the gates. It also measures isolated operations with memory collection outside the timer. This separates cryptographic instability from allocation or garbage-collection pauses. Large inversions or variability above 35% reject the affected profile.

The gate selects the smallest profile that is simultaneously stable under continuous execution, stable under isolated verification, and within budget. Larger unstable profiles are rejected individually; they do not invalidate a smaller profile that passed. The report uses `PARTIAL` to make this distinction explicit and remains only a provisional local measurement.

[RFC 9106](https://datatracker.ietf.org/doc/rfc9106/) defines the Argon2 family and its configurable memory. The RFC literature distinguishes variants and use cases; a positive measurement is therefore not enough to turn a key derivation function into proof of work.

Independent implementations must also be compared before any choice:

- [RandomX](https://github.com/tevador/randomx), optimized for general-purpose CPUs using random code execution and memory-hard techniques;
- [Cuckoo Cycle](https://github.com/tromp/cuckoo), a memory-bound family with fast verification.

These references are research candidates, not Rabbit Chain decisions.

## What the benchmark measures

For 8, 16, 32, and 64 MiB, the program records:

- milliseconds per attempt on the local machine;
- an estimate for a PC four times slower;
- attempts per second;
- verifications possible within one second per block;
- derived difficulty for an 80% chance of finding at least one ticket in a 1,280-second epoch;
- expected time for one ticket;
- estimated cost of producing one thousand tickets.

A local profile is provisionally qualified only when:

- it uses at most 128 MiB;
- the estimated attempt on the low-end PC takes at most 2 seconds;
- at least 8 proofs fit within the one-second verification budget;
- expected ticket time does not exceed the epoch.

## Gates still required

Even if the local benchmark passes, implementation remains prohibited until completing:

1. physical tests on low-end, mid-range, and modern hardware;
2. comparison across CPU, GPU, cloud, and specialized hardware;
3. power consumption and heat during prolonged execution;
4. cost of verifying valid and invalid proofs under flooding;
5. canonical limits for tickets per block and per epoch;
6. deterministic difficulty adjustment, boundaries, and overflow;
7. grinding, ticket withholding, reorganizations, and replay;
8. a new Sybil attack with up to 100,000 identities;
9. resilience, transaction, reward, and signature tests returning to `PASS`;
10. independent review of the design and implementation.

## Launch rule

This benchmark can never authorize mainnet. Its maximum result is `PROVISIONAL`. The frozen genesis, `consensus/lqc`, and the running lab cannot be modified by this tool.
