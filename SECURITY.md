# Rabbit Chain Security Policy

## Supported versions

Rabbit Chain is currently preparing its public testnet. Only binaries attached to releases in the official [`rabbitmainnet/rabbit-geth`](https://github.com/rabbitmainnet/rabbit-geth/releases) repository are distributed by the project.

Preview builds are pre-release software and must not be treated as a mainnet release.

## Reporting a vulnerability

Do not open a public issue containing a vulnerability, exploit, private key, seed phrase, password, server credential, or other sensitive information.

Report security issues privately using [GitHub Security Advisories](https://github.com/rabbitmainnet/rabbit-geth/security/advisories/new).

Please include:

- affected commit, tag, or release;
- affected component and operating system;
- steps to reproduce;
- expected and observed behavior;
- potential impact;
- proof of concept, if safe to share privately.

Do not test an exploit against public infrastructure or other users. Use an isolated environment and fresh test-only keys.

## Release verification

Before running Rabbit software:

1. download only from the official GitHub releases page;
2. verify the archive against `SHA256SUMS.txt` or its individual `.sha256` file;
3. run `rabbit-core --check` before starting;
4. confirm the official Chain ID and genesis hash;
5. confirm that the release tag points to the announced source commit.

Rabbit Testnet identity:

- Chain ID: `9280` (`0x2440`)
- Network ID: `9280`
- Genesis SHA-256: `8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7`

## Key safety

- Never share a seed phrase, raw private key, keystore password, JWT secret, SSH key, or server credential.
- Keep mining wallets separate from wallets holding significant value.
- Store encrypted keystore backups offline.
- Public RPC endpoints must not expose `personal`, `admin`, `debug`, authenticated RPC, or IPC.
- Rabbit Core binds its local JSON-RPC endpoint to `127.0.0.1` by default.

## Upstream reports

Historical go-ethereum audit reports retained in this repository cover upstream components only. They are not audits of Rabbit Chain's LCQ consensus, Rabbit Work V1, registry, rewards, vesting, or release infrastructure.
