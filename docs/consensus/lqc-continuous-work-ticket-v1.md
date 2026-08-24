# Rabbit LQC — Continuous Work Ticket Proposal v1

Status: specification for simulation. Not implemented in consensus.

## Confirmed problem

The current registry grants one complete selection position to every address that completes a registration LightHash. The proof is paid only once. A single controller can create thousands of keys and retain thousands of positions in the queue, fallbacks, and committee.

The `rabbit-lqc-sybil-auditor/1.0.0` auditor confirmed that 5,000 addresses competing against 20 honest addresses receive approximately 99.6% of selection.

## Security goal

Creating additional addresses must not create power without additional cost. Participation must be proportional to recent, verifiable computational work.

This proposal does not promise one identity per person. That would require an identity authority, KYC, stake, trusted hardware, or another external resource. The guarantee available to a permissionless network is one chance per unit of work.

## Work ticket

A ticket is a valid hash proof for a future epoch:

```text
ticketHash = Keccak256(
  "RABBIT-LQC-WORK-TICKET-V1" ||
  chainID ||
  eligibilityEpoch ||
  challenge ||
  signingKey ||
  payoutAddress ||
  nonce
)

ticketHash < target
```

Every attempt requires work. Creating keys or wallets without finding hashes below the target creates no tickets and does not affect selection.

## Per-epoch lifecycle

1. Epoch `N` starts with a challenge derived from an already finalized canonical checkpoint.
2. Miners search for tickets during a public submission window.
3. Each ticket is signed and bound to the chain ID, epoch, producer key, and payout address.
4. The window closes before the selection seed for the use epoch exists.
5. The seed is derived from a checkpoint after the window closes.
6. Tickets are used only in the specified epoch and expire at its end.
7. Future participation requires new work.

A two-epoch activation delay separates the work from the selection seed and reduces grinding:

```text
epoch N:     mine and publish tickets
epoch N+1:   closing, confirmation, and future seed
epoch N+2:   tickets eligible for producer, fallbacks, and committee
```

## LCQ selection

Instead of ordering registered addresses, the queue orders valid ticket identifiers:

```text
score = Keccak256(selectionSeed || ticketHash)
```

The lowest scores fill, in order:

1. producer;
2. fallbacks;
3. committee.

A controller may use multiple keys, but every additional position requires another valid ticket. Splitting the same number of attempts among one thousand keys does not change the expected number of tickets.

## User experience

The user keeps one payout wallet. The Rabbit client should create and renew tickets automatically in the background without requiring the user to manage thousands of keys, nonces, or RPC operations. Ephemeral producer keys may be bound to the same payout by signature, but can never create weight without the corresponding proof.

The miner interface should show at least the current epoch, challenge, attempt rate, tickets found, validity, estimated chance, and canonical confirmation. Participation requires no RAB balance, stake, or administrative authorization.

## Role of IP address and device identity

IP, ASN, fingerprints, MAC addresses, and hardware identifiers do not participate in consensus calculations. They may rate-limit connections or messages in P2P transport, but can never decide a ticket, producer, fallback, committee, or reward. This avoids penalizing users behind NAT/CGNAT and avoids granting power to providers, VPNs, cloud platforms, or manufacturers.

## Difficulty and capacity

Difficulty must be deterministic and use only canonical data from earlier epochs. Adjustment must:

- target a bounded number of tickets per epoch;
- have minimum and maximum limits;
- react with delay to prevent instant manipulation;
- use identical integer arithmetic in every client;
- be covered by boundary and overflow tests.

The protocol must also define limits for operations per block, tickets per epoch, message size, and cache size. When demand exceeds capacity, difficulty must reduce the ticket rate instead of relying on RPC arrival order.

## Canonical state

The minimum ticket state contains:

- version;
- chain ID;
- eligibility epoch;
- challenge;
- ticket hash;
- signing key;
- payout;
- signature;
- canonical inclusion block.

Tickets must be reconstructible from headers, isolated by block hash, reverted during reorganizations, and removed after expiry. Local cache can never be a source of consensus truth.

## Bootstrap

Bootstrap participants may receive a short startup window defined before launch. After that window, everyone, including bootstrap participants, must have tickets. No permanent privilege is allowed.

## Rewards

The selected ticket identifies the key authorized to sign the block and the reward payout. The 70/30 split, issuance, halving, and immediate reward do not need to change because of this proposal.

## Attacks requiring dedicated tests

- multiplication to 100, 1,000, 5,000, and 100,000 keys with fixed total work;
- a real increase in adversarial computing capacity;
- grinding of keys, payout, nonce, timestamp, and checkpoint;
- strategic withholding of tickets;
- pool and gossip flooding;
- duplicate tickets and replay across epochs and networks;
- reorganizations during submission, closing, and activation;
- partial outage and total restart;
- producer and committee composed of tickets from one controller;
- difficulty adjustment during growth and sudden loss of hash power;
- new-node synchronization and historical reconstruction;
- CPU, memory, disk, and bandwidth consumption.

## Mandatory gates

1. The simulator must show that additional identities with constant total work do not increase producer, fallback, or committee selection.
2. The implementation must pass unit tests and fuzzing.
3. A clean lab must repeat the thousands-of-keys scenario.
4. Rewards, signatures, transactions, resilience, and boundaries must remain in `PASS`.
5. Mainnet may be reconsidered only after all these gates pass.
