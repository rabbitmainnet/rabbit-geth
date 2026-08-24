# Rabbit Chain

Rabbit Chain is an EVM-compatible Layer 1 blockchain powered by **LCQ (Live Consensus Queue)** and Rabbit Work V1. The network is designed for open participation: download the official software, run a node, and participate without manual registration or permission from an operator.

> The public testnet is currently in native preview. The binaries are available for verification, but the public network will be activated only after the official bootnodes and RPC infrastructure are online.

## Official links

- Website: [rabbitchain.org](https://rabbitchain.org)
- GitHub organization: [github.com/rabbitmainnet](https://github.com/rabbitmainnet)
- Source code: [rabbitmainnet/rabbit-geth](https://github.com/rabbitmainnet/rabbit-geth)
- Releases: [Rabbit Core downloads](https://github.com/rabbitmainnet/rabbit-geth/releases)
- X: [@rabbit_mainnet](https://x.com/rabbit_mainnet)

Only links published on the website or this repository should be treated as official. Never enter a seed phrase or raw private key on a website.

## Rabbit Testnet

| Parameter | Value |
| --- | --- |
| Chain ID | `9280` (`0x2440`) |
| Network ID | `9280` |
| Native coin | `RAB` (testnet value only) |
| Block target | 10 seconds |
| Consensus | LCQ + Rabbit Work V1 |
| Execution | EVM-compatible |
| Genesis SHA-256 | `8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7` |

The authoritative network parameters are defined in [`networks/rabbit-testnet/genesis.json`](networks/rabbit-testnet/genesis.json).

## Native preview downloads

The first validated preview is [Rabbit Core Testnet Preview v1](https://github.com/rabbitmainnet/rabbit-geth/releases/tag/rabbit-testnet-preview-v1), available for:

- Linux AMD64
- Windows AMD64
- macOS Intel/AMD64
- macOS Apple Silicon/ARM64

Preview packages deliberately contain an empty `bootnodes.txt`. Rabbit Core validates the package with `--check`, but refuses to start an isolated network. A public activation release will be published after the official bootnodes, DNS/TLS endpoints, and final launch gate are validated.

## Reserved public testnet endpoints

These canonical addresses will become operational when the public testnet is activated:

- HTTPS RPC: `https://rpc.testnet.rabbitchain.org`
- WebSocket RPC: `wss://ws.testnet.rabbitchain.org`
- Explorer: `https://explorer.testnet.rabbitchain.org`
- Faucet: `https://faucet.testnet.rabbitchain.org`
- Network status: `https://status.testnet.rabbitchain.org`

Until the official activation announcement, these services may remain offline.

## Verify a download

Download the archive and its checksum from the same GitHub release. On Linux:

```bash
sha256sum -c rabbit-core-testnet-linux-amd64-preview.tar.gz.sha256
```

After extracting the package:

```bash
./rabbit-core --check
```

This check verifies the official genesis and package structure without starting a node or miner.

## How Rabbit Core works

Rabbit Core is the user-facing launcher included with every native package. At public testnet activation it will:

1. verify the official genesis;
2. load the official bootnodes;
3. create or open one encrypted local mining wallet;
4. initialize an isolated Rabbit Testnet data directory;
5. start the Rabbit node and local-only JSON-RPC endpoint;
6. start Rabbit Work V1 mining;
7. stop the node and miner together when the user exits.

Wallet passwords and private keys remain local. The miner submits only signed Work V1 candidates to the local Rabbit node.

## Documentation

- [Documentation index](docs/README.md)
- [Testnet quick start](docs/testnet/quickstart.md)
- [Testnet network reference](docs/testnet/network-reference.md)
- [Rabbit Core](docs/rabbit-core.md)
- [Rabbit Miner Work V1](docs/rabbit-miner.md)
- [Native release process](docs/rabbit-native-release.md)
- [Security policy](SECURITY.md)

## Build from source

Official native releases use Go `1.25.13`, a native C/C++ toolchain, CMake, NASM, and RandomX pinned to commit `7c761cf007c758056dcb6eb438a32f780f81bdbd` with the Rabbit 1 GiB profile.

The reproducible multi-platform workflow is located at [`.github/workflows/rabbit-core-native-preview.yml`](.github/workflows/rabbit-core-native-preview.yml). Building from source does not replace verification of the official genesis, release commit, and checksums.

## Security

- Keep testnet and future mainnet wallets separate.
- Back up encrypted keystore files offline.
- Never expose `personal`, `admin`, `debug`, authenticated RPC, or IPC to the public internet.
- Verify the Chain ID before signing transactions.
- Report vulnerabilities privately through the repository's [Security Advisories](https://github.com/rabbitmainnet/rabbit-geth/security/advisories/new).

## License and upstream

Rabbit Chain is derived from go-ethereum and preserves the applicable upstream copyright and license notices. Library code is licensed under GNU LGPL v3 and command binaries under GNU GPL v3. See [`COPYING.LESSER`](COPYING.LESSER) and [`COPYING`](COPYING).
