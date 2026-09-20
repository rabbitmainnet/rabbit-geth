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

This is only the deterministic committee-candidate foundation. DKG timing,
threshold choice, keyset binding, live VRF epoch assignment and keyset
activation remain OPEN.


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
DKG transcript is valid, which members qualify, when a keyset activates or
which VRF epoch schedule is canonical.

The Rabbit VRF V1 threshold formula is now frozen separately by the DKG session
foundation.

The remaining DKG qualification and lifecycle rules remain OPEN.

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

## Exact next implementation step

Do NOT activate Rabbit VRF yet.

Current committed recovery checkpoint:

    058df1c57 docs(rabbitvrf): update dkg implementation handoff

The current uncommitted protocol slice has now implemented and tested:

    RabbitVRFThresholdPolicyV1
    RabbitVRFDKGSessionContextV1
    RabbitVRFDKGSessionIDV1
    RABBIT-VRF-DKG-SESSION-V1

The next protocol slice is:

    public polynomial commitment format

Do NOT implement private-share transport yet.

The next slice must define the public commitment to each DKG dealer/member's
polynomial before any private evaluation shares are accepted.

The design must determine and freeze:

1. Exact polynomial degree:

       threshold - 1

2. Exact BLS12-381 group used for coefficient commitments.

3. Canonical serialized representation of every coefficient commitment.

4. Domain-separated commitment message binding at least:

       DKG session ID
       dealer immutable ShareID
       ordered coefficient commitments

5. Validation rules for:

       wrong number of coefficients
       point at infinity where forbidden
       malformed/non-canonical point encoding
       wrong session
       zero or out-of-range dealer ShareID
       duplicate dealer commitment
       reordered coefficients
       cross-chain replay
       cross-epoch replay

6. Exact deterministic commitment root/hash used later by complaint evidence and
   the canonical DKG transcript.

The public polynomial commitment layer MUST NOT contain the dealer's secret
polynomial coefficients.

It MUST be possible for a recipient later to verify a private evaluation share
against the dealer's public coefficient commitments.

Do NOT yet implement:

    production private-share encryption
    private-share P2P messages
    complaints
    qualification/disqualification
    transcript finalization
    secret-share persistence
    keyset activation
    public Testnet activation

After the public commitment foundation passes deterministic and adversarial
tests, continue with:

    private share transport/authentication
    complaint evidence
    qualification/disqualification state machine
    canonical transcript root
    crash-safe persistence
    restart/reorg recovery
    keyset activation lifecycle
    canonical VRF round message

VRFProtocolBlock MUST remain disabled in the public Testnet configuration until
the complete activation gate is satisfied.

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
