# Rabbit Chain security rebase — go-ethereum v1.17.5

This source tree rebases the Rabbit-specific consensus, Work V1, transport,
reward, and recovery code on the official go-ethereum v1.17.5 commit:

`9621c6ad10934a01b5514886fb6fbd87640b6c05`

The official Rabbit mainnet genesis is not changed. Its required SHA-256 is:

`ee0e6b167e1cd56162b55d385b998b5e75d68370fbf5959717d58f7695194f37`

## Security properties retained by the rebase

- The eth/69+ delayed RLP decoding and bounded packet handling from v1.17.5.
- A Rabbit NewBlock decoder that counts transactions, uncles, and withdrawals
  before allocating decoded item slices.
- LQC block broadcasts are rejected before block decoding on non-LQC chains.
- The default build cannot activate the frozen Rabbit mainnet Work V1 runtime.
- The laboratory work-ticket switch remains forbidden for the official Rabbit
  mainnet genesis.
- Rabbit bootnode lists remain empty until real public bootnodes are created;
  Ethereum bootnodes are selected only with an explicit Ethereum network flag.

## Release toolchain

The release gate requires official Go 1.25.13 and the already pinned Rabbit
RandomX library. Go 1.25.13 is the newer supported Go branch chosen for this
release while retaining the go-ethereum v1.17.5 module minimum of Go 1.24.

## Required gates before deployment

1. Default compilation and unit tests.
2. Production-tag compilation and unit tests with pinned RandomX.
3. `go vet` and `govulncheck` with the release Go toolchain.
4. Reproducible production/default-guard builds and GNU_STACK validation.
5. Exact genesis hash comparison.

No public bootnode, RPC, or explorer should be deployed unless every required
gate reports PASS.
