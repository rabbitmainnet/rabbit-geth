# Rabbit VRF DKG Handoff

Last green checkpoint: 5e3339bd7
Branch: feat/rabbit-vrf-v0.1
Repo: ~/projects/rabbit-geth-vrf

Completed through final public DKG keyset construction and persistence: canonical dealer commitments, fail-closed complete dealer set, threshold public key aggregation, deterministic transcript root, public VerificationShare derivation for every ShareID, RabbitVRFKeysetRootV1 construction, crash-safe final keyset persistence, restart recovery, duplicate idempotency and conflict rejection. Local SecretShare derivation and encrypted persistence are also complete.
Green suites: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth.

Current flow:
committee/session -> transport keys -> canonical transport set -> persistent dealer polynomial -> commitment gossip -> authenticated ShareID-to-peer routing -> encrypted private evaluation -> direct P2P delivery -> recipient validation -> decrypt/Feldman verification -> verified-evaluation persistence -> canonical evaluation aggregation -> local SecretShare -> encrypted SecretShare persistence -> canonical dealer commitments -> threshold public key -> deterministic transcript root -> public VerificationShares for all ShareIDs -> final RabbitVRF keyset root -> crash-safe final public keyset persistence -> persisted SecretShare/keyset match -> SignPartial -> VerifyPartial -> threshold CombineVerifiedPartials -> final signature verification -> canonical randomness derivation.

Exact next implementation step:
Implement authenticated P2P transport for Rabbit VRF threshold partial signatures using the finalized DKG session and keyset. Bind every partial to the exact canonical session, request/message identity and ShareID; accept only authenticated committee ShareIDs from the finalized keyset; reject stale-session, wrong-message, duplicate/conflicting ShareID and malformed partials; verify each received partial before aggregation; collect only until the canonical threshold is reached; reconstruct with CombineVerifiedPartials and derive randomness only after final threshold-signature verification. Then test restart recovery, insufficient partials, duplicate/conflicting partials, stale-session, wrong-share, mixed-message, malformed signatures, network interruption/reconnect and real multinode behavior. Keep Rabbit VRF disabled on the public Testnet and do not choose VRFProtocolBlock.

Safety:
- Rabbit VRF remains disabled on public Testnet.
- Do not configure VRFProtocolBlock.
- Do not activate a fork.
- Do not git clean or git reset --hard.
- Preserve datadir, blockchain, keystore, WorkSeats, state and backups.
- Do not claim production readiness before multinode/adversarial tests.

Preserve untracked file:
- eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken
