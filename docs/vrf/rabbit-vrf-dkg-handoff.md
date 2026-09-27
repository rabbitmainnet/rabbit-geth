# Rabbit VRF DKG Handoff

Last green checkpoint: 389910313
Branch: feat/rabbit-vrf-v0.1
Repo: ~/projects/rabbit-geth-vrf

Completed through private DKG evaluation transport, verified evaluation persistence, canonical evaluation aggregation, local SecretShare derivation, VerificationShare derivation, encrypted crash-safe SecretShare persistence, restart recovery, duplicate idempotency and conflict rejection.
Green suites: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth.

Current flow:
committee/session -> transport keys -> canonical transport set -> persistent dealer polynomial -> commitment gossip -> authenticated ShareID-to-peer routing -> encrypted private evaluation -> direct P2P delivery -> recipient validation -> transport private-key reload -> decrypt -> Feldman commitment verification -> encrypted verified-evaluation persistence -> load complete canonical dealer evaluation set -> aggregate in BLS12-381 Fr -> local SecretShare -> VerificationShare -> encrypted restart-safe SecretShare persistence.

Exact next implementation step:
Build the final public DKG keyset from canonical dealer polynomial commitments. Derive the threshold public key from the aggregate of the dealers constant coefficient commitments, derive/publish canonical VerificationShares for committee members, define the transcript root over the finalized DKG transcript, call RabbitVRFKeysetRootV1, persist the finalized public keyset crash-safely, and only then wire the persisted local SecretShare into threshold signing. Do not derive the threshold public key from a local SecretShare.

Safety:
- Rabbit VRF remains disabled on public Testnet.
- Do not configure VRFProtocolBlock.
- Do not activate a fork.
- Do not git clean or git reset --hard.
- Preserve datadir, blockchain, keystore, WorkSeats, state and backups.
- Do not claim production readiness before multinode/adversarial tests.

Preserve untracked file:
- eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken
