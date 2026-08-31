# Rabbit Testnet quick start

## Current status

Rabbit Core Testnet v1 contains the validated genesis and two official discovery bootnodes. Use only packages and checksums published in the official Rabbit GitHub release.

## 1. Download

Open the official [Rabbit Core releases page](https://github.com/rabbitmainnet/rabbit-geth/releases) and select the package matching your operating system and architecture.

Supported Testnet v1 platforms:

- Windows AMD64
- Linux AMD64

macOS is not included because it has not been tested for Testnet v1.

## 2. Verify

Verify the archive checksum using `SHA256SUMS.txt` or its individual `.sha256` file. After extraction, run:

Linux:

```bash
./rabbit-core --check
```

Windows PowerShell:

```powershell
.\rabbit-core.exe --check
```

Expected identity:

- Chain ID: `9280`
- Genesis SHA-256: `8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7`
- The check must say that no node or miner was started.

## 3. Public network identity

Official services:

- HTTPS RPC: `https://rpc-testnet.rabbitchain.org`
- WebSocket RPC: `wss://rpc-testnet.rabbitchain.org/ws`
- Explorer: `https://explorer-testnet.rabbitchain.org`
- Website status: `https://rabbitchain.org/status`
- Faucet: planned, no official endpoint published

Rabbit Core runs its own full node and does not depend on the public RPC for
consensus or mining.

## 4. First run

1. open `Start-Rabbit-Core.cmd` on Windows or `start-rabbit-core.command` on Linux;
2. choose a strong mining-wallet password;
3. back up the encrypted keystore created in the Rabbit Testnet data directory;
4. allow the node to synchronize;
5. keep Rabbit Core open to run the node and participate in Rabbit Work V1;
6. press `Ctrl+C` or close the launcher to stop safely.

Rabbit Core uses a dedicated Rabbit Testnet data directory and a local JSON-RPC endpoint at `127.0.0.1:8545`. It does not need a public RPC to mine when its local node is synchronized.

## Troubleshooting

- `wrong genesis SHA-256`: delete the package and download it again from the official release.
- `wrong chain ID`: stop immediately; the node is connected to the wrong network or data directory.
- node startup problems: inspect `logs/rabbit-node.log` inside the Rabbit Testnet data directory.
- never solve an error by entering a seed phrase or private key on a website.
