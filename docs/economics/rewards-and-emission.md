# Rewards and emission

This page describes the subsidy rules implemented by LCQ. Values on Rabbit
Testnet have no guaranteed monetary value and must not be presented as an
investment return.

## Monetary units

`RAB` is the native accounting unit. One RAB contains
`1,000,000,000,000,000,000` wei (`10^18` wei). Consensus performs every reward
calculation in integer wei; displayed decimal values are only a human-readable
representation.

## Block subsidy and halving boundaries

The era length is 8,409,600 blocks. Genesis is block 0 and receives no block
reward.

| Era | Block range | Subsidy per block | Producer 70% | Committee 30% total |
| ---: | --- | ---: | ---: | ---: |
| 0 | 1–8,409,599 | 1.20 RAB | 0.84 RAB | 0.36 RAB |
| 1 | 8,409,600–16,819,199 | 0.60 RAB | 0.42 RAB | 0.18 RAB |
| 2 | 16,819,200–25,228,799 | 0.30 RAB | 0.21 RAB | 0.09 RAB |
| 3+ | 25,228,800 and later | 0.15 RAB | 0.105 RAB | 0.045 RAB |

The exact halving boundaries are therefore:

- first halving: block 8,409,600;
- second halving: block 16,819,200;
- terminal-subsidy boundary: block 25,228,800.

These events are triggered by block height, never by a wall-clock date or a
project administrator. At the 10-second target, an era is approximately 973.33
days, or 2.67 years. This time estimate can differ from reality because block
height—not elapsed time—is authoritative.

The 0.15 RAB terminal subsidy continues indefinitely under the current rules.
This is a permanent tail emission, not a fixed maximum-supply schedule.

## Scheduled issuance totals

Because genesis receives no reward, Era 0 contains 8,409,599 rewarded blocks.
The next two completed eras contain 8,409,600 rewarded blocks each.

| Source | Scheduled issuance |
| --- | ---: |
| Genesis allocation | 15,000,000 RAB |
| Era 0 block subsidies | 10,091,518.80 RAB |
| Era 1 block subsidies | 5,045,760.00 RAB |
| Era 2 block subsidies | 2,522,880.00 RAB |
| Supply at the start of block 25,228,800 | 32,660,158.80 RAB |
| Tail emission after that boundary | 0.15 RAB per canonical rewarded block |

At exactly one block every 10 seconds, the tail subsidy would issue about
473,040 RAB per 365-day year. This is an estimate, not a consensus constant.
There is no finite maximum supply under the implemented tail-emission rule.

## Reward allocation

For a block with an eligible committee:

- 70% goes to the actual canonical block author;
- 30% is divided equally by committee seat;
- any indivisible wei remainder is assigned deterministically to the first
  committee address, so every wei is conserved.

If an authorized fallback creates the block, it receives the 70% producer
share. A fallback that did not create the block receives no producer reward. If
there is no eligible committee, the full subsidy goes to the block author.

The 30% committee amount is divided by the number of eligible committee seats
for that block. For example, with 32 paid seats in Era 0, each seat receives
0.01125 RAB. With 128 paid seats, each seat receives 0.0028125 RAB. Committee
membership is derived by consensus and can change from block to block.

## Work V1 liveness rule

In the Work V1 production profile, the same 70/30 allocation applies to the
actual authorized author and committee. If no eligible Work seats exist, the
registry may preserve liveness while the base subsidy is zero. This prevents a
liveness path from silently becoming an unearned issuance path.

## Reward maturity and legacy vesting

The active LCQ finalization path credits mining rewards directly to protocol
balances. Legacy vesting helpers remain in the source for compatibility and
historical tests, but active LCQ mining rewards are not routed through that
legacy vesting subsystem.

Therefore, the current active LCQ reward path does not impose the legacy
100,000-block mining-reward lock or its quarterly release schedule. This
distinction is important when reading older engineering records in `docs/`.

Transaction fees and the EVM fee rules are separate from the block subsidy.
Wallets and explorers should display subsidy, fees, and balance changes as
distinct concepts.

## Genesis allocation

Rabbit Testnet genesis contains exactly 15,000,000 RAB at
`0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0`. This is testnet state, not a
mining reward and not evidence of a mainnet allocation. The repository does not
assign economic categories to portions of this test allocation. Any faucet,
liquidity, development, or distribution policy must be published separately
and must not be described as a consensus rule.

## What can change the supply

The consensus supply schedule described here consists of the immutable genesis
allocation plus canonical block subsidies. Transaction fees transfer value
between users, block authors, and the protocol fee mechanism; they are not part
of the subsidy table. Applications, faucets, bridges, and tokens deployed as
smart contracts cannot alter native RAB issuance unless a future consensus
upgrade changes the protocol and is adopted by network participants.

## Mainnet warning

This page documents the current Rabbit Testnet implementation. A future
mainnet must publish its own genesis, chain ID, allocation, era parameters,
reward schedule, and independent economic review. Testnet RAB has no guaranteed
conversion into future mainnet RAB.

## Verification references

- Subsidy and distribution: `consensus/lqc/lqc.go`
- Boundary and conservation tests: `consensus/lqc/reward_test.go`
- Work V1 production rules: `docs/rabbit-work-v1-production-release.md`
- Reward audit records: `docs/rabbit-reward-audit-source-review.md`
