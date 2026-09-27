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


## Checkpoint 2026-09-27 — threshold partial P2P transport

Code checkpoint: `d5c5fff06 feat(rabbitvrf): add threshold partial p2p transport`

Completed and green:
- canonical threshold message bound to SessionID + KeysetRoot + RequestID
- deterministic MessageHash and partial MessageID
- rvrfdkg protocol length 6 with message code 5
- authenticated ShareID-to-peer route enforcement
- final-keyset verification of every inbound partial
- stale session, wrong keyset, wrong message and invalid ShareID rejection
- duplicate partial idempotence and conflicting ShareID rejection
- per-request collector until canonical threshold
- public aggregation to final BLS signature and canonical randomness
- real MsgPipe wire test with three canonical shares and threshold completion
- full tagged suites green: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth

Important caveats:
- public Testnet Rabbit VRF remains disabled
- do not choose or configure VRFProtocolBlock
- do not activate a fork
- dealer policy remains fail-closed/all-canonical-dealers-required until complaint/qualification is implemented
- untracked `eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken` must remain untouched

## Exact next implementation step

Connect threshold signing to a CANONICAL pending Rabbit VRF request. Do not sign arbitrary RequestIDs received from P2P. First identify/read the canonical request state from RabbitVRFCoordinatorV1/core state and validate that the request exists and is pending. Then create the local threshold partial from the persisted SecretShare, build RabbitVRFThresholdPartialV1, send/gossip it through authenticated rvrfdkg code 5, and feed the local partial into the same collector path. Preserve session/keyset/request binding and reject stale or already-completed requests. After that add restart/adversarial and multinode tests before any activation discussion.


## Checkpoint 2026-09-27 — canonical pending request gate

Code checkpoint: `8b3aeb640 feat(rabbitvrf): gate threshold partials on canonical requests`

Completed and green:
- threshold partial validation now requires a canonical pending Rabbit VRF request
- production lookup uses canonical head -> StateAt -> RabbitVRFCoordinatorV1.getRequest(bytes32)
- zero/nonexistent request IDs are rejected
- non-PENDING requests are rejected
- pending requests that already contain randomness or proofHash are rejected
- code 5 cannot accept arbitrary RequestIDs without canonical request validation
- threshold wire test includes rejection of a non-canonical request
- full tagged suites green: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth

Remaining fixed blocks: 3
1. Automatic local flow: canonical pending request -> local SecretShare partial -> RabbitVRFThresholdPartialV1 -> local collector + authenticated rvrfdkg code 5 gossip
2. Canonical request finalization: write/validate randomness, proofHash, epoch/round/status exactly once
3. Robustness proof: restart/replay/adversarial tests and real 3+ node multinode validation

Important caveats:
- public Testnet Rabbit VRF remains disabled
- do not choose or configure VRFProtocolBlock
- do not activate a fork
- dealer policy remains fail-closed/all-canonical-dealers-required until complaint/qualification is implemented
- untracked `eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken` must remain untouched

## Exact next implementation step

Implement Block 2 of 4 (first of the 3 remaining blocks): automatic local threshold flow. Starting only from a canonical PENDING request, load the persisted local SecretShare and finalized keyset, build the canonical threshold message, sign the local partial, construct RabbitVRFThresholdPartialV1, feed the local partial into the same collector path used for inbound code 5 packets, and gossip it through authenticated rvrfdkg peers. Prevent duplicate local signing/gossip for the same SessionID + KeysetRoot + RequestID + ShareID and preserve restart safety. Do not finalize the on-chain request yet; that belongs to the next fixed block.


## Checkpoint 2026-09-27 — automatic local threshold flow

Code checkpoint: `4c0d030dc feat(rabbitvrf): automate local threshold partial flow`

Completed and green:
- canonical PENDING request is required before local threshold signing
- finalized DKG keyset is loaded before message/signature creation
- persisted SecretShare is used for the local partial signature
- local RabbitVRFThresholdPartialV1 is persisted restart-safely
- persisted local partial is reused after restart instead of signing again
- local partial enters the same canonical collector path used by inbound code 5 packets
- local partial is gossiped through rvrfdkg code 5
- duplicate persistence is idempotent and conflicting local partials are rejected
- focused restart/reuse test passed
- full tagged suites green: consensus/lqc, crypto/rabbitvrf, internal/rabbitvrfstate, eth

Remaining fixed blocks: 2
1. Canonical request finalization: persist/validate randomness, proofHash, epoch, round and completed status exactly once
2. Robustness proof: restart/replay/adversarial tests and real 3+ node multinode validation

Important caveats:
- public Testnet Rabbit VRF remains disabled
- do not choose or configure VRFProtocolBlock
- do not activate a fork
- dealer policy remains fail-closed/all-canonical-dealers-required until complaint/qualification is implemented
- untracked `eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken` must remain untouched

## Exact next implementation step

Implement Block 3 of 4: canonical request finalization. Starting from a completed threshold result already produced by the collector, bind the result to the canonical request and persist/validate randomness, proofHash, epoch, round and completed status exactly once. Reject stale, already-completed, mismatched or conflicting finalization attempts. Keep finalization deterministic and consensus-safe. Do not enable Rabbit VRF on public Testnet and do not set VRFProtocolBlock. After focused tests, run the full tagged suites before committing.
