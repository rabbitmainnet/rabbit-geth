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

## Exact wallet and data locations

Rabbit Core derives its directory from the operating system user configuration
folder and appends `RabbitChain/TestnetV2`.

### Windows

- Main directory: `%APPDATA%\RabbitChain\TestnetV2`
- Typical full path: `C:\Users\<USER>\AppData\Roaming\RabbitChain\TestnetV2`
- Encrypted wallet: `%APPDATA%\RabbitChain\TestnetV2\keystore\UTC--...`
- Blockchain data: `%APPDATA%\RabbitChain\TestnetV2\rabbit\chaindata`
- Node log: `%APPDATA%\RabbitChain\TestnetV2\logs\rabbit-node.log`

### Linux

- Main directory: `${XDG_CONFIG_HOME:-$HOME/.config}/RabbitChain/TestnetV2`
- Encrypted wallet: `${XDG_CONFIG_HOME:-$HOME/.config}/RabbitChain/TestnetV2/keystore/UTC--...`
- Blockchain data: `${XDG_CONFIG_HOME:-$HOME/.config}/RabbitChain/TestnetV2/rabbit/chaindata`
- Node log: `${XDG_CONFIG_HOME:-$HOME/.config}/RabbitChain/TestnetV2/logs/rabbit-node.log`

Rabbit Core prints the resolved paths at startup. The hidden
`.rabbit-session-password-*` file is temporary, permission-restricted, and
removed automatically when the session ends. It is not a wallet backup.

To move to another computer, stop Rabbit Core cleanly and copy the encrypted
`UTC--...` keystore file. Keep its password separately. Never upload or send
either item to support. Preserve the complete data directory when possible to
avoid downloading and validating the chain again.

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
- Genesis SHA-256: `4970159f077b62b7aa9bde1f81cb61ae837b42ef64861d7b23241e1af4d75c9f`
- RandomX base commit: `7c761cf007c758056dcb6eb438a32f780f81bdbd`
- RandomX dataset: `1073741824` bytes (1 GiB)

Rabbit Core refuses an incompatible genesis or chain ID. The package must contain all three binaries, genesis, bootnodes, launcher, documentation, metadata, notice, and checksums.

## Support report

Provide operating system, Rabbit Git commit, wallet address only, local/public heights, peer count, synchronization state, and the exact sanitized error. Never provide secrets. Official software must come from `rabbitmainnet/rabbit-geth` or the official Rabbit Chain website.

## V2.4.14 fork and upgrade


RABBIT CORE V2.4.14 - TESTNET FORK AT BLOCK 320000

Upgrade miners, full nodes and public infrastructure before activation.
Retain the existing blockchain, encrypted wallet, node identity and
participation state. Stop normally before replacing software.
Do not reset the data directory to perform this upgrade.

The public network remains Chain ID 9280.
Genesis block hash remains:
0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7
The SHA256 of the distributed genesis.json file is:
4970159f077b62b7aa9bde1f81cb61ae837b42ef64861d7b23241e1af4d75c9f

At block 320000, VRF economics V2 and WorkSeat admission V2 activate.
The block target remains 10 seconds. Mining rewards remain 70/30.
The VRF base pricing target becomes US$0.005 equivalent in tRUSD,
payable in tRAB using quoteRequestFee(0); transaction gas is additional.
At successful fulfillment, 20% goes to the fulfillment block producer,
30% equally to validated VRF signers and 50% plus integer rounding
remainders to the existing development allocation address.
Pending requests retain their recorded fee; the split follows the
fulfillment block.

Fresh installations retain the verified automatic multipart bootstrap.
Existing blockchain data must not be replaced by a fresh bootstrap.
Use reliable SSD storage for latency-sensitive VRF persistence.
Early proof verification is distinct from canonical on-chain fulfillment;
local laboratory latency measurements are not a public-network SLA.
