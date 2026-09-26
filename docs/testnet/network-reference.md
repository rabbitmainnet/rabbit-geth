# Rabbit Testnet network reference

This page summarizes the authoritative testnet genesis at `networks/rabbit-testnet/genesis.json`.

## Identity

| Parameter | Value |
| --- | --- |
| Chain ID | `9280` |
| Chain ID hexadecimal | `0x2440` |
| Network ID | `9280` |
| Native coin | `RAB` (testnet value only) |
| Genesis SHA-256 | `8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7` |
| Genesis label | `RABBIT_TESTNET_GENESIS_V1` |
| Initial gas limit | `30,000,000` |
| Initial base fee | `1,000,000,000` wei |

## LCQ parameters

| Parameter | Value |
| --- | ---: |
| Committee minimum | 32 |
| Committee maximum | 128 |
| Committee reward ratio | 30% |
| Committee sizing | ceil(active participants × 10%), clamped to 32–128 and available seats |
| Fallback slots | 5 |
| Fallback window | 3,000 ms |
| Target block time | 10,000 ms |
| Epoch length | 128 blocks |
| Activity window | 128 blocks |
| Registry mode | native |
| Registry protocol block | 1 |
| Minimum bond | 25 |
| Activation delay | 2 blocks |
| Heartbeat window | 64 blocks |
| Heartbeat grace | 16 blocks |
| Recovery timeout | 3,600,000 ms |
| Jail period | 256 blocks |
| Maximum missed turns | 3 |
| Minor slash | 5% |
| Major slash | 20% |

These are testnet parameters. They must not be assumed to be final mainnet parameters.

The committee sizing percentage and committee reward percentage are different
rules. See [LCQ protocol overview](../consensus/lcq-protocol.md) and
[Rewards and emission](../economics/rewards-and-emission.md).

## Public endpoints

| Service | Canonical address | Status |
| --- | --- | --- |
| HTTPS RPC | `https://rpc-testnet.rabbitchain.org` | Live |
| WebSocket RPC | `wss://rpc-testnet.rabbitchain.org/ws` | Live |
| Explorer | `https://explorer-testnet.rabbitchain.org` | Live |
| Faucet | Not published | Planned |
| Website status | `https://rabbitchain.org/status` | Live |

## Official bootnodes

- `enode://2fac5ffabae5e2202666279e2d06f86b6f3f11977fa0818ad795889a55b32e2d4bd651bb5860fc6e88072a01cad23df475a36487cdcd496e97932909600b7791@157.245.245.16:30303`
- `enode://d106f46d3e37a5b8487b1a4b70a11519e77cf8c4532065a3508ebf943e44840867eaff2a4fa8e525e4dea6680d1aef7cb1c2824f3613fc3482b92eb026d357d4@24.144.112.140:30303`

Bootnodes provide peer discovery only. Community bootnodes are supported.
