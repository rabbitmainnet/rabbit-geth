# Pre-server release gate

- Branch: `testnet-release-v1`
- Audited source commit: `d02751ecc8f7aabd87ee46b9c3a9fcfa4f388854`
- Go toolchain: `go1.25.13`
- Linux release package: `rabbit-core-testnet-linux-amd64-d02751ecc8f7.tar.gz`
- Package SHA-256: `43356e124aad186c72fed98ff4bdcabc77642658bfeb7dc9bfbb57476e5b60fa`
- Testnet genesis SHA-256: `8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7`

## Passed gates

- Clean independent source clone
- Go module integrity verification
- LCQ, vesting, miner, parameters, P2P and RPC tests
- Zero reachable vulnerabilities reported by `govulncheck`
- Zero Sonar findings hidden as Accepted or False Positive
- Zero open Sonar Blocker/High findings in critical Rabbit paths
- Native RandomX Linux build and release-package integrity

## Scope

This gate did not change LCQ, RandomX, rewards, halving, terminal subsidy,
genesis or mining eligibility. It did not start mining or public servers.
Open static-analysis findings remain publicly documented and visible.
