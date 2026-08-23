# Rabbit Core

Rabbit Core is the simple launcher for a Rabbit full node and Work V1 miner.
The normal user does not need to configure JSON-RPC flags, keystore paths or
genesis initialization.

## First run

1. Open Rabbit Core.
2. If no mining wallet exists, choose a strong local password twice.
3. Back up the encrypted wallet when instructed by the release UI.
4. Rabbit Core verifies the official genesis, initializes the data directory,
   connects to the official bootnodes and starts synchronization.
5. Mining starts automatically and stops safely with Rabbit Core.

The password and private key remain local. Only signed Work V1 tickets and
normal P2P/RPC data leave the computer.

## Package check

Release packages can be verified without starting a node or miner:

```text
rabbit-core --check
```

## Official network identity

- Chain ID: `9280`
- Network ID: `9280`
- Native coin: `RAB`
- Genesis SHA-256: `8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7`

Rabbit Core refuses a different genesis or chain ID.

## Release safety

The release package must contain `rabbit-core`, `rabbit-node`, `rabbit-miner`,
`genesis.json`, `bootnodes.txt`, documentation and `SHA256SUMS.txt`. The final
`bootnodes.txt` is created only after public P2P nodes are deployed.
