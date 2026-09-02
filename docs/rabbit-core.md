# Rabbit Core — Public Testnet V2

Rabbit Core is the supported automatic launcher for a Rabbit full node and Work V2 admission miner. A normal user does not need to configure JSON-RPC, keystore paths, genesis initialization, or mining commands manually.

## First run

1. Open Rabbit Core using the supplied operating-system launcher.
2. If no wallet exists, choose a strong local password twice.
3. Record the wallet address and back up the exact encrypted keystore file printed on screen. Store its password separately.
4. Rabbit Core verifies the official genesis, initializes its data directory, connects to bootnodes, and synchronizes its full node.
5. Rabbit Miner waits for the canonical admission window and performs the wallet's one-time RandomX Work V2 admission.
6. An accepted proof displays `ADMISSION_PENDING`; keep the application open.
7. At canonical activation it displays `ACTIVE_SEAT`. RandomX stops, while the node remains online for equal-seat LCQ participation.
8. Stop safely with Ctrl+C once and wait for shutdown to finish.

On a fresh network, blocks 1–127 bootstrap canonical history, admission begins at block 128, and the first admitted seats activate at block 256. At a 10-second target these heights are approximately 21 and 43 minutes from genesis, but height—not local elapsed time—controls activation. Read `rabbit-miner.md` for the full lifecycle, reasons, messages, recovery, and troubleshooting.

## Normal operation and recovery

Keep Rabbit Core running to validate, propagate, and participate. Closing it does not erase a canonical seat. Restart with the same wallet and data directory so Rabbit Core detects the persistent seat without repeating RandomX.

If every producer disconnects, the chain pauses at its last valid block. After a two-minute canonical halt, compatible software opens permissionless RandomX recovery admission. Any non-zero wallet may submit a valid proof and resume the existing chain. Recovery does not reset history or erase balances, transactions, and persistent seats.

## Network services

- Bootnodes provide P2P discovery only.
- Public RPC: `https://rpc-testnet.rabbitchain.org`
- WebSocket: `wss://rpc-testnet.rabbitchain.org/ws`
- Explorer: `https://explorer-testnet.rabbitchain.org`
- Website and downloads: `https://rabbitchain.org`

The RPC serves wallets and read access. The explorer independently indexes canonical data and can lag. Neither controls consensus. Anyone may operate a community node, RPC, bootnode, or explorer.

## Rewards and fairness

Every active wallet has one persistent equal seat regardless of CPU speed. The current 1.2 tRAB testnet block reward is divided 70/30: 0.84 tRAB to the selected producer and 0.36 tRAB total to the eligible committee.

## Verification

Run `rabbit-core --check` and verify both the outer archive checksum and internal `SHA256SUMS.txt`.

- Chain ID and Network ID: `9280`
- Native testnet coin: `tRAB`
- Genesis SHA-256: `e2e5494542e37689cb6e385456d6df239e478c1d12e9c3a1cc270e69c6b51686`
- RandomX base commit: `7c761cf007c758056dcb6eb438a32f780f81bdbd`
- RandomX dataset: `1073741824` bytes (1 GiB)

Rabbit Core refuses an incompatible genesis or chain ID. The package must contain all three binaries, genesis, bootnodes, launcher, documentation, metadata, notice, and checksums.

## Support report

Provide operating system, Rabbit Git commit, wallet address only, local/public heights, peer count, synchronization state, and the exact sanitized error. Never provide secrets. Official software must come from `rabbitmainnet/rabbit-geth` or the official Rabbit Chain website.
