# Rabbit Miner Work V1

`rabbit-miner` performs Rabbit Work V1 locally and submits successful RandomX
tickets to a Rabbit node. It does not transmit the wallet password or private
key. Only the participant address, nonce, proof hash and signature are sent.

## Security

- Use an encrypted Web3 keystore dedicated to mining.
- Never enter a seed phrase or raw private key into a website.
- Keep the password file local, permission-restricted and outside the package.
- Verify `SHA256SUMS.txt` before running a release.
- Verify the chain ID. The official Rabbit Testnet chain ID is `9280`.

## Usage

```text
rabbit-miner \
  --rpc http://127.0.0.1:8545 \
  --keystore /path/to/UTC--encrypted-key \
  --password-file /path/to/password.txt
```

The default is one successful ticket per epoch. Advanced users can change this
with `--tickets-per-epoch`. A value of `0` keeps searching without a per-epoch
limit.

The miner waits safely when the node has no active Work V1 commit window. It
automatically resumes when a valid context becomes available.

## Components of Rabbit Core

The Rabbit Core Testnet v1 package contains:

- the Rabbit node (`rabbit-node`);
- the Work V1 miner (`rabbit-miner`);
- the frozen official `genesis.json`;
- a launcher for the operating system;
- checksums and security documentation.

Public bootnodes and RPC domains are frozen for Rabbit Core Testnet v1.
Bootnodes provide discovery only and have no consensus or administrative authority.
