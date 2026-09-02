# Rabbit Miner — Public Testnet V2

Rabbit Miner performs the one-time RandomX admission required to obtain one persistent Rabbit Chain consensus seat. After activation, RandomX stops automatically. The Rabbit node remains online for LCQ block production, validation, P2P propagation, and committee participation.

A faster CPU may find admission sooner, but it never receives extra consensus weight. One wallet can own at most one persistent seat.

## Official network identity

- Network: Rabbit Chain Testnet
- Chain ID and Network ID: `9280`
- Currency: `tRAB` (testnet only)
- Target block time: approximately 10 seconds
- Epoch length: 128 blocks
- Public RPC: `https://rpc-testnet.rabbitchain.org`
- WebSocket: `wss://rpc-testnet.rabbitchain.org/ws`
- Explorer: `https://explorer-testnet.rabbitchain.org`
- Website: `https://rabbitchain.org`
- Source: `https://github.com/rabbitmainnet/rabbit-geth`
- Genesis SHA-256: `e2e5494542e37689cb6e385456d6df239e478c1d12e9c3a1cc270e69c6b51686`

## Complete timeline on a fresh network

1. Blocks 1–127 bootstrap canonical history. The activation wallet keeps the new network moving while nodes discover peers and agree on the same chain.
2. At block 128, the first Work V2 admission epoch opens. Rabbit Miner prepares the Rabbit RandomX 1 GiB dataset and searches for the wallet's one-time admission proof.
3. `ADMISSION_ACCEPTED_LOCAL` means the local relay accepted the proof and is propagating it.
4. `ADMISSION_PENDING state=accepted_by_local_relay` means local acceptance exists, but canonical confirmation is pending.
5. `ADMISSION_PENDING state=canonical_waiting_for_activation` means the proof is canonical. Do not restart admission mining or delete the data directory.
6. During blocks 129–255, all nodes deterministically confirm the same admissions before changing the consensus producer set.
7. At block 256, the first canonical admissions become persistent seats. Rabbit Miner prints `ACTIVE_SEAT`.
8. After activation, RandomX stops. Keep Rabbit Core running so the node can be selected for LCQ production and committee participation.

At the 10-second target, block 128 is about 21 minutes and block 256 about 43 minutes after genesis. These are estimates. Activation follows canonical block height, not a timer on one computer, so a paused or slow chain takes longer.

Wallets joining later mine only during an applicable open admission epoch and activate at its deterministic canonical selection boundary. The miner reports the state automatically; users do not need to calculate the boundary.

## Why activation waits

Immediate activation would allow a local or late proof to change the producer set before every honest node saw the same canonical evidence. Epoch-boundary activation gives every node the same ordered persistent-seat set and protects deterministic consensus, safe restart, and equal treatment across the P2P network.

## Status messages

- `Waiting for canonical Work V2 status`: no admission window exists at the current height. Waiting is normal and resumes automatically.
- `Preparing the Rabbit RandomX 1 GiB dataset`: one-time epoch preparation; slower computers may take longer.
- `Rabbit RandomX dataset ready`: admission proof search can begin.
- `Mining Work V2 admission`: active proof search. Attempts and H/s show progress but never create multiple seats.
- `ADMISSION_ACCEPTED_LOCAL`: accepted by the local relay and propagating.
- `ADMISSION_PENDING`: accepted; wait for canonical activation and do not mine a duplicate proof.
- `ACTIVE_SEAT`: the wallet owns one persistent equal seat. RandomX is done, but the node remains online for consensus.
- `NETWORK_STATUS`: height, peers, synchronization, balance, latest producer, and blocks produced in this session.
- `candidate_rejected`: read the exact `error=` reason. The miner normally refreshes its context automatically.

`committed=false` after `ACTIVE_SEAT` is expected: the temporary commitment was consumed when it became a persistent seat.

## Equal participation and rewards

Each active wallet has exactly one seat. CPU speed, thread count, attempts, and hardware price do not multiply selection weight. A different wallet must earn its own valid admission proof and still receives at most one seat.

The testnet block reward is divided by consensus: 70% to the selected producer and 30% total to the eligible committee. With the current 1.2 tRAB reward, that is 0.84 tRAB plus 0.36 tRAB. Committee recipients may split their total differently according to canonical eligibility, while the total remains 70/30.

## Complete network halt and recovery

If every producer goes offline, the chain pauses at its last valid canonical block. It does not reset. After a two-minute canonical halt, permissionless recovery admission opens. Any non-zero wallet running compatible software may find and submit a valid RandomX recovery proof. The recovery identity advances the existing chain until canonical persistent-seat operation resumes.

Recovery does not erase blocks, balances, transactions, or existing persistent seats. RPCs, bootnodes, explorers, and project operators have no authority to assign recovery seats or rewrite consensus.

## Safe start, stop, restart, and update

- Start the supplied Rabbit Core launcher.
- Wait for peers and `synced=true` before diagnosing admission or rewards.
- Stop with Ctrl+C once and wait for clean shutdown.
- Preserve the encrypted keystore, separate password, and Rabbit data directory. Rabbit Core prints their locations.
- Restart with the same wallet and data directory. An active canonical seat does not require a second admission proof.
- Never run two miners against the same wallet and data directory.
- Never delete chain data merely because the miner is waiting normally.
- Before an upgrade, back up the keystore, verify checksums, read the notice, stop cleanly, and then replace the software.

## RPC, bootnodes, and explorer

Bootnodes provide discovery. The public RPC and WebSocket serve wallets and inspection. The explorer indexes canonical blocks and may temporarily lag. None creates seats, selects producers, stores user passwords, or controls consensus. A participant validates through its local Rabbit node; the public RPC is not a replacement for that node.

Compare local height, public RPC height, and explorer height when diagnosing. Consensus follows valid canonical blocks, not the explorer interface.

## Troubleshooting

- `peers=0`: verify internet, firewall, VPN/router restrictions, system clock, and `bootnodes.txt`; allow Rabbit P2P TCP and UDP traffic.
- `synced=false`: keep the node running until its height catches up.
- Local RPC unavailable: confirm Rabbit Core and `rabbit-node` are still open; Rabbit Miner normally uses `127.0.0.1:8545`.
- Explorer behind RPC: the indexer is delayed; this does not change consensus.
- Zero balance before activation: normal until producer or committee rewards are received.
- Window closes immediately: start the supplied launcher from a terminal and inspect the log path printed by Rabbit Core.
- Repeated rejection or no progress across an applicable epoch: record wallet address, height, peers, exact error, and Git commit, then request support.

## Security and verification

- Use an encrypted Web3 keystore and back up the exact file Rabbit Core prints.
- Keep the password separately; the project cannot recover it.
- Never share password, private key, seed phrase, or keystore with support.
- Rabbit Miner sends participant address, nonce, proof hash, and signature; it does not transmit the password or private key.
- Verify the published archive checksum and internal `SHA256SUMS.txt`.
- Confirm chain ID 9280 and the official genesis hash above.
- Testnet tRAB is not mainnet RAB and has no guaranteed monetary value.

## Advanced command line

```text
rabbit-miner \
  --rpc http://127.0.0.1:8545 \
  --keystore /path/to/UTC--encrypted-key \
  --password-file /path/to/password.txt
```

The package contains `rabbit-node`, `rabbit-miner`, `rabbit-core`, the frozen `genesis.json`, `bootnodes.txt`, launchers, build metadata, documentation, security notice, and checksums.
