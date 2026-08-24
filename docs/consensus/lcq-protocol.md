# LCQ protocol overview

This page is the public protocol overview for Live Consensus Queue (LCQ) on
Rabbit Testnet. The implementation and the authoritative genesis remain the
source of truth.

## Purpose

LCQ combines an open, native participant registry with deterministic producer,
fallback, and committee selection. Rabbit Work V1 supplies verifiable work
tickets used by the production selection profile. No official RPC, explorer,
website, bootnode, or project server has consensus authority.

## Open participation

Rabbit Testnet starts with an empty `bootstrapParticipants` set. Block 1 is the
permissionless activation block: a local wallet can activate the chain without
being pre-approved. From registry protocol block 1 onward, signed native
registry operations add and maintain participants under the same protocol rules
for every node.

The testnet registry parameters include:

| Rule | Value |
| --- | ---: |
| Registry mode | `native` |
| Minimum bond | `25` |
| Activation delay | 2 blocks |
| Heartbeat window | 64 blocks |
| Heartbeat grace | 16 blocks |
| Activity window | 128 blocks |
| Jail period | 256 blocks |
| Maximum missed turns | 3 |
| Minor slash | 5% |
| Major slash | 20% |

Registry operations are signed by the participant, canonically ordered, bound
to the chain ID, sequence checked, validity limited, and committed into the
canonical chain. Operators do not register through a project-controlled web
form or allowlist.

## Deterministic queue

For each block, nodes independently derive the same ordered participant set
from canonical chain state and the parent seed. The first eligible position is
the producer. Up to five subsequent positions are fallbacks.

The producer may create the next block after the 10-second target. If it misses
its turn, fallback positions open sequentially in 3-second windows. An
authorized fallback that actually creates the canonical block becomes that
block's author and receives the producer portion of its reward.

After a prolonged halt, the one-hour recovery timeout reopens producer
activation. This liveness mechanism continues from the existing canonical
history; it does not reset execution state or create authority for a project
server.

## Committee and the two percentages

Two different percentages appear in LCQ and must not be confused:

- committee sizing uses `ceil(active participants * 10%)`, clamped to the
  configured minimum of 32 and maximum of 128, then capped by the participants
  available after producer and fallback positions;
- committee rewards use 30% of the block subsidy, divided by committee seat.

The `committeeRatioBps = 3000` genesis field controls the reward split. It does
not change the current 10% committee-sizing algorithm.

## Rabbit Work V1

Rabbit Work V1 uses signed RandomX work tickets. Tickets are chain- and
epoch-bound, validated against canonical anchors, difficulty checked, and
admitted only while the participant remains eligible. Epochs are 128 blocks.
Pending tickets are persisted, and tickets removed by a reorganization are
readmitted only while still valid.

Work does not grant an operator administrative control. It is protocol input
verified independently by every full node. The production profile is built
with the `rabbit_workv1` and `rabbit_randomx` tags.

## Block validity and finality

Every full node verifies the header, producer authorization, timing, registry
commitment, work rules, execution result, and state root. LCQ does not depend on
the official explorer or RPC to decide which block is valid.

As with other live distributed networks, applications should choose a
confirmation policy appropriate to their risk. A block being visible in an
explorer is not itself a finality guarantee.

## Authoritative references

- Testnet parameters: `networks/rabbit-testnet/genesis.json`
- Chain configuration: `params/config.go`
- LCQ engine: `consensus/lqc/`
- Production Work V1 profile: `docs/rabbit-work-v1-production-release.md`
- Network reference: `docs/testnet/network-reference.md`
