# Rabbit VRF DKG Handoff

Last green checkpoint: e6b5da684
Branch: feat/rabbit-vrf-v0.1
Repo: ~/projects/rabbit-geth-vrf

Completed through final public DKG keyset construction and persistence: canonical dealer commitments, fail-closed complete dealer set, threshold public key aggregation, deterministic transcript root, public VerificationShare derivation for every ShareID, RabbitVRFKeysetRootV1 construction, crash-safe final keyset persistence, restart recovery, duplicate idempotency and conflict rejection. Local SecretShare derivation and encrypted persistence are also complete.
Green suites: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth.

Current flow:
committee/session -> transport keys -> canonical transport set -> persistent dealer polynomial -> commitment gossip -> authenticated ShareID-to-peer routing -> encrypted private evaluation -> direct P2P delivery -> recipient validation -> decrypt/Feldman verification -> verified-evaluation persistence -> canonical evaluation aggregation -> local SecretShare -> encrypted SecretShare persistence -> canonical dealer commitments -> threshold public key -> deterministic transcript root -> public VerificationShares for all ShareIDs -> final RabbitVRF keyset root -> crash-safe final public keyset persistence.

Exact next implementation step:
Wire operational threshold signing to the persisted DKG state. Load the local SecretShare and finalized public keyset for the exact canonical session, require their ShareID/public VerificationShare to match, produce partial signatures only while the canonical secret-operation gate is ready, verify received partials against the persisted keyset, combine at threshold with CombineVerifiedPartials, and verify the final signature/randomness against the persisted threshold public key. Keep Rabbit VRF disabled on the public Testnet and do not choose VRFProtocolBlock. After runtime integration, run restart, stale-session, wrong-share, insufficient-partial, duplicate-partial, mixed-message, multinode and adversarial tests before any activation discussion.

Safety:
- Rabbit VRF remains disabled on public Testnet.
- Do not configure VRFProtocolBlock.
- Do not activate a fork.
- Do not git clean or git reset --hard.
- Preserve datadir, blockchain, keystore, WorkSeats, state and backups.
- Do not claim production readiness before multinode/adversarial tests.

Preserve untracked file:
- eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken
