# Rabbit VRF Protocol Specification v0.1

Status: DRAFT / NOT ACTIVATABLE
Protocol: Rabbit VRF
Network integration: Rabbit Chain / LCQ
Activation block: UNSET
Mainnet readiness: NO

## 1. Purpose

Rabbit VRF provides publicly verifiable, bias-resistant randomness generated
natively by Rabbit Chain.

The protocol is intended to serve:

- Rabbit Chain smart contracts.
- External EVM applications.
- Web2 applications through verifiable gateways.
- Batch randomness consumers.
- Games, NFTs, lotteries, DeFi, DAOs, selection systems and other applications
  requiring independently verifiable randomness.

Rabbit VRF MUST NOT depend on a trusted operator, founder-controlled key,
producer-selected entropy, or an unverifiable off-chain random source.

## 2. Activation rule

Rabbit VRF MUST remain disabled until every consensus-critical item in this
specification is resolved and the activation checklist is complete.

While this document contains any consensus-critical item marked OPEN:

- Testnet vrfProtocolBlock MUST remain unset or disabled.
- Mainnet vrfProtocolBlock MUST remain unset or disabled.
- No release may describe Rabbit VRF as consensus-active.
- No fallback may silently weaken the cryptographic protocol.
- Implementation testing may use local/devnet artificial activation heights.

A public hard fork is the activation of already completed code.
A public hard fork MUST NOT be used as the development environment.

## 3. Core security invariants

The final protocol MUST preserve all of the following.

1. No block producer may choose between multiple valid randomness outputs.
2. No aggregator may choose randomness by selecting a favorable valid subset
   of threshold participants.
3. A valid threshold signature for one canonical message MUST be unique.
4. The same canonical message and epoch key MUST produce the same final
   threshold signature regardless of which valid threshold subset reconstructed
   it.
5. Participant ordering MUST NOT change the threshold signature.
6. Randomness MUST NOT be derived from blockhash alone.
7. Randomness MUST NOT be derived from RandomX output alone.
8. Randomness MUST NOT fall back to ordinary independent signatures when the
   threshold protocol fails.
9. A request MUST be committed before its randomness becomes knowable.
10. Results MUST be domain-separated across chain, protocol version, epoch,
    round and application/request context.
11. Reorg handling MUST be deterministic.
12. Restart/recovery MUST NOT create a second valid canonical result for an
    already finalized round.
13. An invalid or malicious participant MUST NOT be able to make honest nodes
    accept a different committee, threshold key, round message or output.
14. Rabbit VRF MUST NOT weaken LCQ producer fairness or LCQ liveness.
15. No administrator may replace a threshold key or randomness result
    arbitrarily.

## 4. Cryptographic profile

Rabbit VRF V1 has a byte-frozen cryptographic profile.

Profile identifier:

    RABBIT-VRF-BLS12381-G1PK-G2SIG-V1

Frozen parameters:

- Curve: BLS12-381.
- Scalar field: Fr.
- Public key group: G1.
- Signature and partial-signature group: G2.
- Secret encoding: exactly 32-byte canonical big-endian Fr, non-zero.
- Public key encoding: exactly 48-byte compressed G1.
- Signature encoding: exactly 96-byte compressed G2.
- Message mapping: HashToG2.
- HashToG2 DST:
  `RABBIT-VRF-BLS12381G2_XMD:SHA-256_SSWU_RO_V1`.
- Randomness domain:
  `RABBIT-VRF-RANDOMNESS-V1`.
- Randomness:
  `Keccak256(domain || message || compressed_signature_96)`.

Public keys and signatures MUST:

- use the byte-frozen canonical encodings;
- be on curve;
- be in the correct subgroup;
- reject the point at infinity.

Pairing inputs MUST be validated before PairingCheck.

Canonical interoperability vectors are published in:

    docs/consensus/rabbit-vrf-crypto-profile-v1.md

Executable byte-for-byte conformance tests are in:

    crypto/rabbitvrf/vectors_test.go

A library or implementation change MUST NOT change any V1 vector byte.

OPEN:
- Reproduce the frozen vectors with an independent implementation.
- Obtain dedicated cryptographic review before Mainnet.

## 5. Threshold BLS

Rabbit VRF uses one distributed threshold public key per active VRF epoch.

For threshold t-of-n:

- Each member owns one secret share.
- Each member has one corresponding public verification share.
- Each partial signature is bound to one immutable ShareID.
- Duplicate ShareIDs MUST be rejected.
- ShareID zero MUST be invalid.
- Relabeling a valid partial under another ShareID MUST be rejected before
  aggregation.
- Invalid partial signatures MUST be rejected before aggregation.
- Fewer than t valid partials MUST NOT produce an output.
- Lagrange interpolation is performed at x=0.
- Different valid subsets of size >= t MUST reconstruct the same threshold
  signature.
- Different valid ordering of the same shares MUST reconstruct the same
  threshold signature.

The reconstructed threshold signature MUST verify against the epoch threshold
public key.

For deterministic test-only Shamir polynomials, the reconstructed threshold
signature SHOULD equal byte-for-byte the direct BLS signature produced by the
corresponding test master secret.

A centralized Shamir dealer MUST NOT exist in production.

Completed test coverage:
- Property tests cover multiple thresholds, committee sizes and ShareID sets,
  including 1-of-1, 1-of-n, t-of-n, threshold-equals-n, irregular IDs,
  near-MaxUint64 IDs, subset variation and participant reordering.
- Property tests reject duplicate verified tokens, insufficient shares and
  invalid thresholds.
- Fuzz targets cover threshold reconstruction across varying t/n, ShareID
  ranges, ordering and messages.
- Fuzz targets cover relabeled ShareIDs, mutated partial signatures and
  canonical G1/G2 decoding of untrusted byte encodings.

OPEN:
- Extend fuzz mutation coverage specifically for duplicate verified tokens and
  insufficient-share reconstruction inputs.
- Run longer fuzz campaigns and retain any discovered regression corpus.

## 6. Distributed Key Generation

Production Rabbit VRF MUST use a decentralized DKG or equivalent distributed
threshold-key establishment protocol.

There MUST NOT be a process in which one trusted machine learns the complete
production epoch secret key.

The DKG design MUST define:

- participant set;
- participant index / ShareID assignment;
- threshold;
- polynomial commitment scheme;
- share transport;
- authentication;
- confidentiality requirements;
- complaint handling;
- invalid-share handling;
- timeout handling;
- absent participant handling;
- disqualification rules;
- qualified participant set;
- final threshold public key derivation;
- public verification share derivation;
- transcript commitment;
- persistence;
- restart recovery;
- equivocation detection;
- epoch transition;
- resharing or fresh-DKG policy.

The final DKG transcript MUST be independently derivable or verifiable from
canonical protocol data.

OPEN:
- Select and formally specify the DKG construction.
- Decide fresh DKG versus resharing policy.
- Define complaint and qualification state machine.
- Define malicious-participant threshold assumptions.
- Define secure local storage of secret shares.
- Define crash-consistent DKG persistence.

## 7. LCQ / WorkSeat integration

Rabbit VRF membership MUST derive from canonical Rabbit consensus state.

Candidate membership source:

- canonical WorkSeatV1 set;
- deterministic Rabbit selection;
- no producer-controlled selection seed;
- no process-local participant list;
- no gateway-controlled membership.

A Rabbit wallet MUST NOT gain additional VRF selection weight merely by using
a faster machine.

VRF committee derivation MUST define exactly:

- source Work epoch;
- canonical source root;
- eligible-seat rules;
- liveness eligibility;
- jailed participant behavior;
- committee size;
- threshold;
- deterministic ordering;
- ShareID assignment.

Committee derivation MUST produce the same result on every honest node from the
same canonical chain state.

OPEN:
- Freeze VRF committee selection algorithm.
- Freeze committee size.
- Freeze threshold formula.
- Define handling of unavailable/jail-transition seats.
- Define exact source epoch/root.

## 8. Epoch model

Rabbit Work V1 uses 128-block epochs.

Rabbit VRF MUST NOT change the Work V1 epoch length.

Rabbit VRF MAY use a longer epoch composed of an integer number of Work epochs.

A VRF epoch transition MUST NOT occur until the next threshold key is ready and
canonically committed.

The old and new epoch rules MUST define an unambiguous transition boundary.

No round may begin under one epoch key and finalize under another.

OPEN:
- Benchmark DKG on realistic committee sizes.
- Select VRF epoch duration.
- Define preparation window.
- Define activation boundary.
- Define overlap rules.
- Define failure behavior when the next epoch key is not ready.

## 9. Canonical round message

Every threshold signature MUST sign one canonical Rabbit VRF message.

Candidate logical fields include:

- protocol domain;
- protocol version;
- chain ID;
- VRF epoch;
- round or batch identifier;
- threshold-key commitment;
- committee commitment;
- finalized parent/anchor reference;
- previous finalized Rabbit randomness;
- request root;
- optional application-domain commitment.

Encoding MUST be canonical and byte-for-byte deterministic.

No producer-controlled mutable field may be included if changing that field
allows grinding for a favorable output.

OPEN:
- Freeze exact field set.
- Freeze field ordering.
- Freeze integer encoding.
- Freeze hash/serialization construction.
- Define finalized anchor rule.
- Define previous-randomness chaining rule.

## 10. Randomness derivation

The final random seed MUST be derived only after threshold-signature
verification.

Candidate construction:

    seed = Keccak256(
        randomness_domain ||
        canonical_message ||
        compressed_threshold_signature
    )

Per-request outputs SHOULD derive from the finalized seed rather than requiring
one threshold signature for every application request.

Candidate construction:

    output_i = Keccak256(
        request_domain ||
        seed ||
        request_commitment ||
        request_index
    )

Range mapping MUST avoid modulo bias.

OPEN:
- Freeze exact seed construction.
- Freeze request-output construction.
- Freeze unbiased integer-range mapping.
- Produce interoperability vectors.

## 11. Request commitment and batching

A request MUST be committed before its output becomes knowable.

Batching MAY be used for throughput.

A canonical batch MUST define:

- request identifier;
- application identifier;
- chain/domain identifier;
- request nonce;
- payload commitment;
- number of requested words;
- deadline/expiry if applicable;
- deterministic ordering;
- Merkle construction;
- duplicate handling;
- maximum batch size.

Gateways MUST NOT be able to omit and retry a request for a more favorable
result without leaving verifiable evidence or incurring the protocol-defined
cost.

OPEN:
- Freeze request leaf.
- Freeze canonical ordering.
- Freeze Merkle construction.
- Define admission cutoff.
- Define omitted-request behavior.
- Define retry semantics.

## 12. Round state machine

Every node MUST implement the same deterministic round state machine.

The state machine MUST define at least:

- pending;
- collecting partials;
- threshold reached;
- reconstructed;
- verified;
- provisional;
- finalized;
- expired/failed where applicable.

The protocol MUST define exactly what happens if threshold is not reached.

There MUST NOT be an insecure randomness fallback.

OPEN:
- Freeze round timing.
- Freeze retry/rebroadcast policy.
- Freeze missing-threshold behavior.
- Freeze canonical result acceptance rule.

## 13. Reorg and finality

Rabbit VRF MUST explicitly distinguish provisional and finalized results.

The protocol MUST define:

- canonical anchor block;
- required confirmation/finality rule;
- behavior when an anchor is reorged;
- behavior when a request root is reorged;
- behavior when a provisional signature exists on an abandoned fork;
- whether the same request may be reconstructed on the new fork;
- API status representation.

A proof from an abandoned fork MUST NOT be presented as finalized randomness.

OPEN:
- Freeze finality depth/rule.
- Freeze reorg state transitions.
- Add adversarial reorg tests around epoch and fork boundaries.

## 14. P2P protocol

Rabbit VRF P2P messages MUST be versioned and bounded.

Potential message classes:

- DKG announcement;
- DKG commitment;
- encrypted/private share transport metadata;
- complaint;
- qualification result;
- partial signature;
- reconstructed threshold signature;
- round status.

Every message type MUST define:

- version;
- epoch;
- round;
- sender identity;
- maximum encoded size;
- authentication;
- replay protection;
- duplicate policy;
- validation order;
- rate limiting.

Expensive pairing operations MUST NOT be reachable through unlimited unauthenticated
spam.

OPEN:
- Freeze wire protocol.
- Define rate limits.
- Define deduplication keys.
- Define peer penalty rules.
- Benchmark adversarial pairing load.

## 15. Persistence and restart recovery

The node MUST persist enough Rabbit VRF state to recover deterministically after
restart without creating conflicting canonical state.

Persistence MUST cover where applicable:

- current epoch metadata;
- local secret share;
- public verification shares;
- DKG transcript/state;
- committee commitment;
- threshold public key;
- completed rounds;
- pending rounds;
- finalized randomness;
- replay/deduplication state.

Secret shares MUST NOT be written to logs.

Backup and restore procedures MUST avoid accidentally cloning one participant's
identity/share onto multiple active nodes.

OPEN:
- Choose storage format.
- Define encryption-at-rest policy.
- Define database schema/version.
- Define atomic write boundaries.
- Test kill -9 at every state transition.
- Test restore/restart across epoch transitions.

## 16. LCQ liveness isolation

Rabbit VRF MUST NOT make normal LCQ block production depend on an unrelated
application randomness request.

A VRF outage MUST NOT silently stop Rabbit Chain block production unless a
future explicitly specified consensus feature intentionally requires VRF.

VRF networking, DKG and pairing workloads MUST be resource-bounded so that they
cannot starve mining or consensus processing.

OPEN:
- Define scheduler/resource isolation.
- Benchmark low-end hardware.
- Test LCQ during VRF flood/failure.

## 17. EVM integration

The final Rabbit EVM interface MUST provide deterministic verification and
request/result access.

Candidate paths:

- Rabbit-specific precompile;
- native protocol/system interface;
- contracts consuming canonical Rabbit VRF state.

Rabbit MUST NOT enable unrelated Prague behavior solely to obtain BLS support.

The interface MUST define:

- request ABI;
- result ABI;
- proof ABI;
- gas accounting;
- failure/revert behavior;
- maximum request size;
- batch limits;
- historical result access;
- verification method.

OPEN:
- Select final EVM architecture.
- Freeze addresses/selectors if applicable.
- Freeze gas schedule.
- Add state-transition tests.

## 18. Web2 / external-chain interface

External users MAY consume Rabbit VRF through gateways, but correctness MUST
remain independently verifiable.

Gateway responses SHOULD include enough data to verify:

- Rabbit chain ID;
- epoch;
- round;
- request commitment;
- final seed/output;
- threshold signature;
- threshold public key or canonical reference;
- anchor/finality information.

Gateway signatures MUST NOT substitute for Rabbit threshold proofs.

OPEN:
- Freeze receipt format.
- Define REST API.
- Define SDK verification flow.
- Define external-chain relay/proof format.

## 19. Fees and gas

Rabbit VRF service pricing MUST be separate from cryptographic validity.

A payment failure MUST NOT allow manipulation of an already committed random
result.

The service MAY abstract gas from end users, but network computation is not
free and MUST be metered.

OPEN:
- Finalize RAB-denominated payment flow.
- Finalize USD reference mechanism if retained.
- Finalize prepaid-credit design if retained.
- Define stale-price behavior.
- Define relayer reimbursement.
- Define protocol revenue accounting.

## 20. Security and adversarial testing

Before public activation, tests MUST include:

- malformed G1/G2 points;
- infinity points;
- wrong subgroup;
- non-canonical scalar encoding;
- zero scalar;
- wrong message;
- wrong key;
- wrong epoch;
- wrong chain ID;
- duplicate ShareID;
- ShareID relabel;
- insufficient shares;
- invalid partial;
- partial withholding;
- equivocation;
- participant offline;
- malicious DKG dealer contribution;
- complaint flood;
- duplicate P2P messages;
- oversized messages;
- pairing DoS;
- reorg before threshold;
- reorg after provisional result;
- restart before/after DKG completion;
- restart during round;
- partition and reunification;
- stale peer;
- node syncing across VRF activation;
- old node versus new node behavior;
- fork boundary -1 / exact / +1;
- fresh sync from genesis;
- multi-node deterministic convergence.

Fuzz tests MUST cover all protocol decoders exposed to untrusted network input.

## 21. Platform release matrix

Official release validation MUST include:

- Linux amd64.
- Windows amd64.
- macOS arm64.
- macOS amd64.

Each platform MUST test:

- BLS encoding;
- BLS verification;
- threshold reconstruction;
- deterministic vectors;
- persistence/restart;
- official Rabbit build tags;
- RandomX coexistence where relevant.

## 22. Observability

Nodes SHOULD expose Rabbit VRF operational state without exposing secret
material.

Metrics/logs SHOULD include:

- active VRF epoch;
- committee hash;
- local membership status;
- DKG phase;
- qualified member count;
- partial count;
- threshold;
- round state;
- finalized round;
- failure reason;
- malformed/invalid message counters.

Secret shares and private DKG material MUST NEVER appear in logs or RPC output.

## 23. Fork compatibility

Rabbit VRF activation MUST be guarded by ChainConfig.

Required compatibility testing:

- disabled configuration;
- pre-fork block;
- exact fork block;
- post-fork block;
- incompatible configuration detection;
- persisted chain restart;
- sync batch crossing activation;
- reorg crossing activation;
- fresh sync from genesis;
- old binary behavior at activation.

The activation height MUST NOT be selected until the implementation is frozen.

## 24. Release gate

Rabbit VRF may receive a public Testnet activation block only when all of the
following are true:

- No consensus-critical OPEN item remains.
- Protocol message encoding is frozen.
- Cryptographic DSTs are frozen.
- DKG is implemented and tested.
- Committee and threshold rules are frozen.
- Epoch transition is implemented and tested.
- Reorg/finality rules are implemented and tested.
- Persistence/restart is implemented and tested.
- P2P protocol is implemented and adversarially tested.
- EVM/API interface is implemented and tested.
- Multi-node tests pass.
- Four official release platforms pass.
- Full Rabbit consensus suite passes.
- Fuzz/adversarial suites pass.
- Deterministic vectors are published.
- Security review finds no unresolved critical/high issue.
- Upgrade/recovery procedure is documented.
- Explorer/RPC compatibility is validated.

Only after this gate passes may vrfProtocolBlock be assigned a future public
Testnet block.

Mainnet activation requires an additional Mainnet readiness review after
Testnet observation.

## 25. Current implementation state

Completed:

- Disabled Rabbit VRF ChainConfig plumbing.
- BLS12-381 primitive package.
- Non-zero Fr secret generation.
- Canonical secret decoding.
- G1 public keys.
- G2 signatures.
- HashToG2.
- Pairing verification.
- Infinity rejection.
- Wrong-key/wrong-message rejection.
- Deterministic randomness derivation.
- Basic threshold partial signing prototype.
- Lagrange interpolation prototype.
- Different 3-of-5 subsets reconstruct identical threshold signature in tests.
- Different 3-of-5 subsets derive identical randomness in tests.
- VerificationShare validates non-zero ShareID and canonical G1 public key.
- Raw PartialSignature values cannot enter the public reconstruction API directly.
- VerifyPartial binds ShareID, verification public key, message and signature.
- Verified partials from different messages are rejected during reconstruction.
- Duplicate and zero ShareIDs are rejected.
- Relabeled partial signatures are rejected.
- Participant ordering does not alter the reconstructed signature.
- More than threshold verified shares use a deterministic lowest-ShareID subset.
- Test threshold signatures equal the direct master BLS signature byte-for-byte.
- Reconstructed signatures are verified against the threshold public key before return.
- Wrong threshold public keys are rejected.
- Rabbit VRF V1 cryptographic profile is frozen byte-for-byte.
- HashToG2 DST and randomness domain are immutable V1 constants.
- Three base interoperability vectors are frozen.
- Fr modulus boundary rejection is frozen by test.
- One complete 3-of-5 threshold interoperability vector is frozen.
- Frozen vectors are enforced by executable conformance tests.
- Threshold property tests cover multiple t-of-n configurations, committee
  sizes, ShareID sets, subset variation, ordering, duplicate tokens,
  insufficient shares and invalid thresholds.
- Threshold fuzz targets cover reconstruction, relabeled ShareIDs, mutated
  partial signatures and canonical G1/G2 decoding.
- Initial fuzz campaigns for all five Rabbit VRF fuzz targets complete without
  panic, crash or invariant failure.
- Frozen Rabbit VRF V1 vectors pass natively on Linux amd64, Windows amd64,
  macOS arm64 and macOS amd64 in the dedicated Testnet interoperability CI.

In progress:

- Independent implementation reproduction of frozen cryptographic vectors.

Not started / OPEN:

- Production DKG.
- VRF committee definition.
- Threshold formula.
- VRF epoch duration.
- Canonical round message.
- Request batching.
- Round state machine.
- P2P transport.
- Persistence.
- Reorg/finality implementation.
- EVM interface.
- External API.
- Fee path.
- Multi-node adversarial suite.
- External security review.

## 26. Non-negotiable activation principle

Do not schedule the fork because the feature appears to work.

Schedule the fork only after the protocol is specified, the implementation is
frozen, all consensus-critical tests pass, recovery is proven, multi-node
behavior is deterministic, and no known critical protocol question remains.

Until then:

    vrfProtocolBlock = disabled
