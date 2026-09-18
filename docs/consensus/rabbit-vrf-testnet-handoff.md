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
    anti-withholding rules
    anti-spam behavior

Product:

    callback execution/retry semantics
    explorer indexing
    public playground
    developer documentation
    release packaging
    multinode testing

## Exact next implementation step

Do NOT activate Rabbit VRF yet.

Next step:

Inspect how this repository packages and installs EVM system predeploy bytecode
and how fork-time state transitions install new system contracts.

Then implement the smallest first slice:

    RabbitVRFCoordinatorV1 predeploy skeleton
    +
    deterministic PreExecution system call
    +
    tests

The first implementation slice should only establish coordinator/predeploy and
TWAP observation plumbing.

It must NOT yet enable public Testnet VRF requests.

VRFProtocolBlock must remain disabled in the public Testnet configuration until
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
