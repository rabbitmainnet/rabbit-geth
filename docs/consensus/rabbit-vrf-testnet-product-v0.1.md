# Rabbit VRF Testnet Product V0.1

Status: DRAFT / NOT ACTIVATABLE
Network: Rabbit Chain Testnet
Chain ID: 9280
Native test currency: tRAB
Mainnet scope: NONE

## 1. Purpose

This document defines the product and EVM-facing behavior required to expose
Rabbit VRF to Testnet users and developers.

The cryptographic randomness source remains consensus-native.

No contract, RPC server, website operator or administrator may choose, replace
or override a Rabbit VRF randomness result.

The Testnet product layer MUST make the protocol usable, observable and
testable without weakening the consensus guarantees defined by the Rabbit VRF
protocol specification.

## 2. Testnet goals

Rabbit VRF Testnet V0.1 MUST allow a user or dApp to:

- request verifiable randomness;
- pay the request cost in tRAB;
- receive a deterministic request identifier;
- observe the VRF round lifecycle;
- receive the final randomness result;
- verify the result and associated protocol metadata;
- optionally receive a contract callback;
- inspect fees, rewards, timeout state and fulfillment state;
- reproduce example use cases from a public playground.

## 3. Non-goals

V0.1 MUST NOT:

- activate Rabbit VRF on Mainnet;
- introduce an administrator able to replace VRF results;
- allow a coordinator owner to choose committee members;
- allow a coordinator owner to change the threshold;
- allow a website or RPC operator to decide whether a valid result exists;
- hide protocol failure behind trusted fallback randomness;
- use centralized off-chain randomness as a fallback.

## 4. End-to-end request lifecycle

The target Testnet lifecycle is:

1. User or contract submits a VRF request.
2. Request receives a canonical request ID.
3. Required tRAB payment is locked or charged.
4. The request is assigned to a canonical VRF round.
5. The active epoch committee processes the round.
6. Participants submit valid threshold partial signatures.
7. The protocol reaches threshold or reaches its failure condition.
8. The threshold signature is reconstructed and verified.
9. Final Rabbit VRF randomness is derived.
10. The request is marked fulfilled.
11. Protocol rewards are accounted.
12. Any required callback is executed.
13. Request, round, fee and result data become publicly inspectable.

The exact consensus rules for steps 4 through 9 remain defined by the Rabbit
VRF protocol specification.

## 5. EVM architecture

The preferred V0.1 architecture is:

- consensus-native Rabbit VRF engine;
- deterministic EVM-visible system interface or coordinator;
- consumer contracts;
- read-only protocol metadata;
- public events for requests and fulfillment.

Canonical Testnet V0.1 names:

    RabbitVRFCoordinatorV1
    IRabbitVRFCoordinatorV1
    IRabbitVRFConsumerV1

These names are frozen for Testnet V0.1.

The coordinator MUST NOT generate randomness itself.

The coordinator MUST NOT contain a privileged method capable of replacing a
randomness result.

The coordinator MUST NOT contain a privileged method capable of replacing the
active threshold public key, committee or threshold rules.

DECIDED FOR TESTNET V0.1:

- Rabbit VRF cryptographic execution and consensus decisions remain
  consensus-native.
- The EVM-facing RabbitVRFCoordinatorV1 will be an immutable system predeploy.
- The system predeploy is a deterministic EVM facade over consensus-native VRF
  state and MUST NOT generate or choose randomness itself.
- Consensus controls epoch, committee, threshold, threshold public key,
  canonical round validity and final randomness.
- The coordinator may expose requests, payments, fulfillment state, callbacks,
  events and read-only protocol metadata.
- There is no mutable owner/admin upgrade path in V0.1.
- No privileged account may replace randomness, committee membership,
  threshold parameters or the active threshold public key.

DECIDED SYSTEM PREDEPLOY ADDRESS:

    0xdFc21aeA108e3F527E5f236ebf354dc8262719da

The address is deterministically derived as the low 20 bytes of:

    keccak256("RABBIT_VRF_COORDINATOR_V1")

The derivation is part of the Testnet V0.1 specification and avoids the
standard low precompile range and all system/predeploy addresses currently
reserved by Rabbit Core.

OPEN:
- Freeze the exact native-to-EVM fulfillment mechanism.

## 6. Request interface

A request MUST expose enough information to prevent ambiguity between different
users, epochs, rounds and applications.

DECIDED REQUEST ABI FOR TESTNET V0.1:

    requestRandomness(
        uint32 callbackGasLimit,
        bytes32 appDataHash
    )
        external
        payable
        returns (bytes32 requestId)

    quoteRequestFee(
        uint32 callbackGasLimit
    )
        external
        view
        returns (uint256 fee)

    nextRequestNonce(
        address requester
    )
        external
        view
        returns (uint64 nonce)

    getRequest(
        bytes32 requestId
    )
        external
        view
        returns (
            address requester,
            uint64 requesterNonce,
            uint64 requestBlock,
            uint64 epoch,
            uint64 round,
            uint32 callbackGasLimit,
            uint256 feePaid,
            bytes32 appDataHash,
            bytes32 randomness,
            bytes32 proofHash,
            uint8 status
        )

Testnet V0.1 uses the same request entry point for EOAs and contracts.

Rules:

- msg.sender is the canonical requester.
- A contract requesting a callback is also the callback consumer.
- callbackGasLimit == 0 means no callback is requested.
- callbackGasLimit > 0 requests a callback to msg.sender.
- The requester nonce starts at zero.
- The requester nonce increments only after a successful request.
- msg.value MUST equal quoteRequestFee(callbackGasLimit).
- Ordinary transaction gas is separate from the Rabbit VRF protocol fee.
- appDataHash is application-defined metadata binding and MAY be bytes32(0).

The canonical request domain is:

    keccak256("RABBIT_VRF_REQUEST_V1")

The canonical requestId is:

    keccak256(
        abi.encode(
            REQUEST_DOMAIN_V1,
            block.chainid,
            address(this),
            msg.sender,
            requesterNonce,
            callbackGasLimit,
            appDataHash
        )
    )

The requestId MUST be calculated before incrementing the requester nonce.

Canonical request statuses:

    0 = NONE
    1 = PENDING
    2 = FULFILLED
    3 = EXPIRED
    4 = FAILED

Canonical V0.1 events:

    RandomnessRequested(
        bytes32 indexed requestId,
        address indexed requester,
        uint64 indexed requesterNonce,
        uint32 callbackGasLimit,
        bytes32 appDataHash,
        uint256 feePaid
    )

    RandomnessFulfilled(
        bytes32 indexed requestId,
        uint64 indexed epoch,
        uint64 indexed round,
        bytes32 randomness,
        bytes32 proofHash
    )

    RandomnessRequestExpired(
        bytes32 indexed requestId,
        uint256 refundAmount
    )

    RandomnessCallbackResult(
        bytes32 indexed requestId,
        address indexed consumer,
        bool success
    )

OPEN:
- Freeze maximum callback gas.
- Freeze callback execution/retry rules.
- Freeze the canonical threshold proof bytes committed by proofHash.

## 7. Payment model

Testnet V0.1 uses a pay-per-request model.

The Rabbit VRF service price is denominated in tRUSD, but users pay the
protocol fee in native tRAB.

Canonical Testnet pricing assets:

    tRUSD:
        0xaB9fEC2ff2b4f481F585b2358f3842C66e0194bd
        decimals = 6

    tWRAB:
        0xef03f43ed1cb21d56cb0b26934d09cabc1994c8d
        decimals = 18

    RabbitSwap tRUSD/tWRAB Pair:
        0x8b9f4581b71964049ac6be03b22000132438b385

    RabbitSwap Factory:
        0x3455FF1c81B8FC1D8229019766495cD2a9A6C577

    RabbitSwap Router02:
        0xF5A9BF9Df2c6CEb8987b2cb26f4CcE310577A7b0

The verified RabbitSwap pair has:

    token0 = tRUSD
    token1 = tWRAB

and exposes:

    getReserves()
    price0CumulativeLast()
    price1CumulativeLast()

The pair uses UQ112x112 cumulative pricing.

Because token0 is tRUSD and token1 is tWRAB, Rabbit VRF V0.1 uses the
time-weighted `price0` direction to convert a tRUSD-denominated protocol fee
into tRAB wei.

The protocol MUST NOT use the instantaneous reserve ratio as the canonical VRF
billing price.

The protocol MUST NOT use a website, RPC operator, centralized API or trusted
off-chain price feed as the canonical VRF billing price.

Canonical TWAP arithmetic:

    Q112 = 2 ** 112

    averagePrice0X112 =
        (
            currentPrice0Cumulative
            - previousPrice0Cumulative
        )
        / elapsedSeconds

For a configured VRF service price expressed in tRUSD base units:

    protocolFeeWei =
        ceil(
            vrfFeeTRUSDBaseUnits
            * averagePrice0X112
            / Q112
        )

Because tRUSD base units use 6 decimals and tWRAB uses 18 decimals, the raw
UQ112x112 pair price already performs the required base-unit conversion.
No floating-point arithmetic is permitted.

The conversion MUST round upward so the protocol is not underpaid because of
integer truncation.

The canonical current cumulative price MUST account for elapsed time since the
pair's last reserve update using the same reserve ratio and uint32 timestamp
semantics as RabbitSwapPair.

A lack of swaps MUST NOT freeze the VRF price oracle.

TWAP observations MUST be maintained deterministically by protocol-controlled
state. A user request MUST NOT be allowed to choose or initialize its own TWAP
baseline.

If a valid TWAP observation is unavailable, stale beyond the protocol limit or
otherwise invalid, a new VRF request MUST fail deterministically.

There is no trusted fallback price.

The VRF protocol fee is separate from ordinary transaction gas.

Callback execution funding is also separate from the VRF protocol fee.

The 50/30/20 Rabbit VRF reward split applies only to the VRF protocol fee.
Callback gas funding MUST NOT alter that split.

Frozen Testnet V0.1 pricing parameters:

    VRF_BASE_FEE_TRUSD_BASE_UNITS = 10_000
    TWAP_MIN_WINDOW_SECONDS = 1_800
    TWAP_OBSERVATION_CADENCE_SECONDS = 300
    TWAP_MAX_AGE_SECONDS = 3_600

`tRUSD` has 6 decimals, therefore:

    10_000 tRUSD base units = 0.01 tRUSD

The canonical Testnet V0.1 VRF service price is therefore 0.01 tRUSD per
request, converted to native tRAB using the canonical RabbitSwap TWAP.

The TWAP observation used for billing MUST span at least 1,800 seconds.

Canonical protocol observations SHOULD be advanced no more frequently than
once every 300 seconds.

An observation older than 3,600 seconds MUST NOT be accepted as a valid
billing baseline.

These pricing and TWAP parameters are protocol constants for Testnet V0.1.

The coordinator MUST NOT expose an owner, admin or privileged setter capable
of changing the service price, TWAP window, observation cadence or maximum
observation age.

Changing any of these Testnet V0.1 pricing constants requires a protocol fork.

Frozen deterministic TWAP observation mechanism:

The Rabbit VRF pricing oracle is advanced by a consensus-driven EVM system
call during `PreExecution`.

The system call MUST execute only when:

    ChainConfig.IsRabbitVRF(blockNumber) == true

`VRFProtocolBlock == 0` means the Rabbit VRF protocol, including its pricing
oracle system call, is disabled.

The pricing observation system call:

- executes before normal transactions in the block;
- originates from `params.SystemAddress`;
- targets `RabbitVRFCoordinatorV1`;
- has no keeper, relayer or externally owned operator;
- cannot be triggered as a privileged pricing update by a user;
- cannot be controlled by an owner or admin.

Running the pricing update before normal transactions ensures that swaps made
inside the current block cannot change the canonical billing observation used
by VRF requests in that same block.

`RabbitVRFCoordinatorV1` maintains a fixed-size circular buffer of:

    TWAP_OBSERVATION_RING_SIZE = 16

Each observation contains:

    observedAt
    price0Cumulative

where `observedAt` is the canonical block timestamp of the system observation
and `price0Cumulative` is the counterfactual RabbitSwap `price0` cumulative
value for that timestamp.

The current cumulative value MUST be derived from RabbitSwapPair using the same
semantics as the verified pair implementation.

Given:

    storedPrice0Cumulative = pair.price0CumulativeLast()

and:

    (reserve0, reserve1, pairTimestampLast) = pair.getReserves()

the protocol computes the counterfactual cumulative value using the same
UQ112x112 reserve ratio and uint32 timestamp arithmetic as RabbitSwapPair.

Conceptually:

    currentTimestamp32 = uint32(block.timestamp)

    elapsed32 =
        currentTimestamp32 - pairTimestampLast

    currentPrice0Cumulative =
        storedPrice0Cumulative
        +
        UQ112x112(reserve1 / reserve0) * elapsed32

The uint32 subtraction MUST preserve the same wraparound semantics used by
RabbitSwapPair.

If either reserve is zero, the protocol MUST NOT fabricate a price.

An invalid or temporarily unavailable pair price MUST NOT cause a trusted
fallback price to be used.

The observation ring is advanced deterministically.

If no prior observation exists, the first valid pre-execution observation is
stored.

Otherwise, a new observation is stored only when:

    block.timestamp - newestObservation.observedAt
        >= TWAP_OBSERVATION_CADENCE_SECONDS

where:

    TWAP_OBSERVATION_CADENCE_SECONDS = 300

Because the system call executes every active block, the first valid block at
or after the cadence boundary advances the observation ring.

A VRF request uses only protocol-recorded observations.

The request MUST NOT use the instantaneous pair reserve ratio and MUST NOT use
a price observation created by the requesting transaction.

For billing, let `latest` be the newest valid protocol observation.

The protocol selects the newest older observation `baseline` satisfying:

    latest.observedAt - baseline.observedAt
        >= TWAP_MIN_WINDOW_SECONDS

and:

    block.timestamp - baseline.observedAt
        <= TWAP_MAX_AGE_SECONDS

where:

    TWAP_MIN_WINDOW_SECONDS = 1800
    TWAP_MAX_AGE_SECONDS = 3600

The canonical TWAP is:

    averagePrice0X112 =
        (
            latest.price0Cumulative
            - baseline.price0Cumulative
        )
        /
        (
            latest.observedAt
            - baseline.observedAt
        )

The selected observation pair is deterministic.

If multiple baseline observations satisfy the rules, the newest satisfying
baseline MUST be selected.

If no valid baseline exists, a new VRF request MUST fail deterministically.

This includes the initial protocol warm-up period after VRF activation and
recovery after a sufficiently long observation gap.

The protocol MUST accumulate at least `TWAP_MIN_WINDOW_SECONDS` of valid price
history before accepting new requests.

There is no administrative bypass and no trusted fallback during warm-up.

The 16-entry ring provides sufficient history for the frozen Testnet V0.1
cadence, minimum window and maximum age while keeping bounded state.

The exact coordinator storage layout and system-call function selector will be
frozen with the `RabbitVRFCoordinatorV1` implementation.

OPEN:
- Freeze callback gas prepayment/accounting.
- Freeze unused callback budget refund behavior.
- Freeze failed/expired request refund behavior.

No production Mainnet price or oracle configuration is defined by this
document.

## 8. Reward model

Rabbit VRF rewards MUST incentivize correct and timely participation without
creating an advantage for withholding or selective participation.

DECIDED FOR TESTNET V0.1:

The Rabbit VRF protocol fee is split independently from the normal Rabbit block
reward:

    50% = Producer
    30% = VRF Committee
    20% = Rabbit Allocation

Canonical basis-point constants:

    VRF_PRODUCER_BPS = 5000
    VRF_COMMITTEE_BPS = 3000
    VRF_RABBIT_BPS = 2000
    VRF_TOTAL_BPS = 10000

For a successfully fulfilled request with protocol fee `feePaid`:

    producerReward =
        feePaid * VRF_PRODUCER_BPS / VRF_TOTAL_BPS

    committeeReward =
        feePaid * VRF_COMMITTEE_BPS / VRF_TOTAL_BPS

    rabbitAllocation =
        feePaid - producerReward - committeeReward

Integer division uses normal EVM floor division.

Any integer-division remainder is therefore assigned deterministically to the
Rabbit Allocation so that:

    producerReward
    + committeeReward
    + rabbitAllocation
    == feePaid

The Producer share belongs to the canonical Rabbit block producer responsible
for the consensus-defined successful fulfillment inclusion.

The Committee share belongs to the VRF committee reward pool for that fulfilled
request.

The Rabbit Allocation is a protocol-defined allocation and MUST NOT be
controlled by the RabbitVRFCoordinatorV1 owner because V0.1 has no mutable
owner/admin role.

The Rabbit VRF 50/30/20 split is independent from the existing Rabbit block
reward 70/30 split. The two reward systems MUST NOT be mixed implicitly.

OPEN:
- Freeze how the 30% committee pool is divided between valid contributors.
- Freeze treatment of late partial signatures.
- Freeze treatment of invalid partial signatures.
- Freeze treatment of offline participants.
- Freeze the destination and internal policy for the 20% Rabbit Allocation.
- Define anti-withholding incentives.
- Define anti-spam economics.
- Freeze reward behavior for expired or failed requests.

## 9. Failure and refund behavior

Rabbit VRF MUST have explicit deterministic behavior when threshold is not
reached.

Possible request terminal states:

- fulfilled;
- expired;
- failed;
- cancelled only if protocol rules explicitly allow cancellation.

OPEN:
- Freeze timeout measured in blocks, rounds or time.
- Freeze retry behavior.
- Freeze whether failed requests automatically move to another round.
- Freeze refund behavior.
- Freeze whether any participant reward is paid on failed rounds.
- Freeze callback behavior after expiration.

There MUST NOT be trusted manual fulfillment.

## 10. Callback model

A consumer contract MAY request automatic fulfillment.

Target callback shape:

    rawFulfillRandomness(requestId, randomness)

The final ABI is not frozen.

The callback execution MUST NOT be capable of changing the already-finalized
randomness.

A callback revert MUST NOT invalidate the underlying valid Rabbit VRF result.

OPEN:
- Freeze callback ABI.
- Freeze callback gas accounting.
- Freeze retry policy after callback revert.
- Decide whether failed callbacks may be manually retried permissionlessly.
- Define reentrancy protections.
- Define maximum callback gas.

## 11. Public Testnet playground

The Rabbit Chain website SHOULD provide a dedicated Rabbit VRF Testnet page.

Target user flow:

    Connect Wallet
        ->
    Add Rabbit Testnet
        ->
    Obtain / hold tRAB
        ->
    Choose demo
        ->
    See estimated VRF cost
        ->
    Request Randomness
        ->
    Watch round progress
        ->
    Receive result
        ->
    Verify result

Initial public demos SHOULD include:

- Random Number;
- Coin Flip;
- Dice Roll;
- Raffle Draw;
- NFT Reveal Seed;
- Game / Loot Randomness;
- Contract Callback Demo.

These demos MUST use the real Testnet Rabbit VRF request path.

They MUST NOT substitute frontend pseudo-randomness for Rabbit VRF output.

## 12. Request status UI

The website SHOULD expose states such as:

- Submitted;
- Paid;
- Assigned;
- Collecting Partials;
- Threshold Reached;
- Reconstructed;
- Fulfilled;
- Callback Executed;
- Callback Failed;
- Expired / Failed.

For each request, the UI SHOULD expose:

- request ID;
- requester;
- consumer;
- transaction hash;
- epoch;
- round;
- committee reference;
- threshold;
- fee;
- callback gas limit;
- request block;
- fulfillment block;
- threshold signature or proof reference;
- final randomness;
- callback status.

## 13. Developer section

The Testnet developer documentation SHOULD provide:

- coordinator/system interface address;
- ABI;
- Solidity consumer example;
- ethers example;
- viem example;
- request transaction example;
- event definitions;
- callback example;
- fee estimation;
- timeout behavior;
- error definitions;
- result verification guidance.

A minimal developer path SHOULD require only:

1. connect to Rabbit Testnet;
2. fund with tRAB;
3. deploy or use a consumer;
4. request randomness;
5. consume the callback or query the result.

## 14. Explorer and observability

Rabbit VRF Testnet SHOULD expose protocol observability for debugging.

Required views should eventually include:

- active VRF epoch;
- threshold public key;
- committee members;
- threshold;
- current rounds;
- completed rounds;
- expired rounds;
- request count;
- fulfillment latency;
- successful threshold count;
- failed threshold count;
- participant contribution status;
- invalid partial count;
- callback success/failure;
- fee accounting;
- reward accounting.

This data MUST reflect consensus state or deterministically derived indexed
state.

## 15. Consensus vs product boundary

Consensus-critical:

- epoch;
- committee;
- threshold;
- threshold public key;
- canonical message;
- valid partial signature rules;
- threshold reconstruction;
- final randomness derivation;
- round success/failure;
- consensus-visible fee/reward rules if those affect state;
- reorg behavior;
- persistence rules.

Product / UX:

- website layout;
- labels;
- charts;
- demo presentation;
- wallet connection UX;
- API convenience endpoints;
- explorer visual presentation.

A product-layer component MUST NOT override consensus state.

## 16. Testnet adversarial product tests

Before public activation, Testnet testing MUST cover at least:

- request with insufficient balance;
- duplicate request submission;
- malformed request data;
- maximum callback gas;
- callback revert;
- callback reentrancy attempt;
- user disconnect;
- RPC disconnect;
- participant offline;
- invalid partial signature;
- duplicate partial signature;
- threshold not reached;
- threshold reached at timeout boundary;
- node restart during request;
- node restart after threshold;
- reorg before fulfillment;
- reorg after request assignment;
- epoch transition during pending request;
- spam requests;
- many concurrent requests;
- deterministic fee accounting;
- deterministic reward accounting;
- deterministic refund accounting.

## 17. Public Testnet activation gate

Rabbit VRF product activation MUST remain disabled until:

- the consensus protocol is frozen;
- DKG is implemented and tested;
- committee rules are frozen;
- threshold rules are frozen;
- epoch rules are frozen;
- canonical round message is frozen;
- persistence and restart recovery are proven;
- reorg behavior is proven;
- EVM request interface is frozen;
- fee rules are frozen;
- reward rules are frozen;
- failure/refund rules are frozen;
- callback rules are frozen;
- multi-node adversarial tests pass;
- public Testnet UI is ready for external testing.

Activation block remains UNSET.

## 18. Immediate design decisions still required

OPEN:
- Native-to-EVM fulfillment mechanism.
- Testnet VRF fee.
- Committee internal reward distribution.
- Rabbit Allocation destination and internal policy.
- Refund rules.
- Timeout rules.
- Callback ABI.
- Maximum callback gas.
- Callback gas accounting.
- Failure retry behavior.
- Canonical proofHash input bytes.
- Subscription support after V0.1.
- Explorer indexing model.

These decisions MUST be resolved before the product layer is considered frozen.
