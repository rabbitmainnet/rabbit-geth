# Rabbit VRF DKG Handoff

Last green checkpoint: 03036d971
Branch: feat/rabbit-vrf-v0.1
Repo: ~/projects/rabbit-geth-vrf

Completed through private DKG evaluation transport and verified decryption.
Green suites: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth.

Current flow:
committee/session -> transport keys -> canonical transport set -> persistent dealer polynomial -> commitment gossip -> authenticated ShareID-to-peer routing -> encrypted private evaluation -> direct P2P delivery -> recipient validation -> transport private-key reload -> decrypt -> commitment verification.

Exact next implementation step:
Persist each verified decrypted evaluation keyed by session + recipient ShareID + dealer ShareID, with duplicate/conflict detection and restart safety. Then aggregate the complete dealer evaluation set into the recipient SecretShare, derive VerificationShare, and build/persist the final threshold keyset.

Safety:
- Rabbit VRF remains disabled on public Testnet.
- Do not configure VRFProtocolBlock.
- Do not activate a fork.
- Do not git clean or git reset --hard.
- Preserve datadir, blockchain, keystore, WorkSeats, state and backups.
- Do not claim production readiness before multinode/adversarial tests.

Preserve untracked file:
- eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken
