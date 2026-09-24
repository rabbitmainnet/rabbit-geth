# Rabbit VRF Testnet V0.1 - Implementation Handoff

Status date: 2026-09-18

This file is a development handoff/checkpoint.
It is not the normative protocol specification.

## Repository state

Primary VRF worktree:

    ~/projects/rabbit-geth-vrf

Branch:

    feat/rabbit-vrf-v0.1

Stable pre-VRF Rabbit Core checkpoint:

    b6fda8e6118d6ac7a6af81119120345e80081249
    Rabbit Core Testnet v2.3.4

Current architecture checkpoint before this handoff:

    78c4a4595
    docs(rabbitvrf): freeze testnet pricing and twap architecture

The original Rabbit geth worktree must not be modified casually:

    ~/projects/rabbit-geth

## Scope

This work is for Rabbit Chain Testnet preparation.

Testnet chain ID:

    9280

Mainnet activation is NOT in scope.

Rabbit VRF MUST remain disabled on the public Testnet until protocol,
implementation, tests, coordinator, DKG, callback behavior, economics,
observability and release work are complete.

Do not choose a public VRF activation block early.

## Activation plumbing already implemented

Rabbit LQC config contains:

    VRFProtocolBlock uint64

Semantics:

    VRFProtocolBlock == 0
        => Rabbit VRF disabled

Activation gate already exists:

    ChainConfig.IsRabbitVRF(blockNumber)

The chain-config compatibility machinery already protects changes to the
Rabbit VRF fork block.

The current Rabbit Testnet genesis does not configure a VRF activation block.

## Cryptographic profile

Profile:

    RABBIT-VRF-BLS12381-G1PK-G2SIG-V1

Curve:

    BLS12-381

Public keys:

    G1 compressed, 48 bytes

Signatures:

    G2 compressed, 96 bytes

Hash-to-curve DST:

    RABBIT-VRF-BLS12381G2_XMD:SHA-256_SSWU_RO_V1

Randomness domain:

    RABBIT-VRF-RANDOMNESS-V1

Randomness derivation occurs only after signature verification.

Threshold API uses verified partial-signature tokens before combination.

Frozen byte vectors exist and were independently reproduced with py_ecc.

Relevant commits include:

    1b9bac2ce  BLS12-381 core
    007e29e0e  crypto profile and threshold vectors
    97f794d6a  threshold property/fuzz coverage
    690defb7f  cross-platform interop CI
    d099646b5  cross-platform validation documentation
    01ccd49c4  independent vector reproduction

## Coordinator architecture

Canonical names:

    RabbitVRFCoordinatorV1
    IRabbitVRFCoordinatorV1
    IRabbitVRFConsumerV1

Coordinator is an immutable system predeploy facade over consensus-native VRF.

Frozen coordinator address:

    0xdFc21aeA108e3F527E5f236ebf354dc8262719da

Derived from:

    low20(keccak256("RABBIT_VRF_COORDINATOR_V1"))

No mutable owner/admin path may replace:

    randomness
    committee
    threshold
    epoch key
    canonical price
    protocol parameters

Consensus remains authoritative for VRF validity.

## Request ABI

Frozen request entry point:

    requestRandomness(
        uint32 callbackGasLimit,
        bytes32 appDataHash
    ) external payable returns (bytes32 requestId)

Other frozen public concepts include:

    quoteRequestFee(...)
    nextRequestNonce(...)
    getRequest(...)

Pending request storage semantics:

    requestBlock = block.number of the successful request
    epoch = 0 while PENDING
    round = 0 while PENDING
    randomness = bytes32(0) while PENDING
    proofHash = bytes32(0) while PENDING
    feePaid = Rabbit VRF protocol fee only

The zero epoch/round values above are pending placeholders. Canonical epoch
and round assignment belongs to the future fulfillment path.

Callback funding remains separate from feePaid. Until callback escrow and
execution semantics are frozen and implemented, the pre-activation
coordinator rejects callbackGasLimit > 0. callbackGasLimit == 0 remains the
supported request mode.

Request status values:

    0 NONE
    1 PENDING
    2 FULFILLED
    3 EXPIRED
    4 FAILED

Request domain:

    keccak256("RABBIT_VRF_REQUEST_V1")

The ABI may still require a controlled revision when callback escrow fields are
finalized. Do not silently conflate callback funding with the VRF protocol fee.

## Testnet pricing assets

tRUSD:

    0xaB9fEC2ff2b4f481F585b2358f3842C66e0194bd
    decimals = 6

tWRAB:

    0xef03f43ed1cb21d56cb0b26934d09cabc1994c8d
    decimals = 18

RabbitSwap Factory:

    0x3455FF1c81B8FC1D8229019766495cD2a9A6C577

RabbitSwap Router02:

    0xF5A9BF9Df2c6CEb8987b2cb26f4CcE310577A7b0

Canonical tRUSD/tWRAB pair:

    0x8b9f4581b71964049ac6be03b22000132438b385

Pair ordering:

    token0 = tRUSD
    token1 = tWRAB

The verified RabbitSwapPair exposes:

    getReserves()
    price0CumulativeLast()
    price1CumulativeLast()

The pair implements UQ112x112 cumulative pricing.

## Frozen Testnet V0.1 pricing

Service price unit:

    tRUSD

Payment asset:

    native tRAB

Base VRF protocol fee:

    0.01 tRUSD

Equivalent base units:

    VRF_BASE_FEE_TRUSD_BASE_UNITS = 10_000

Canonical conversion source:

    RabbitSwap tRUSD/tWRAB TWAP

Spot reserve pricing is forbidden for canonical VRF billing.

Trusted web APIs, RPC operators and centralized price feeds are forbidden as
canonical billing sources.

## Frozen TWAP parameters

    TWAP_MIN_WINDOW_SECONDS = 1800
    TWAP_OBSERVATION_CADENCE_SECONDS = 300
    TWAP_MAX_AGE_SECONDS = 3600
    TWAP_OBSERVATION_RING_SIZE = 16

TWAP observations are stored in RabbitVRFCoordinatorV1.

Each observation contains:

    observedAt
    price0Cumulative

The canonical oracle update is consensus-driven.

It runs during:

    PreExecution

It is gated by:

    ChainConfig.IsRabbitVRF(blockNumber)

The system call originates from:

    params.SystemAddress

and targets:

    RabbitVRFCoordinatorV1

There is:

    no keeper
    no relayer
    no privileged EOA
    no admin price setter
    no trusted fallback

Running the oracle update during PreExecution prevents swaps inside the
current block from changing the pricing observation used by VRF requests in
that same block.

The protocol computes the counterfactual current cumulative price using the
same UQ112x112 and uint32 timestamp semantics as RabbitSwapPair.

If valid TWAP history does not exist, requests fail deterministically.

The newest observation and the selected baseline are both subject to:

    TWAP_MAX_AGE_SECONDS = 3600

After an observation outage makes the retained history stale, a single fresh
observation does not restore service. A new valid TWAP window of at least:

    TWAP_MIN_WINDOW_SECONDS = 1800

must accumulate before new requests are accepted again.

Canonical billing uses one final full-precision rounded-up conversion:

    protocolFeeWei =
        ceil(
            VRF_BASE_FEE_TRUSD_BASE_UNITS
            * deltaPrice0Cumulative
            /
            (
                elapsedSeconds
                * 2**112
            )
        )

`deltaPrice0Cumulative` uses uint256 wraparound subtraction matching the
RabbitSwap cumulative accumulator. The implementation must avoid intermediate
uint256 multiplication overflow and must not truncate an intermediate average
price before the final fee calculation.

An initial warm-up period of at least 1800 seconds is required.

## Reward model

The Rabbit VRF protocol fee is separate from the normal Rabbit block reward.

For a successfully fulfilled VRF request:

    producer = 50%
    VRF committee = 30%
    Rabbit allocation = 20%

Basis points:

    VRF_PRODUCER_BPS = 5000
    VRF_COMMITTEE_BPS = 3000
    VRF_RABBIT_BPS = 2000

The integer remainder belongs to the Rabbit allocation.

This split applies only to the VRF protocol fee.

Callback execution funding is separate and MUST NOT change the 50/30/20 split.

The internal destination/policy of the Rabbit 20% allocation is not yet frozen.

The internal distribution of the committee 30% is not yet frozen.

Frozen Testnet V0.1 reward settlement architecture:

    VRF_SETTLEMENT_PERIOD_BLOCKS = 128
    settlementPeriod =
        (block.number - VRFProtocolBlock)
        / VRF_SETTLEMENT_PERIOD_BLOCKS

Rewards are accounted when a request becomes canonically `FULFILLED`.

The producer 50% is credited to the canonical producer of the fulfillment
block. It is not assigned to the producer of the request block.

The committee 30% is credited to the canonical committee reward pool for the
fulfillment. The producer/requester MUST NOT provide an arbitrary recipient
list. Exact valid-contributor distribution remains OPEN.

The Rabbit 20% is credited to protocol accounting, but its final destination
and internal policy remain OPEN.

Credits are pull-based:

    fulfillment
        -> pending credit
        -> next settlement-period boundary
        -> claimable credit
        -> participant claim

The 128-block schedule is anchored to `VRFProtocolBlock`, so the first
settlement period begins exactly at Rabbit VRF activation and contains 128
VRF-active blocks.

The 128-block boundary is a logical pulse only. There is no global transfer
loop over miners or committee members at the boundary.

Maturation is intended to be lazy and bounded so a participant may aggregate
multiple rewards before claiming.

Expired or failed requests without canonical valid fulfillment create no
50/30/20 fulfillment rewards. Exact refund economics remain OPEN.

Callback escrow is independent from this reward ledger.

The coordinator's raw native balance is not authoritative accounting. Only
explicit protocol-tracked liabilities and credits are eligible for settlement.

## Existing system-call infrastructure confirmed

core/state_processor.go already has deterministic PreExecution and
PostExecution system-call infrastructure.

Existing calls use:

    params.SystemAddress
    zero gas price
    direct EVM system calls
    StateDB finalization/access-list accounting

PreExecution is executed before normal block transactions.

Rabbit VRF should reuse this infrastructure instead of creating an external
keeper or parallel execution mechanism.

## Frozen VRF committee derivation foundation

The first native Rabbit VRF consensus foundation is now defined separately from
per-block LCQ liveness selection.

Canonical committee-candidate derivation:

    validated CLOSED WorkEpochSnapshotV1
        -> canonical closed-epoch RandomX selection entropy
        -> RABBIT-VRF-COMMITTEE-SEED-V1
        -> deterministic WorkSeat committee candidate

The Rabbit VRF committee seed binds:

    committee version
    chain ID
    source epoch
    SelectionRoot
    closed-epoch entropy

It deliberately excludes:

    block number
    parent hash
    current producer
    heartbeat/liveness state
    per-block jail/availability filtering

Committee sizing reuses `ComputeCommitteeSizeWithBounds` and is capped to the
number of canonical WorkSeats present. Rabbit VRF does not reserve producer or
fallback slots.

The deterministic committee candidate remains independent from per-block
liveness state. The source-to-target DKG epoch schedule and threshold policy are
frozen separately below. Final keyset qualification, failed-ceremony behavior
and live keyset activation remain separate lifecycle rules.


## Frozen VRF committee ShareID and commitment

The deterministic committee identity layer is now also frozen.

For a canonically ordered VRF committee:

    committee position 0 -> ShareID 1
    committee position 1 -> ShareID 2
    committee position 2 -> ShareID 3
    ...

ShareID zero is invalid and ShareIDs are never renumbered because of later DKG
participation, qualification, complaint, liveness or disqualification state.

Canonical committee commitment:

    RABBIT-VRF-COMMITTEE-ROOT-V1

The commitment binds:

    committee version
    chain ID
    source Work epoch
    SelectionRoot
    committee seed
    ordered committee members
    TicketHash
    Participant
    immutable ShareID

The commitment deliberately excludes threshold, threshold public key,
verification shares, DKG transcript and live VRF epoch assignment. Those belong
to the future keyset layer.


## Frozen VRF keyset commitment foundation

The public threshold-key material now has a deterministic commitment primitive:

    RABBIT-VRF-KEYSET-ROOT-V1

It binds:

    keyset version
    chain ID
    VRF epoch
    committee root
    original committee size
    declared threshold
    threshold public key
    DKG transcript root
    canonical verification shares

Verification shares are ordered by their immutable committee ShareID. Arrival
order is irrelevant. Missing ShareIDs are not renumbered.

The primitive performs structural validation only. It does not decide whether a
DKG transcript is valid, which members qualify, or whether and when a qualified
keyset becomes live.

The Rabbit VRF V1 threshold formula and deterministic source-to-target epoch
schedule are frozen separately by the DKG session and epoch-schedule
foundations.

The remaining complaint, qualification, transcript, failed-ceremony and live
keyset activation lifecycle rules remain OPEN.

## Critical work still open

Implementation:

    RabbitVRFCoordinatorV1 bytecode/predeploy
    coordinator storage layout
    pricing system-call selector
    PreExecution Rabbit VRF system call
    genesis/fork-time predeploy installation behavior
    TWAP unit/property tests
    activation boundary tests

VRF protocol:

    epoch keyset binding
    production DKG
    committee selection integration
    threshold context binding
    partial submission/validation
    reconstruction/finalization
    native-to-EVM fulfillment mechanism
    canonical proofHash bytes

Economics:

    callback gas escrow
    unused callback refund
    failed/expired request refund
    committee 30% internal distribution
    Rabbit 20% destination/policy
    settlement/claim storage and ABI implementation
    anti-withholding rules
    anti-spam behavior

Product:

    callback execution/retry semantics
    explorer indexing
    public playground
    developer documentation
    release packaging
    multinode testing

## Frozen threshold policy and DKG session foundation

Threshold policy is now deterministic before DKG begins.

For original deterministic committee size N:

    f = floor((N - 1) / 3)
    threshold = N - f

The committee size used here is the ORIGINAL committee size committed by
RABBIT-VRF-COMMITTEE-ROOT-V1.

It is not the later qualified-set size.

Examples:

    N=32  -> threshold=22, maxFaults=10
    N=64  -> threshold=43, maxFaults=21
    N=100 -> threshold=67, maxFaults=33
    N=128 -> threshold=86, maxFaults=42

Complaint, absence, timeout or disqualification MUST NOT lower threshold after
the DKG session begins.

If the final qualified population is below threshold, the ceremony is not
eligible to produce an activatable V1 keyset.

Canonical DKG session domain:

    RABBIT-VRF-DKG-SESSION-V1

RabbitVRFDKGSessionContextV1 binds:

    version
    chain ID
    target VRF epoch
    committee root
    original committee size
    threshold
    max fault budget

RabbitVRFDKGSessionIDV1 is derived deterministically from the canonical RLP
payload and Keccak256.

The session deliberately excludes:

    block producer
    mutable heartbeat state
    mutable liveness ordering
    network arrival order
    local peer order
    administrator input

Implementation files:

    consensus/lqc/rabbit_vrf_dkg_session_v1.go
    consensus/lqc/rabbit_vrf_dkg_session_v1_test.go

This foundation does NOT yet implement polynomial generation, share exchange,
complaints, qualification, transcript generation, networking or activation.

## DKG implementation inspection checkpoint

The repository inspection after the canonical keyset foundation established the
following implementation facts.

Participant authentication already has a Rabbit-native model.

LCQ/Work participation signs domain-separated canonical payloads with the wallet
controlling the canonical WorkSeat Participant address. Verification recovers
the secp256k1 public key and requires:

    crypto.PubkeyToAddress(recoveredKey) == Participant

The Rabbit VRF DKG SHOULD reuse this Participant identity model. It MUST NOT
introduce an administrator-controlled DKG identity.

Existing signing patterns include:

    accounts.Wallet.SignData
    crypto.SigToPub
    crypto.PubkeyToAddress

The P2P node key is a separate identity. It MUST NOT silently replace the
canonical WorkSeat Participant wallet identity.

Rabbit already has dedicated bounded subprotocol patterns that can be reused as
implementation references:

    lqct
    lqcw

They already demonstrate:

    p2p.Protocol
    protocol status handshake
    ReadMsg
    p2p.Send
    peer tracking
    validation
    relay/deduplication

Rabbit VRF may eventually use a dedicated versioned DKG subprotocol, but DKG
traffic MUST remain isolated so it cannot stall normal LCQ block production.

Confidential cryptographic transport primitives are already available in:

    crypto/ecies

Available primitives include:

    GenerateShared
    Encrypt
    Decrypt

RLPx also already uses ECDH/ECIES internally.

This does NOT yet freeze the Rabbit VRF private-share encryption construction.
The exact encryption format, authenticated context and replay protection remain
protocol work.

LQC already owns an ethdb.Database and has deterministic persistence patterns,
including:

    StoreWorkTicketSnapshot / LoadWorkTicketSnapshot
    StoreRegistrySnapshot / LoadRegistrySnapshot
    WorkCommitPoolPersistenceV1
    LQC recovery checkpoints using l.db.Put / l.db.Get

Rabbit VRF DKG persistence should follow versioned crash-safe storage patterns.

Secret DKG shares MUST NOT be written to logs.

Secret-share-at-rest encryption, atomic write boundaries and restore behavior
remain OPEN.

Current cryptographic dependencies already include:

    github.com/consensys/gnark-crypto v0.18.1
    github.com/protolambda/bls12-381-util v0.1.0
    github.com/supranational/blst v0.3.16
    github.com/kilic/bls12-381 v0.1.0 indirect

No production drand or kyber dependency is currently required by the Rabbit VRF
foundation.

## Frozen public polynomial commitment foundation

The public polynomial commitment layer is now implemented and tested.

Implementation files:

    crypto/rabbitvrf/dkg_commitment_v1.go
    crypto/rabbitvrf/dkg_commitment_v1_test.go
    consensus/lqc/rabbit_vrf_dkg_commitment_v1.go
    consensus/lqc/rabbit_vrf_dkg_commitment_v1_test.go

Frozen representation:

    scalar coefficients: BLS12-381 Fr
    public commitments: BLS12-381 G1
    compressed commitment size: 48 bytes
    polynomial coefficient count: threshold
    exact polynomial degree: threshold - 1

For coefficient a[j]:

    C[j] = G1 * a[j]

Every G1 encoding is required to be canonical, on-curve and in-subgroup.

The constant coefficient commitment and highest-degree coefficient commitment
MUST be non-infinity.

Intermediate coefficient commitments MAY be canonical G1 infinity points,
representing zero intermediate scalar coefficients.

Canonical commitment domain:

    RABBIT-VRF-DKG-POLY-COMMITMENT-V1

Each dealer commitment binds:

    version
    DKG session ID
    immutable DealerShareID
    ordered coefficient commitments

The commitment identity is Keccak256 over the canonical RLP payload.

The implementation rejects:

    wrong coefficient count
    malformed G1 encoding
    non-canonical G1 encoding
    zero constant commitment
    zero highest-degree commitment
    zero DealerShareID
    DealerShareID above original committee size
    wrong DKG session

Dealer commitment collections are ordered by ascending DealerShareID.

Duplicate DealerShareIDs are rejected.

P2P arrival order cannot select the canonical order.

Canonicalized commitments deep-copy coefficient slices.

This layer contains no private polynomial coefficients and performs no DKG
network transport.

Dealer authentication is NOT yet supplied by this structure itself. Future DKG
messages must bind the dealer ShareID to the canonical WorkSeat Participant
wallet identity.

## Frozen private evaluation verification foundation

The pure private polynomial evaluation verification layer is now implemented
and tested.

Implementation files:

    crypto/rabbitvrf/dkg_evaluation_v1.go
    crypto/rabbitvrf/dkg_evaluation_v1_test.go
    consensus/lqc/rabbit_vrf_dkg_evaluation_v1.go
    consensus/lqc/rabbit_vrf_dkg_evaluation_v1_test.go

Canonical dealer evaluation representation:

    BLS12-381 Fr
    32 canonical bytes

Individual dealer evaluations MAY be zero.

Final aggregated secret shares remain subject to the existing non-zero
SecretShare requirement.

For recipient ShareID x:

    G1 * evaluation
        ==
    C[0] + x*C[1] + x^2*C[2] + ... + x^(t-1)*C[t-1]

The implementation verifies this equation directly against the dealer's ordered
public polynomial commitments.

The consensus wrapper additionally binds verification to:

    canonical DKG session
    valid dealer polynomial commitment
    recipient ShareID != 0
    recipient ShareID <= original committee size

Tests cover:

    canonical scalar parsing
    zero individual dealer contribution
    multiple recipient ShareIDs
    altered private evaluation
    altered public commitment
    wrong recipient ShareID
    recipient bounds
    cross-session rejection

No network transport, sender authentication or encryption is implemented by
this layer.

## Frozen authenticated DKG envelope foundation

The Participant-authenticated public DKG envelope foundation is implemented and
tested.

Implementation files:

    consensus/lqc/rabbit_vrf_dkg_envelope_v1.go
    consensus/lqc/rabbit_vrf_dkg_envelope_v1_test.go

Envelope version:

    1

Current public message type:

    RabbitVRFDKGMessagePolynomialCommitmentV1 = 1

Domains:

    RABBIT-VRF-DKG-ENVELOPE-SIGN-V1
    RABBIT-VRF-DKG-ENVELOPE-SLOT-V1
    RABBIT-VRF-DKG-ENVELOPE-ID-V1

The signed payload binds:

    version
    canonical DKG SessionID
    message type
    immutable sender ShareID
    canonical Participant
    payload hash

Signing uses:

    accounts.Wallet.SignData

Verification performs:

    canonical RLP signing data
    Keccak256
    secp256k1 SigToPub recovery
    recovered address == canonical Participant

The P2P node key is explicitly NOT the DKG Participant identity.

For public polynomial commitment messages:

    PayloadHash = validated canonical polynomial commitment root

Replay/equivocation identity is split into:

    SlotID
    EnvelopeID

SlotID binds:

    version
    SessionID
    message type
    sender ShareID

EnvelopeID binds:

    SlotID
    Participant
    PayloadHash

Signature bytes do not affect EnvelopeID.

For the current singleton polynomial commitment message, no monotonic sequence
number is required.

Two valid differently signed payloads for one:

    SessionID + message type + ShareID

produce:

    same SlotID
    different EnvelopeID

and are therefore suitable as later objective equivocation evidence.

Cross-session and cross-chain replay rejection are explicitly tested.

Real keystore-backed:

    accounts.Wallet.SignData

is explicitly tested against:

    VerifyRabbitVRFDKGEnvelopeV1

Raw private polynomial evaluations are NOT carried in this public envelope.

## Frozen recipient-bound encrypted private evaluation foundation

Rabbit VRF V1 now implements and tests recipient-bound encrypted private
polynomial evaluation transport at the protocol-object layer.

Implementation files:

    consensus/lqc/rabbit_vrf_dkg_encrypted_evaluation_v1.go
    consensus/lqc/rabbit_vrf_dkg_encrypted_evaluation_v1_test.go

The canonical plaintext is exactly one:

    DKGPolynomialEvaluationV1

encoded as exactly 32 canonical BLS12-381 Fr bytes.

The encrypted object binds:

- protocol version;
- canonical DKG SessionID;
- immutable dealer ShareID;
- dealer Participant wallet;
- immutable recipient ShareID;
- recipient Participant wallet;
- canonical dealer polynomial CommitmentRoot;
- authenticated recipient TransportKeyRoot;
- randomized geth ECIES ciphertext;
- dealer Participant-wallet signature.

Rabbit VRF V1 uses the authenticated recipient transport key with geth ECIES
over secp256k1 using AES-128 / SHA-256.

For the exact 32-byte evaluation plaintext, the V1 ciphertext representation is
fixed at 145 bytes under this geth ECIES profile.

Exact shared-information domains are:

    RABBIT-VRF-DKG-EVAL-ECIES-KDF-V1
    RABBIT-VRF-DKG-EVAL-ECIES-MAC-V1

Ciphertext slot and identity domains are:

    RABBIT-VRF-DKG-EVAL-CIPHERTEXT-SLOT-V1
    RABBIT-VRF-DKG-EVAL-CIPHERTEXT-ID-V1

Dealer authentication uses:

    RABBIT-VRF-DKG-EVAL-CIPHERTEXT-SIGN-V1

The dealer signs canonical RLP metadata plus the exact ciphertext hash using the
canonical Participant wallet identity.

The P2P node key and DKG transport key are NOT accepted as substitutes for the
dealer Participant wallet signature.

ECIES is randomized.

Therefore two valid encryptions for the same semantic dealer-to-recipient slot:

- have the same SlotID;
- normally have different ciphertext bytes;
- normally have different MessageIDs;
- are NOT automatically equivocation merely because the ciphertext differs.

MessageID commits to the semantic SlotID and exact ciphertext hash.

Signature bytes themselves are excluded from MessageID.

Decryption requires:

- the exact canonical DKG session;
- the canonical dealer;
- the dealer's authenticated Participant-wallet signature;
- the canonical recipient;
- the authenticated recipient transport-key binding;
- the exact local transport private key matching that binding;
- successful ECIES authentication/decryption;
- canonical 32-byte Fr evaluation decoding;
- successful Feldman evaluation verification against the dealer commitment.

Tests cover:

- encrypt/decrypt round-trip;
- domain-separated s1 and s2;
- randomized ciphertext with stable semantic SlotID;
- ciphertext tampering rejection;
- wrong transport private-key rejection;
- metadata tampering;
- cross-session rejection;
- cross-chain rejection;
- unauthenticated replacement transport-key rejection;
- dealer signature tampering;
- wrong dealer signer;
- real accounts.Wallet.SignData compatibility;
- deterministic s1, s2, SlotID and MessageID vectors;
- full LQC regression.

Raw private polynomial evaluations remain forbidden from public DKG gossip.

No production P2P delivery is enabled by this foundation.

## Frozen encrypted DKG transport-key persistence foundation

Rabbit VRF now has an isolated crash-safe persistence layer for each
session-scoped DKG transport private key.

Implementation:

    internal/rabbitvrfstate/dkg_transport_key_store_v1.go
    internal/rabbitvrfstate/dkg_transport_key_store_sync_unix.go
    internal/rabbitvrfstate/dkg_transport_key_store_sync_windows.go
    internal/rabbitvrfstate/dkg_transport_key_store_v1_test.go

The persistence layer is deliberately outside consensus/lqc.

The persisted secret is one dedicated secp256k1 DKG transport private key.

It is NOT:

- the Participant wallet private key;
- the P2P node private key;
- raw plaintext chain state;
- a generic SaveECDSA plaintext file.

The store binds encrypted secret state to:

- store version;
- DKG transport binding version;
- canonical SessionID;
- immutable ShareID;
- Participant wallet address;
- transport Scheme;
- exact canonical compressed transport PublicKey;
- exact 32-byte secp256k1 private scalar.

Encryption uses the existing geth keystore CryptoJSON V3 machinery through:

    keystore.EncryptDataV3
    keystore.DecryptDataV3

Production construction uses the standard geth scrypt parameters.

The password/credential is supplied to the store by its caller.

The storage layer deliberately does NOT yet decide whether Rabbit Core runtime
will reuse an existing local credential or introduce a Rabbit-VRF-specific
credential source.

Security properties now tested:

- create a new independent transport key once;
- encrypt private key at rest;
- no raw private-key bytes in the persisted JSON;
- no hex-encoded private key in the persisted JSON;
- reload after simulated restart returns the exact same private key;
- reload reproduces the exact same authenticated public binding;
- wrong password fails closed;
- corrupted ciphertext fails closed;
- tampered public metadata fails closed;
- metadata is duplicated inside authenticated encrypted plaintext and must
  exactly match the outer record;
- missing state fails closed;
- cross-session lookup does not silently reuse another session's key;
- Participant-wallet key reuse is rejected;
- inconsistent ECDSA D/PublicKey pairs are rejected;
- nil store use fails closed;
- an existing persisted transport key is never silently overwritten;
- concurrent independent store instances cannot both replace the same
  session/share key;
- private directory is restricted to 0700 where supported;
- private file is restricted to 0600 where supported;
- temporary secret file is fsynced before publication;
- directory entries are synced around publication/removal;
- final publication uses create-without-replacement semantics;
- Linux/Unix tests pass;
- Go race detector passes;
- Windows amd64 compilation passes;
- full consensus/lqc regression passes.

This foundation alone does NOT announce a transport binding and does NOT start
DKG P2P transport.

Rabbit VRF remains disabled on public Testnet.

## Exact next implementation step

**Implement the canonical Work-state -> Rabbit VRF committee -> DKG session
bridge before wiring transport-key persistence into Rabbit Core runtime.**

The deterministic foundation now already defines:

    CLOSED WorkEpochSnapshotV1
        -> Rabbit VRF committee candidate
        -> immutable ShareIDs
        -> committee root
        -> threshold/maxFaults
        -> source Work epoch N
        -> DKG preparation epoch N+2
        -> TargetVRFEpoch N+3
        -> deterministic DKG SessionID

What is still missing is the production bridge from the live canonical LQC Work
state to those pure foundations.

The next implementation slice MUST enforce this order:

1. start from the canonical LQC Work runtime state at a canonical chain head;
2. obtain the validated CLOSED `WorkEpochSnapshotV1` through the existing Work
   snapshot machinery;
3. derive the exact RandomX dataset/cache key for that CLOSED source epoch from
   existing Work V1 consensus rules;
4. perform the selection-beacon RandomX evaluation through the existing
   `crypto/rabbitx` implementation;
5. nodes SHOULD use the bounded-memory `rabbitx.LightHasher` for this consensus
   recomputation;
6. feed that result into `DeriveWorkSelectionEntropyV1`;
7. derive the deterministic Rabbit VRF committee using
   `RabbitVRFCommitteeForSnapshotV1`;
8. preserve the committee's canonical ordering and immutable ShareIDs;
9. derive the canonical committee root;
10. derive `TargetVRFEpoch` through
    `RabbitVRFTargetEpochForSourceWorkEpochV1`;
11. construct `RabbitVRFDKGSessionContextV1`;
12. derive and verify the canonical DKG SessionID;
13. resolve whether a local Participant wallet is a member and, if so, its exact
    immutable ShareID.

Architecture requirements:

- do NOT implement a second RandomX engine;
- do NOT move RandomX CGo ownership into the pure committee primitives;
- the runtime layer may own `rabbitx.LightHasher` and provide its `Hash` method
  as the `WorkSelectionBeaconHasherV1`;
- pure consensus derivation must remain independently testable with an injected
  deterministic hasher;
- do NOT use per-block liveness ordering, producer identity, heartbeat state or
  arrival order in VRF committee derivation;
- do NOT generate a DKG transport private key until the local node has resolved
  one canonical DKG SessionID and local ShareID;
- do NOT persist or announce a transport binding for a wallet which is not a
  member of that canonical committee;
- reorg handling must derive from the new canonical Work state rather than from
  stale process-local state;
- no caller may choose SourceWorkEpoch, TargetVRFEpoch, committee members,
  ShareIDs or SessionID manually.

After this bridge is implemented and tested, wire the persisted transport-key
lifecycle into the Rabbit Core runtime in this order:

1. resolve canonical DKG SessionID and local immutable ShareID;
2. resolve Participant wallet identity;
3. resolve the secure Rabbit VRF state directory from the node datadir;
4. obtain the explicit storage credential;
5. load an existing transport key when persisted state exists;
6. create one only when no prior authenticated binding exists for that
   session/share slot;
7. reject Participant-wallet key reuse;
8. reject local P2P node-key reuse;
9. construct and authenticate the canonical transport-key binding;
10. announce the binding only after persistence succeeds.

Fail closed requirements remain:

- never silently regenerate after an authenticated binding was announced;
- missing persisted state after an existing binding is known is fatal for that
  DKG participation;
- corrupted state is fatal for that DKG participation;
- wrong credential is fatal for that DKG participation;
- persisted/public-key mismatch is fatal;
- Participant/P2P transport-key reuse is fatal;
- never fall back to plaintext secret storage;
- never derive the transport key from a wallet signature;
- never silently retarget a failed DKG session to another VRF epoch.

After runtime persistence lifecycle integration is frozen and restart-tested,
continue with confidential point-to-point DKG delivery, replay/duplicate state,
complaint evidence, qualification, transcript aggregation and keyset lifecycle.

Rabbit VRF MUST remain disabled on public Testnet until the complete lifecycle
and multinode activation tests are finished.

## Working rules

Do not delete datadirs, blockchain state, keystores or WorkSeats.

Do not use destructive git cleanup/reset operations.

Do not modify the original stable rabbit-geth worktree casually.

Keep VRF development in:

    ~/projects/rabbit-geth-vrf

Do not claim consensus or VRF behavior is fixed without tests.

Before public activation require multinode validation.

Before any future Testnet fork, discover and test the full activation surface.

One eligible wallet, one fair chance remains a Rabbit Chain consensus design
goal.

## Rabbit VRF DKG transport-key binding checkpoint

Previous committed checkpoint before this slice:

`333c031bc feat(rabbitvrf): add authenticated dkg envelope`

Current transport-key slice:

- envelope message type `2` added for transport-key bindings;
- canonical 33-byte compressed secp256k1 transport public key;
- transport scheme V1 bound to geth secp256k1 ECIES AES-128 / SHA-256;
- transport binding commits SessionID, ShareID, Participant, scheme and key;
- Participant-wallet authenticated through the existing DKG envelope;
- Participant wallet-key reuse rejected;
- P2P node key explicitly remains a different runtime identity and MUST NOT be
  reused as the Rabbit VRF DKG transport private key;
- conflicting authenticated transport keys retain the same singleton SlotID
  and different EnvelopeIDs for equivocation evidence;
- ECIES compatibility is tested with a real encryption/decryption round-trip;
- raw private polynomial evaluations remain disabled.

The session transport private key must eventually survive node restart for the
active DKG session. It MUST NOT be silently regenerated after its authenticated
public binding has been published, because publishing a different key in the
same singleton slot is equivocation.

The private transport secret MUST NOT be stored as plaintext in a generic chain
database. Encrypted crash-safe persistence is implemented in
`internal/rabbitvrfstate`; Rabbit Core runtime lifecycle wiring remains pending.

## Frozen Rabbit VRF epoch and DKG preparation schedule

Rabbit VRF V1 now has a deterministic source Work epoch -> DKG -> target VRF
epoch mapping.

For source Work epoch N:

    source Work epoch          N
    delayed selection epoch    N + 2
    DKG preparation epoch      N + 2
    target VRF epoch           N + 3

Canonical formula:

    TargetVRFEpoch = SourceWorkEpoch + 3

The source Work snapshot is already canonical and available at the first block
of N+2. The entire N+2 epoch is reserved as the DKG preparation window. The
session is intended for the VRF epoch beginning at N+3.

For epochLength=128 and source N=1:

    preparation starts at block 257
    target epoch 4 starts at block 385

The target is immutable session identity. A failed DKG does not silently retarget
the same session.

Still OPEN at this layer:

- final qualified-keyset activation state machine;
- failed-ceremony rollover behavior;
- old-key continuity if the intended next keyset is unavailable;
- exact public activation fork block;
- P2P DKG transport runtime.


## DKG transport key-store hardening checkpoint

Completed after `f9c9e0c2f`:

- DKG transport key-store CryptoJSON metadata is validated before `DecryptDataV3`.
- Cipher must be `aes-128-ctr`.
- KDF must be `scrypt`.
- Persisted scrypt `n`, `r`, `p`, and `dklen` must match the store profile.
- Salt, IV, MAC, and ciphertext are structurally validated before decrypt/scrypt.
- Malformed or attacker-edited KDF metadata returns `ErrInvalidDKGTransportKeyStoreV1` before the expensive decrypt path.
- Wrong passwords continue to use `ErrDKGTransportKeyStoreDecryptV1`.
- The outer persisted DKG transport public key is now canonical `0x` hex instead of a JSON integer array.
- The inner authenticated secret remains the fixed 33-byte transport public key representation.
- Tests cover tampered scrypt parameters, malformed crypto metadata, and canonical public-key JSON persistence.
- `go test -race ./internal/rabbitvrfstate -count=1` passes.
- `go vet ./internal/rabbitvrfstate` passes.
- Windows amd64 compile check passes.
- `go test ./consensus/lqc -count=1` passes.

## Work -> Rabbit VRF -> DKG session bridge checkpoint

Completed at:

`5b4130b42 feat(rabbitvrf): bridge work state to dkg session`

The canonical bridge is now implemented in:

- `consensus/lqc/rabbit_vrf_dkg_bridge_v1.go`
- `consensus/lqc/rabbit_vrf_dkg_bridge_v1_test.go`

The production Work entropy path was resolved before wiring:

- `WorkEpochSnapshotV1.Anchor` is the Work challenge/commit anchor.
- It MUST NOT be treated as the RandomX dataset anchor.
- The canonical dataset anchor block is derived with
  `WorkDatasetAnchorBlockV1(sourceEpoch, epochLength)`.
- The runtime must resolve the canonical ancestor header at that block.
- `RandomXWorkDatasetKeyV1(chainID, sourceEpoch, datasetHeader.Hash())`
  derives the DatasetKey.
- `DeriveWorkSelectionEntropyV1` performs the single deterministic RandomX
  evaluation used by the bridge.

`RabbitVRFDKGBridgeForSnapshotV1` composes only existing frozen rules:

- validated CLOSED Work snapshot;
- canonical Work selection entropy;
- deterministic Rabbit VRF committee commitment;
- immutable ShareID assignment;
- source Work epoch -> DKG preparation -> target VRF epoch schedule;
- deterministic DKG threshold/fault policy;
- canonical DKG SessionID.

For source Work epoch N:

    preparation epoch = N + 2
    target VRF epoch  = N + 3

Bridge tests cover:

- deterministic repeated construction;
- exact epoch mapping;
- immutable sequential ShareIDs;
- DKG session binding to the resulting committee;
- invalid snapshot / DatasetKey / hasher rejection;
- DatasetKey binding through entropy, committee seed, committee root and SessionID.

Validation passed:

- `go test ./consensus/lqc -count=1`
- targeted Rabbit VRF bridge tests
- targeted `go test -race ./consensus/lqc`
- `go vet ./consensus/lqc`
- `git diff --check`

Rabbit VRF activation remains disabled.

## Canonical runtime bridge + preactivation DKG checkpoint

Completed at:

`13c5112f2 feat(rabbitvrf): wire canonical dkg runtime bridge`

The canonical LQC Work runtime now exposes
`RabbitVRFDKGBridgeContextV1`.

The runtime bridge:

- resolves the validated CLOSED Work selection snapshot;
- resolves the RandomX dataset anchor through the parent branch;
- derives `RandomXWorkDatasetKeyV1` from the canonical ancestor header;
- reuses the existing cached Work selection-beacon hasher;
- derives the deterministic Rabbit VRF committee and immutable ShareIDs;
- derives source Work epoch, DKG preparation epoch and target VRF epoch;
- derives the canonical DKG SessionID;
- remains read-only and does not create transport keys or publish messages.

Tests cover:

- disabled configuration with no runtime side effects;
- canonical Work runtime construction;
- parent-branch dataset-anchor correctness across a fork/reorg fixture.

Validation passed with the pinned Rabbit RandomX build:

- targeted Rabbit VRF runtime bridge tests;
- full `consensus/lqc` suite with
  `rabbit_workv1 rabbit_randomx`;
- `git diff --check`.

## Rabbit VRF preactivation DKG window checkpoint

Completed at:

`d57bb77d7 feat(rabbitvrf): derive preactivation dkg window`

Rabbit VRF keeps one public fork parameter:

    VRFProtocolBlock

Semantics are now frozen as:

    VRFProtocolBlock == 0
        => Rabbit VRF disabled
        => DKG preparation disabled

    VRFProtocolBlock > 0
        => VRFProtocolBlock is the first Rabbit VRF-active block

The activation block MUST be the first block of a Work epoch and MUST target
VRF epoch 4 or later.

The initial DKG preparation gate is derived automatically from the same fork:

    DKGPreparationBlock =
        VRFProtocolBlock - EpochLength

No second public DKG fork parameter is introduced.

For epochLength=128 and VRFProtocolBlock=385:

    blocks <= 256
        DKG preparation disabled
        Rabbit VRF disabled

    blocks 257..384
        DKG preparation enabled
        Rabbit VRF still disabled

    block 385+
        Rabbit VRF active

`IsRabbitVRFDKGPreparation` is separate from `IsRabbitVRF`.

`RabbitVRFDKGBridgeContextV1` uses the preparation gate, allowing the first
committee ceremony to be prepared during the Work epoch immediately before
Rabbit VRF activation.

The normal Rabbit VRF protocol gate continues to protect coordinator
activation, pricing observations and other VRF-active behavior.

Validation passed:

- full `go test ./params -count=1`;
- full `go test -tags="rabbit_workv1 rabbit_randomx" ./consensus/lqc -count=1`;
- `git diff --check`.

The public Rabbit Testnet still has no Rabbit VRF activation block configured.
Rabbit VRF therefore remains disabled.

## Exact next implementation step

Implement the Rabbit VRF DKG runtime lifecycle that consumes the already
canonical `RabbitVRFDKGBridgeContextV1` during the preparation window.

Before editing lifecycle code, inspect the existing DKG session, transport,
persistent-state and keyset structures and identify the smallest canonical
runtime insertion point.

The first lifecycle slice MUST remain deterministic and bounded:

1. detect the canonical preparation session from
   `RabbitVRFDKGBridgeContextV1`;
2. create or resume exactly that SessionID;
3. never retarget an existing session after restart or reorg;
4. never create a DKG runtime when `IsRabbitVRFDKGPreparation` is false;
5. keep transport-key creation, credential reads and P2P publication behind
   explicit lifecycle phases;
6. preserve crash-safe persisted state;
7. do not activate a target keyset until qualification rules are satisfied.

Still required after this lifecycle slice:

- confidential DKG P2P transport runtime;
- replay and duplicate handling;
- complaint evidence and dealer qualification;
- transcript aggregation;
- final qualified-keyset activation;
- failed-ceremony/old-key continuity rules;
- threshold signing and canonical fulfillment;
- restart, sync, reorg and multinode testing;
- callbacks, economics, observability, documentation and release validation;
- final public Testnet activation block selection only after all of the above.

## Runtime DKG lifecycle checkpoint 2026-09-24

Current branch:

    feat/rabbit-vrf-v0.1

Latest committed checkpoints:

    434baed85 feat(rabbitvrf): wire dkg lifecycle runtime
    ba6b7bf64 feat(rabbitvrf): persist deterministic dkg lifecycle
    b594be413 docs(rabbitvrf): checkpoint preactivation dkg runtime
    d57bb77d7 feat(rabbitvrf): derive preactivation dkg window

Worktree was clean immediately after commit `434baed85`.

### What is now implemented

Rabbit VRF remains disabled on the public Testnet unless `VRFProtocolBlock`
is explicitly configured.

There is still only ONE configured Rabbit VRF activation fork.

For an activation aligned to Work epoch boundaries, DKG preparation is derived
automatically one Work epoch before the target VRF epoch.

Example with epoch length 128:

    DKG preparation begins: block 257
    Rabbit VRF activation:  block 385
    target VRF epoch:       4

The canonical runtime bridge can now derive:

- closed source Work epoch;
- canonical Work selection snapshot;
- RandomX dataset key;
- deterministic selection entropy;
- deterministic VRF committee;
- immutable ShareIDs;
- committee root;
- DKG session context;
- canonical SessionID;
- preparation epoch;
- target VRF epoch.

The public deterministic DKG lifecycle is now persisted by SessionID.

Persistence properties implemented and tested:

- deterministic create/resume;
- restart recovery;
- canonical RLP validation;
- corruption rejection;
- conflicting metadata rejection;
- canonical committee-root recomputation;
- concurrent Ensure serialization;
- exactly one create result under concurrent callers;
- no global mutable active-session pointer;
- branch-derived SessionIDs may coexist without retargeting one another.

Runtime wiring implemented in `434baed85`:

    Finalize
      -> maybeEnsureRabbitVRFDKGLifecycleV1
      -> RabbitVRFDKGBridgeContextV1
      -> preparation gate
      -> canonical Work bridge
      -> EnsureRabbitVRFDKGLifecycleV1

Important safety property:

A local persistence failure logs an error but does NOT make an otherwise valid
consensus block invalid.

The runtime path currently performs NO:

- DKG transport-key generation;
- password or credential access;
- Participant-wallet signing;
- P2P publication;
- private evaluation delivery;
- complaint processing;
- dealer qualification;
- keyset activation.

Build separation is preserved:

- Work/RandomX build contains the real lifecycle runtime hook;
- ordinary build contains a no-op stub.

Validation completed before checkpoint `434baed85`:

- ordinary `go test ./consensus/lqc`;
- official `rabbit_workv1 rabbit_randomx` targeted tests;
- race detector targeted tests;
- full official LQC suite;
- official-tag `go vet`;
- staged whitespace/diff checks.

All passed.

### Existing secret transport-key foundation

The encrypted transport-key store already exists at:

    internal/rabbitvrfstate/dkg_transport_key_store_v1.go

It stores an independent Rabbit VRF DKG transport key, NOT the Participant
wallet key and NOT the P2P node key.

The persisted secret is bound to:

- canonical SessionID;
- immutable ShareID;
- Participant address;
- transport scheme;
- canonical compressed transport public key.

The private key is encrypted using geth keystore CryptoJSON V3 machinery.

Transport-key creation is deliberately nondeterministic and MUST remain outside
consensus replay and header verification.

### Exact next implementation step

Do NOT immediately generate transport keys from the LQC Finalize hook.

First inspect and freeze the Rabbit Core runtime ownership needed for the secret
side of the DKG lifecycle.

Resolve the exact production objects/APIs for:

1. local Participant wallet identity;
2. determining whether that Participant is actually a member of the canonical
   bridge committee and obtaining its immutable ShareID;
3. canonical Rabbit Core/geth datadir;
4. secure Rabbit VRF private-state directory;
5. explicit storage credential/password source;
6. Participant wallet signing/backend access;
7. local P2P node public/private identity so transport-key reuse can be rejected;
8. lifecycle start/stop ownership outside deterministic consensus replay.

After those runtime ownership points are known, implement a separate local
runtime service with this ordering:

1. consume an already-persisted canonical DKG lifecycle;
2. resolve the local Participant;
3. find the Participant in the canonical committee;
4. if not a committee member, perform no secret side effect;
5. obtain the exact immutable ShareID;
6. open the encrypted Rabbit VRF transport-key store beneath the canonical
   datadir;
7. obtain the explicit credential;
8. load the existing session/share transport key when present;
9. create exactly one new independent transport key only when safe to do so;
10. reject Participant-wallet key reuse;
11. reject local P2P node-key reuse;
12. construct the canonical transport binding;
13. authenticate it with the Participant wallet;
14. only after persistence and authentication succeed may later P2P publication
    be implemented.

Fail closed:

- never silently regenerate an already-announced session/share key;
- wrong password is fatal for that DKG participation;
- corrupted secret state is fatal for that DKG participation;
- metadata/public-key mismatch is fatal;
- never fall back to plaintext;
- never derive the transport key from a wallet signature;
- never create a transport key for a wallet outside the canonical committee;
- never let VerifyHeader/replay create private DKG state.

### Resume instruction for a new ChatGPT conversation

Use this exact message:

    Leia docs/consensus/rabbit-vrf-testnet-handoff.md, confira o HEAD e o git status sem apagar nenhuma alteração, e continue do "Exact next implementation step". Estamos no worktree ~/projects/rabbit-geth-vrf e Rabbit VRF continua desativado na Testnet pública.

Do not delete datadirs, blockchain state, keystores, WorkSeats or untracked
backups.

Do not use `git clean`, `git reset --hard` or destructive repository recovery.

Do not modify the original `~/projects/rabbit-geth` worktree casually.

## Local DKG runtime checkpoint e37583308

Current implementation checkpoint:

    e37583308 feat(rabbitvrf): resolve local dkg committee runtime
    7daaf8bb3 docs(rabbitvrf): checkpoint dkg lifecycle runtime
    434baed85 feat(rabbitvrf): wire dkg lifecycle runtime
    ba6b7bf64 feat(rabbitvrf): persist deterministic dkg lifecycle

Worktree was clean immediately after commit `e37583308`.

### What e37583308 adds

Rabbit Core now owns the local operational Rabbit VRF DKG observer.

The implementation lives in:

    eth/rabbit_vrf_dkg_runtime_lab.go
    eth/rabbit_vrf_dkg_runtime_stub.go
    eth/rabbit_vrf_dkg_runtime_lab_test.go

`Ethereum` owns the runtime lifecycle.

Production flow:

    Ethereum.Start
      -> rabbitVRFDKGRuntime.Start
      -> canonical ChainHeadEvent
      -> current canonical header
      -> RabbitVRFDKGBridgeContextV1
      -> LoadRabbitVRFDKGLifecycleV1
      -> AccountManager local wallets
      -> canonical committee match
      -> immutable ShareID resolution

Shutdown flow:

    Ethereum.Stop
      -> rabbitVRFDKGRuntime.Close

The runtime is only constructed when Rabbit VRF is configured.

A disabled VRF configuration creates no operational DKG runtime.

### Security boundary now frozen

The deterministic/public layer remains inside consensus/lqc.

The local operational layer lives in eth.

The rabbit-miner is NOT the owner of persistent DKG participation.

The current local DKG runtime performs NO:

- password reads;
- DKG transport-key generation;
- wallet DKG signing;
- DKG private evaluation generation;
- DKG P2P publication;
- complaint processing;
- qualification;
- secret-share activation.

Local committee matching preserves canonical committee ordering and immutable
ShareIDs.

A local wallet outside the canonical committee receives no local DKG member
context.

Malformed ShareIDs and duplicate committee participants are rejected.

### Validation for e37583308

Passed:

    go test ./eth -count=1

Passed with official tags:

    go test -tags="rabbit_workv1 rabbit_randomx" ./consensus/lqc ./eth -count=1

Passed race detector:

    go test -race -tags="rabbit_workv1 rabbit_randomx" ./eth -run '^TestRabbitVRFDKGRuntimeV1' -count=1

Passed vet:

    go vet -tags="rabbit_workv1 rabbit_randomx" ./consensus/lqc ./eth

Passed:

    git diff --check
    git diff --cached --check

### Exact next implementation step

Begin the SECRET local DKG layer, but do not publish anything over P2P yet.

First inspect and freeze the exact existing APIs in:

    internal/rabbitvrfstate/dkg_transport_key_store_v1.go
    consensus/lqc/rabbit_vrf_dkg_transport_key_v1.go

Resolve precisely:

1. transport-key store constructor;
2. canonical directory/path expectations;
3. Create API;
4. Load API;
5. missing/already-exists/decrypt error behavior;
6. password input contract;
7. binding construction API;
8. Participant wallet authentication API;
9. how to compare the generated transport public key against:
   - Participant wallet public key;
   - local P2P node public key.

Then implement the smallest local secret-runtime slice:

    canonical local DKG member
      -> local secret-state path
      -> load existing transport key
         OR safely create exactly one
      -> bind SessionID + ShareID + Participant
      -> reject key reuse with Participant wallet
      -> reject key reuse with P2P node key
      -> keep all state local
      -> NO P2P publication yet

Fail closed:

- no transport key for non-members;
- no transport key before canonical DKG preparation;
- no secret creation from VerifyHeader, Finalize, replay or branch reconstruction;
- no silent regeneration after an existing record;
- wrong password must not create a replacement;
- corrupt secret state must not create a replacement;
- never persist plaintext private keys;
- never log passwords or private key material;
- never reuse Participant or P2P node keys as DKG transport keys.

Public Testnet Rabbit VRF remains disabled until an activation block is
explicitly configured.

### Resume instruction

If a new ChatGPT conversation is required, send:

    Leia docs/consensus/rabbit-vrf-testnet-handoff.md, confira o HEAD e o git status sem apagar nenhuma alteração, e continue do "Exact next implementation step". Estamos no worktree ~/projects/rabbit-geth-vrf e Rabbit VRF continua desativado na Testnet pública.

Do not use git clean or git reset --hard.

Do not delete datadirs, blockchain state, keystores, WorkSeats or backups.
