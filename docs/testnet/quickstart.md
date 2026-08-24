# Rabbit Testnet quick start

## Current status

The native preview binaries are published for external verification. The public testnet is not active until an official activation release contains validated bootnodes.

Do not manually add an unofficial enode or start an isolated chain using the official genesis.

## 1. Download

Open the official [Rabbit Core releases page](https://github.com/rabbitmainnet/rabbit-geth/releases) and select the package matching your operating system and architecture.

Current preview platforms:

- Linux AMD64
- Windows AMD64
- macOS Intel/AMD64
- macOS Apple Silicon/ARM64

## 2. Verify

Verify the archive checksum using `SHA256SUMS.txt` or its individual `.sha256` file. After extraction, run:

Linux and macOS:

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

## 3. Wait for activation

The preview package intentionally has an empty `bootnodes.txt`, so Rabbit Core refuses to start an isolated network. This is a safety feature.

At public activation, a new release will include the validated bootnode configuration. Download and verify that release instead of editing the preview package manually.

Reserved services:

- `https://rpc.testnet.rabbitchain.org`
- `wss://ws.testnet.rabbitchain.org`
- `https://explorer.testnet.rabbitchain.org`
- `https://faucet.testnet.rabbitchain.org`
- `https://status.testnet.rabbitchain.org`

They may remain offline until public activation.

## 4. First public-testnet run

After the activation release is published:

1. open `Start-Rabbit-Core.cmd` on Windows or `start-rabbit-core.command` on Linux/macOS;
2. choose a strong mining-wallet password;
3. back up the encrypted keystore created in the Rabbit Testnet data directory;
4. allow the node to synchronize;
5. keep Rabbit Core open to run the node and participate in Rabbit Work V1;
6. press `Ctrl+C` or close the launcher to stop safely.

Rabbit Core uses a dedicated Rabbit Testnet data directory and a local JSON-RPC endpoint at `127.0.0.1:8545`. It does not need a public RPC to mine when its local node is synchronized.

## Troubleshooting

- `official bootnodes are not configured yet`: you are using the preview before activation.
- `wrong genesis SHA-256`: delete the package and download it again from the official release.
- `wrong chain ID`: stop immediately; the node is connected to the wrong network or data directory.
- node startup problems: inspect `logs/rabbit-node.log` inside the Rabbit Testnet data directory.
- never solve an error by entering a seed phrase or private key on a website.
