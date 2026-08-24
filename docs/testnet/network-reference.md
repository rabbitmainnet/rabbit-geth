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
| Committee ratio | 30% |
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

## Reserved public endpoints

| Service | Canonical address | Status |
| --- | --- | --- |
| HTTPS RPC | `https://rpc.testnet.rabbitchain.org` | Activates with the public testnet |
| WebSocket RPC | `wss://ws.testnet.rabbitchain.org` | Activates with the public testnet |
| Explorer | `https://explorer.testnet.rabbitchain.org` | Activates with the public testnet |
| Faucet | `https://faucet.testnet.rabbitchain.org` | Activates with the public testnet |
| Network status | `https://status.testnet.rabbitchain.org` | Activates with the public testnet |

Bootnode enodes will be added after persistent nodekeys and external P2P connectivity are validated.
