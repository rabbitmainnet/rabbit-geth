# Rabbit VRF DKG Handoff

Last green checkpoint: 1dbf4140f
Branch: feat/rabbit-vrf-v0.1
Repo: ~/projects/rabbit-geth-vrf

Completed through private DKG evaluation transport, verified decryption, encrypted crash-safe verified-evaluation persistence, restart recovery, duplicate idempotency and conflict rejection.
Green suites: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth.

Current flow:
committee/session -> transport keys -> canonical transport set -> persistent dealer polynomial -> commitment gossip -> authenticated ShareID-to-peer routing -> encrypted private evaluation -> direct P2P delivery -> recipient validation -> transport private-key reload -> decrypt -> Feldman commitment verification -> encrypted verified-evaluation persistence -> restart recovery / duplicate / conflict protection.

Exact next implementation step:
Load the complete verified evaluation set for each local recipient across all canonical dealers. Aggregate the canonical BLS12-381 Fr evaluations into the recipient SecretShare, fail closed if any required dealer evaluation is missing or if the final aggregate is zero, derive the VerificationShare, then persist the final local secret-share/keyset state crash-safely before enabling threshold signing.

Safety:
- Rabbit VRF remains disabled on public Testnet.
- Do not configure VRFProtocolBlock.
- Do not activate a fork.
- Do not git clean or git reset --hard.
- Preserve datadir, blockchain, keystore, WorkSeats, state and backups.
- Do not claim production readiness before multinode/adversarial tests.

Preserve untracked file:
- eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken
