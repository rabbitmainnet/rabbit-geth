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
## Checkpoint 2026-09-28 — Block 3 canonical finalization in progress

Repo: ~/projects/rabbit-geth-vrf
Branch: feat/rabbit-vrf-v0.1
Rabbit VRF remains disabled on public Testnet. Do NOT set VRFProtocolBlock or schedule a fork.

Fixed macro-blocks:
3. Canonical request finalization — IN PROGRESS
4. Robustness proof — restart/replay/adversarial + real 3+ node multinode

Completed/green in Block 3:
- V5 producer and validator paths integrated.
- RabbitVRFFinalizationV1 exists and now requires RequestID, KeysetRoot, Epoch, Round, Randomness, ProofHash and aggregate rabbitvrf.Signature.
- Finalization canonical ordering/dedup tests pass.
- rabbitvrf.VerifyAndDeriveRandomness verifies aggregate BLS and derives randomness.
- Collector already produces aggregate Signature + Randomness.
- Local DKG keyset files are NOT acceptable as consensus truth.
- RabbitVRFKeysetCertificateV1 added.
- DKG envelope message type RabbitVRFDKGMessageKeysetCertificateV1 = 4 added.
- Certificate uses compact ordered wallet signatures, V1 fail-closed/all-canonical-members.
- ValidateRabbitVRFKeysetCertificateShapeV1 exists.
- LQCHeaderEnvelopeV5 now has RabbitVRFKeysetCertificates []RabbitVRFKeysetCertificateV1.
- MaxRabbitVRFKeysetCertificatesPerBlockV1 = 1.
- Focused tests currently green:
  go test ./consensus/lqc -run 'RabbitVRFKeyset|RabbitVRFFinalizationV1|LQCHeaderExtraV5' -count=1
- Keep eth/rabbit_vrf_dkg_polynomial_transport_lab_test.go.broken untouched.

Consensus safety decision:
Never mutate canonical state from P2P. Correct path:
collector -> local candidate -> producer commits V5 proof/keyset data -> validators verify deterministically -> core executes systemFinalizeRequest deterministically.

Exact next implementation step:
1. Finish V5 encoding/decoding for 0 or 1 RabbitVRFKeysetCertificateV1 with canonical byte-for-byte re-encoding.
2. Extend V5 runtime builder/provider to publish certificate once.
3. Validate certificate against canonical DKG session + committee members from chain context.
4. Resolve certified KeysetRoot -> ThresholdPublicKey from canonical chain history, never local disk for consensus.
5. Validate every finalization: matching keyset/epoch, canonical threshold message, VerifyAndDeriveRandomness, derived Randomness equality, derived ProofHash equality, canonical request PENDING.
6. Expose validated finalizations to core via small consensus interface; avoid core importing lqc.
7. Execute coordinator systemFinalizeRequest deterministically in core using existing system-call/EVM pattern.
8. Regenerate/verify coordinator artifact after Solidity change.
9. Run focused tests, git diff --check, full tagged suites including ./core.
10. Commit Block 3 and update handoff. Then only Block 4 remains.

Do NOT commit Block 3 yet.

Next-chat instruction:
Leia docs/vrf/rabbit-vrf-dkg-handoff.md e continue do "Exact next implementation step". Estamos no repo ~/projects/rabbit-geth-vrf, branch feat/rabbit-vrf-v0.1.

## Checkpoint 2026-09-28 — Block 3 V5 keyset certificate progress

New green progress:
- LQCHeaderEnvelopeV5 carries RabbitVRFKeysetCertificates.
- Maximum 1 keyset certificate per block.
- Encode/decode validates certificate shape and preserves it in canonical byte-for-byte re-encoding.
- EncodeLQCHeaderExtraV5WithKeysetCertificate exists.
- BuildLQCHeaderExtraV5WithCanonicalRuntimeAndKeysetCertificateV1 exists.
- WorkV1EngineLabRabbitVRFKeysetCertificateProvider and setter added.
- Prepare path now chooses V5 builder with certificate when provider returns hasCertificate=true.
- Old V5 path without certificate remains supported.
- Focused tests green after this step.

Still in Block 3/4. Do NOT commit or activate fork yet.

Real next security step:
- Validate the published Keyset Certificate against the canonical Rabbit VRF DSG session and canonical committee members.
- Never trust local final keyset disk state for consensus.
- Then resolve certified KeysetRoot -> ThresholdPublicKey from canonical chain history.
- Then validate each RabbitVRFFinalizationV1 cryptographically: keyset root, epoch, threshold message, aggregate BLS signature, derived randomness, proof hash, and canonical PENDING request.
- Then expose only validated finalizations to core and execute systemFinalizeRequest deterministically.

The fixed macro-block count remains:
3. Canonical request finalization — IN PROGRESS
4. Robustness proof — restart/replay/adversarial + real 3+ node multinode

Next-chat instruction:
Leia docs/vrf/rabbit-vrf-dkg-handoff.md e continue do trecho mais recente. Repo: ~/projects/rabbit-geth-vrf. Branch: feat/rabbit-vrf-v0.1.


## Checkpoint 2026-09-28 — Block 3 validated finalization cache

Current macro state: Block 3/4 Canonical request finalization is still in progress. Rabbit VRF remains disabled on public Testnet. Do not set VRFProtocolBlock and do not schedule a fork.

New green progress:
- V5 Keyset Certificate is validated against canonical DKG bridge/session/members resolved from canonical chain history.
- rabbitVRFDKGBridgeForTargetBlockV1 resolves target VRF epoch -> source Work epoch -> DKG preparation block -> canonical ancestor -> canonical DKG bridge.
- rabbitVRFKeysetCertificateForRootV1 resolves KeysetRoot from the current V5 envelope or canonical V5 ancestors within the same VRF epoch.
- ValidateRabbitVRFFinalizationProofV1 validates KeysetRoot, target Epoch, canonical threshold message, aggregate BLS signature, derived randomness and RabbitVRFFinalizationProofHashV1.
- The active V5 verification path now validates each RabbitVRFFinalizationV1 against a canonically certified threshold public key before accepting the header.
- workV1EngineLabRuntime now has validatedFinalizations map[common.Hash][]RabbitVRFFinalizationV1.
- workV1EngineLabRememberRabbitVRFFinalizations and RabbitVRFValidatedFinalizationsV1 were added and focused tests are green.

Latest focused test result:
ok github.com/ethereum/go-ethereum/consensus/lqc

Exact next implementation step:
After the entire V5 validation path succeeds, store v5Envelope.RabbitVRFFinalizations under header.Hash() using workV1EngineLabRememberRabbitVRFFinalizations. Then add an optional neutral interface in consensus for retrieving validated Rabbit VRF finalizations by block hash, implement it on LQC, and consume it from core StateProcessor via p.chain.Engine() without importing lqc into core. Core must deterministically verify canonical request PENDING/epoch/round and execute systemFinalizeRequest. Do not let P2P mutate canonical state.

Remaining inside fixed Block 3 only:
1. finish engine -> consensus -> core bridge for validated finalizations;
2. deterministic systemFinalizeRequest with canonical PENDING + epoch + round validation;
3. real Keyset Certificate signature production/gossip/collection/provider;
4. coordinator artifact + header size/focused/full tagged tests + Block 3 commit.
Then Block 4 only: restart/replay/adversarial + real 3+ node multinode.

## Checkpoint 2026-09-28 — Block 3 build restored

- Tagged build restored green after V5/finalization integration fixes.
- Fixed ValidateRabbitVRFKeysetCertificateV1 two-value returns.
- Removed misplaced validated-finalization cache insertion from V4 replay path.
- Fixed epoch length lookup to v4ctx.Work.Parent.Work.EpochLength.
- rabbitVRFDKGTransport now has a single finalizations map[common.Hash]lqc.RabbitVRFFinalizationV1 cache initialized in constructor.
- Focused tagged tests green:
  - consensus/lqc: OK
  - eth: OK
- Canonical pending request semantics confirmed: epoch=0 and round=0 are placeholders only while PENDING.
- Protocol spec still leaves canonical round construction OPEN.
- Current V1 implementation direction under evaluation: bind Epoch to CanonicalSession.TargetVRFEpoch and define a deterministic request-derived Round before constructing RabbitVRFFinalizationV1.
- Exact next implementation step: construct real RabbitVRFFinalizationV1 from threshold result + canonical request/session and expose it through the existing V5 finalization provider path.
- Rabbit VRF remains disabled on public Testnet. No VRFProtocolBlock or fork activation has been configured.
