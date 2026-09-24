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

Canonical pending-request storage semantics:

- requestBlock is the block.number of the successful request transaction.
- requestBlock MUST fit in uint64.
- epoch is stored as zero while the request is PENDING.
- round is stored as zero while the request is PENDING.
- Pending epoch == 0 and round == 0 are placeholders only and MUST NOT be
  interpreted as assignment to VRF epoch zero or round zero.
- epoch and round are assigned only by the future canonical Rabbit VRF
  fulfillment path once the request is bound to the finalized VRF result.
- randomness is bytes32(0) while PENDING.
- proofHash is bytes32(0) while PENDING.
- feePaid records only the Rabbit VRF protocol fee.
- Callback execution funding or escrow MUST NOT be included in feePaid.
- Until callback escrow and execution semantics are frozen and implemented,
  the pre-activation Testnet V0.1 coordinator MUST reject
  callbackGasLimit > 0.
- callbackGasLimit == 0 is the supported request mode during this
  pre-activation implementation stage.
- quoteRequestFee(0) returns the canonical Rabbit VRF protocol fee.
- quoteRequestFee(callbackGasLimit > 0) MUST reject while callback funding is
  not implemented.

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

For the selected `latest` and `baseline` observations:

    elapsedSeconds =
        latest.observedAt
        - baseline.observedAt

    deltaPrice0Cumulative =
        unchecked(
            latest.price0Cumulative
            - baseline.price0Cumulative
        )

The cumulative subtraction MUST use uint256 wraparound semantics matching the
RabbitSwapPair cumulative-price accumulator.

The conceptual TWAP price is:

    averagePrice0X112 =
        deltaPrice0Cumulative
        / elapsedSeconds

However, canonical billing MUST NOT first truncate `averagePrice0X112` and then
perform the fee conversion.

For a configured VRF service price expressed in tRUSD base units, the canonical
fee is computed directly as:

    protocolFeeWei =
        ceil(
            vrfFeeTRUSDBaseUnits
            * deltaPrice0Cumulative
            /
            (
                elapsedSeconds
                * Q112
            )
        )

The implementation MUST perform this multiply-divide operation with sufficient
intermediate precision to avoid uint256 multiplication overflow and MUST round
the final quotient upward.

Equivalently, the implementation requires a full-precision `mulDiv` operation
with rounding up. Intermediate integer truncation before the final division is
not permitted.

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

The 30/50/20 Rabbit VRF reward split applies only to the VRF protocol fee.
Callback gas funding MUST NOT alter that split.

Frozen Testnet V0.1 pricing parameters:

    VRF_BASE_FEE_TRUSD_BASE_UNITS = 1_000
    TWAP_MIN_WINDOW_SECONDS = 1_800
    TWAP_OBSERVATION_CADENCE_SECONDS = 300
    TWAP_MAX_AGE_SECONDS = 3_600

`tRUSD` has 6 decimals, therefore:

    1_000 tRUSD base units = 0.001 tRUSD

The canonical Testnet V0.1 VRF service price is therefore 0.001 tRUSD per
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

The `latest` observation itself MUST satisfy:

    block.timestamp - latest.observedAt
        <= TWAP_MAX_AGE_SECONDS

The protocol then selects the newest older observation `baseline` satisfying:

    latest.observedAt - baseline.observedAt
        >= TWAP_MIN_WINDOW_SECONDS

and:

    block.timestamp - baseline.observedAt
        <= TWAP_MAX_AGE_SECONDS

where:

    TWAP_MIN_WINDOW_SECONDS = 1800
    TWAP_MAX_AGE_SECONDS = 3600

Both observations therefore remain bounded by the same maximum protocol age.
An old `latest` observation MUST NOT remain usable merely because no newer
observation has been recorded.

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

If `latest` is stale, if no valid baseline exists, or if either selected
observation is otherwise invalid, a new VRF request MUST fail deterministically.

This includes:

- the initial protocol warm-up period after VRF activation;
- an observation outage long enough to make the retained history stale;
- recovery after such an outage.

Recording one fresh observation after an outage MUST NOT immediately restore
billing. The protocol MUST accumulate a new valid observation window spanning
at least `TWAP_MIN_WINDOW_SECONDS` before accepting requests again.

The protocol MUST therefore accumulate at least `TWAP_MIN_WINDOW_SECONDS` of
valid price history before initial service and after stale-history recovery.

There is no administrative bypass, manually supplied baseline or trusted
fallback during warm-up or recovery.

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

    30% = Producer
    50% = VRF Committee
    20% = Rabbit Allocation

Canonical basis-point constants:

    VRF_PRODUCER_BPS = 3000
    VRF_COMMITTEE_BPS = 5000
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

The Rabbit VRF 30/50/20 split is independent from the existing Rabbit block
reward 70/30 split. The two reward systems MUST NOT be mixed implicitly.

Frozen Testnet V0.1 reward-accounting architecture:

    VRF_SETTLEMENT_PERIOD_BLOCKS = 128

The settlement schedule is anchored to the Rabbit VRF activation block.

For an active Rabbit VRF block `B`:

    settlementPeriod =
        (B - VRFProtocolBlock)
        / VRF_SETTLEMENT_PERIOD_BLOCKS

using normal integer floor division.

Therefore:

    period 0 =
        VRFProtocolBlock
        through
        VRFProtocolBlock + 127

    period 1 =
        VRFProtocolBlock + 128
        through
        VRFProtocolBlock + 255

and so on.

This guarantees that every complete Rabbit VRF settlement period contains
exactly 128 Rabbit VRF-active blocks regardless of the chosen activation
height.

A successful VRF fulfillment is the economic accounting point.

For every request that transitions canonically to `FULFILLED`:

1. `producerReward` is credited to the canonical Rabbit producer of the block
   containing that successful fulfillment.
2. `committeeReward` is credited to the canonical VRF committee reward pool
   associated with that fulfillment.
3. `rabbitAllocation` is credited to the Rabbit Allocation accounting ledger.
4. The callback escrow, if any, remains completely separate from all three
   reward credits.

The producer reward therefore belongs to the producer of the fulfillment block,
not the producer of the original request block.

Multiple rewards belonging to the same recipient during one settlement period
MAY be aggregated into one pending balance.

Reward accounting MUST use a pull-based settlement model.

A successful fulfillment MUST NOT iterate over reward recipients and MUST NOT
perform arbitrary external value transfers to miners or committee members.

Credits created during settlement period `P` are `pending` during `P`.

When the chain enters a later settlement period, credits from earlier periods
are mature and MAY become `claimable`.

The 128-block settlement boundary is a logical accounting pulse. It MUST NOT
require a global loop over miners, committee members or reward accounts.

Maturation SHOULD be performed lazily when an affected reward account is
credited, queried or claimed, so protocol cost remains bounded independently
of the total number of historical participants.

A participant MAY accumulate rewards across multiple settlement periods before
claiming them.

A reward claim is an ordinary EVM transaction and MUST NOT be required for
Rabbit block production, VRF threshold completion or consensus liveness.

The reward implementation MUST follow checks-effects-interactions or an
equivalent reentrancy-safe withdrawal design.

The canonical producer recipient is consensus-derived. A block producer MUST
NOT be allowed to nominate a different address as the producer reward recipient
for an already-defined fulfillment.

The canonical committee reward recipients MUST be derived from protocol-
verifiable VRF participation evidence. The block producer, requester,
coordinator caller, website, relayer or administrator MUST NOT be able to
supply an arbitrary committee reward list.

The exact rule deciding which valid committee contributors share the 30% pool
is not yet frozen. Until that rule is frozen, no production implementation may
invent a committee distribution policy.

A request that has not reached a canonical valid fulfillment MUST NOT create the
30/50/20 fulfillment reward credits.

The eventual refund amount for an expired or failed request remains a separate
rule and is not defined by this reward-accounting section.

Coordinator native balance MUST NOT be treated as equivalent to distributable
VRF revenue.

Only protocol-tracked request fees, escrows, refunds and reward credits may
participate in Rabbit VRF accounting. Untracked native balance already present
at the coordinator address MUST NOT become claimable merely because the
coordinator code is installed.

OPEN:
- Freeze how the 30% committee pool is divided between valid contributors.
- Freeze treatment of late partial signatures.
- Freeze treatment of invalid partial signatures.
- Freeze treatment of offline participants.
- Freeze the destination and internal policy for the 20% Rabbit Allocation.
- Freeze the exact claim ABI and payout-address policy.
- Define anti-withholding incentives.
- Define anti-spam economics.

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

### Frozen VRF committee derivation foundation

Testnet V0.1 freezes the deterministic committee-candidate foundation as
follows:

- The source MUST be a validated CLOSED `WorkEpochSnapshotV1`.
- The source epoch, canonical `SelectionRoot`, canonical WorkSeats and the
  already-derived closed-epoch RandomX selection entropy are consensus input.
- Rabbit VRF derives its own committee seed under the domain
  `RABBIT-VRF-COMMITTEE-SEED-V1`.
- The seed binds the Rabbit VRF committee version, chain ID, closed source
  epoch, canonical selection root and canonical closed-epoch entropy.
- Block number, parent hash, producer identity, heartbeat state, jail state and
  other mutable per-block liveness state MUST NOT be inputs to the VRF
  committee seed.
- VRF committee sizing reuses Rabbit's existing dynamic committee-size rule and
  caps the result to the canonical seats actually available.
- VRF committee derivation does NOT reserve producer or fallback positions.
- Committee seats are selected deterministically from the canonical closed
  WorkSeat snapshot.
- No owner, administrator, requester or block producer may choose or replace
  committee members.

This derivation produces a deterministic VRF committee candidate only.

The deterministic source-to-target epoch schedule is frozen separately below.
Keyset qualification, successful activation and failure behavior remain separate
lifecycle rules.


### Frozen VRF committee ShareID and commitment

Testnet V0.1 also freezes the committee identity layer:

- The deterministic VRF committee ordering is consensus input.
- Each committee member receives one immutable ShareID derived only from its
  canonical committee position.
- ShareID assignment is:

      ShareID = committee_position + 1

- ShareID zero is invalid by construction.
- ShareIDs MUST NOT be renumbered because of liveness, absence, DKG complaints,
  disqualification or later qualification decisions.
- The ordered committee is committed under the domain
  `RABBIT-VRF-COMMITTEE-ROOT-V1`.
- The committee commitment binds:
  - Rabbit VRF committee version;
  - chain ID;
  - source Work epoch;
  - canonical source SelectionRoot;
  - deterministic committee seed;
  - every ordered member;
  - each member TicketHash;
  - each member Participant address;
  - each member immutable ShareID.
- Reordering the same members MUST change the committee commitment.
- Callers MUST NOT provide an arbitrary committee ordering when deriving the
  canonical snapshot commitment.

The committee commitment intentionally does NOT contain the VRF epoch number,
threshold, threshold public key, verification shares or DKG transcript. Those
belong to the future keyset commitment and remain OPEN.


### Frozen VRF keyset commitment foundation

Testnet V0.1 freezes the public keyset commitment primitive:

    RABBIT-VRF-KEYSET-ROOT-V1

A keyset commitment binds:

- Rabbit VRF keyset version;
- chain ID;
- VRF epoch;
- canonical VRF committee root;
- original committee size;
- declared threshold;
- threshold public key;
- canonical DKG transcript root;
- canonical public verification shares.

Verification shares use the immutable committee ShareIDs. They are canonicalized
by ascending ShareID, so network arrival order cannot change the keyset root.

Missing committee ShareIDs are preserved as gaps and MUST NOT cause
renumbering. For example, qualified ShareIDs `1,3,4` remain `1,3,4`.

The structural keyset primitive rejects:

- ShareID zero;
- ShareID greater than the original committee size;
- duplicate ShareIDs;
- invalid public verification keys;
- invalid threshold public key;
- zero threshold;
- zero transcript root;
- threshold larger than the number of committed verification shares.

This primitive does NOT prove that a DKG transcript is valid and does NOT decide
which participants are qualified.

The threshold formula is frozen separately by the Rabbit VRF DKG session
foundation below.

DKG construction, complaint/disqualification rules, VRF epoch schedule and
keyset activation policy remain separate consensus rules that MUST be frozen
before public activation.


### Frozen Rabbit VRF threshold policy and DKG session

The Rabbit VRF threshold is fixed BEFORE a DKG ceremony begins.

Let:

    N = original deterministic VRF committee size
    f = floor((N - 1) / 3)

Then:

    threshold = N - f

The threshold is derived from the ORIGINAL deterministic committee size, not
from the number of members that later survive DKG qualification.

Examples:

    N=32  -> f=10 -> threshold=22
    N=64  -> f=21 -> threshold=43
    N=100 -> f=33 -> threshold=67
    N=128 -> f=42 -> threshold=86

The V1 policy is designed around the assumption that at most `f` members may be
Byzantine, unavailable or otherwise unusable during the ceremony.

Complaints, absence, timeout or disqualification MUST NOT lower the threshold
after the DKG session begins.

If fewer than `threshold` valid qualified share holders remain, that DKG
ceremony cannot produce an activatable V1 keyset. The protocol MUST NOT reduce
the quorum to rescue that ceremony.

The deterministic DKG session domain is:

    RABBIT-VRF-DKG-SESSION-V1

The session context binds:

- DKG session version;
- chain ID;
- target VRF epoch;
- canonical committee root;
- original committee size;
- deterministic threshold;
- deterministic maximum fault budget.

The DKG session ID is Keccak256 over the canonical RLP session payload.

The session context deliberately excludes:

- block producer;
- heartbeat state;
- mutable per-block liveness ordering;
- network message arrival order;
- process-local peer order;
- administrator input.

This section freezes threshold policy and immutable DKG session identity.

Later frozen sections now define the public polynomial commitment format,
private evaluation verification, authenticated encrypted private evaluations,
session-scoped transport-key binding, encrypted crash-safe transport-key
persistence, and the deterministic source-to-target VRF epoch schedule.

Still OPEN at this lifecycle layer:

- polynomial-generation runtime lifecycle;
- confidential point-to-point DKG delivery;
- complaint evidence;
- qualification/disqualification state machine;
- transcript aggregation;
- failed-ceremony rollover behavior;
- final qualified-keyset activation lifecycle.

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
### Frozen Rabbit VRF public polynomial commitment foundation

Rabbit VRF V1 now freezes the public polynomial commitment representation used
before private DKG evaluation shares are exchanged.

Each DKG dealer uses a polynomial over the BLS12-381 scalar field Fr.

For threshold `t`, V1 requires exactly:

    t coefficient commitments

corresponding to an exact polynomial degree:

    t - 1

For scalar coefficients:

    a[0], a[1], ..., a[t-1]

the public commitments are:

    C[j] = G1 * a[j]

The commitment group is BLS12-381 G1, matching the Rabbit VRF threshold public
key group.

Each coefficient commitment uses the existing gnark-crypto canonical compressed
G1 representation:

    48 bytes

Every encoded point MUST:

- decode successfully;
- consume exactly 48 bytes;
- be on the BLS12-381 curve;
- be in the correct subgroup;
- re-encode to exactly the same canonical bytes.

V1 requires:

- C[0] to be non-infinity;
- C[t-1] to be non-infinity;
- intermediate coefficient commitments MAY be the canonical G1 point at
  infinity, representing a zero intermediate scalar coefficient.

Therefore every dealer polynomial has an exact non-zero highest degree and a
non-zero constant contribution.

The canonical dealer commitment domain is:

    RABBIT-VRF-DKG-POLY-COMMITMENT-V1

The commitment payload binds:

- commitment version;
- canonical DKG session ID;
- immutable dealer ShareID;
- ordered coefficient commitments.

The dealer commitment hash is:

    Keccak256(
        RLP(
            domain,
            version,
            sessionID,
            dealerShareID,
            orderedCoefficientCommitments
        )
    )

Changing coefficient order, dealer ShareID, DKG session, chain or target epoch
changes the resulting commitment identity.

A commitment is invalid if:

- the dealer ShareID is zero;
- the dealer ShareID exceeds the original committee size;
- the coefficient count differs from the frozen threshold;
- C[0] is infinity;
- C[t-1] is infinity;
- any G1 encoding is malformed or non-canonical;
- the commitment belongs to another DKG session.

Collections of dealer commitments are canonicalized by ascending immutable
DealerShareID.

Network arrival order MUST NOT affect the canonical collection.

Two commitments carrying the same DealerShareID are rejected rather than
resolved by arrival order or local peer preference.

The canonicalized collection deep-copies coefficient bytes so mutation of an
input object cannot modify the canonical result.

This layer contains NO private polynomial scalar coefficients.

It also does NOT yet authenticate the dealer over P2P. Dealer authentication is
a separate future message-envelope rule bound to the canonical Participant
wallet identity.

Still OPEN after this foundation:

- production polynomial generation;
- private polynomial evaluation share representation;
- private evaluation share verification against public commitments;
- authenticated/encrypted private share transport;
- complaint evidence;
- qualification/disqualification state machine;
- canonical DKG transcript;
- DKG persistence and restart recovery;
- keyset activation lifecycle.
### Frozen Rabbit VRF private evaluation verification foundation

Rabbit VRF V1 now freezes the local cryptographic verification of one dealer's
private polynomial evaluation for one recipient ShareID.

The canonical private dealer evaluation type is:

    DKGPolynomialEvaluationV1

Its encoding is exactly:

    32 bytes

representing one canonical BLS12-381 Fr scalar.

Parsing uses canonical Fr decoding. Values outside the scalar field or
non-canonical encodings are rejected.

An individual dealer evaluation MAY be zero.

This is intentional.

A zero contribution from one dealer does not imply that the recipient's final
aggregated DKG secret share may be zero. The final aggregate secret-share rule
remains separate and MUST reject an unusable zero final secret share.

For immutable recipient ShareID `x` and dealer coefficient commitments:

    C[j] = G1 * a[j]

the private evaluation:

    s = f(x)

is valid exactly when:

    G1 * s
        ==
    C[0] + x*C[1] + x^2*C[2] + ... + x^(t-1)*C[t-1]

All scalar arithmetic is over the BLS12-381 Fr field.

The pure cryptographic verifier rejects:

- recipient ShareID zero;
- empty coefficient commitment list;
- non-canonical evaluation scalar encoding;
- malformed coefficient commitments;
- an evaluation that does not satisfy the public commitment equation.

The consensus wrapper additionally requires:

- a valid canonical DKG session context;
- recipient ShareID <= original deterministic committee size;
- a valid dealer polynomial commitment bound to that DKG session.

Therefore a private evaluation from another chain, epoch or DKG session cannot
be accepted simply by reusing its scalar bytes.

The implementation currently remains LOCAL and PURE.

It does NOT yet define:

- a network message;
- sender authentication;
- recipient authentication;
- encryption;
- replay protection;
- complaint evidence;
- persistence.

Implementation files:

    crypto/rabbitvrf/dkg_evaluation_v1.go
    crypto/rabbitvrf/dkg_evaluation_v1_test.go
    consensus/lqc/rabbit_vrf_dkg_evaluation_v1.go
    consensus/lqc/rabbit_vrf_dkg_evaluation_v1_test.go

Still OPEN after this foundation:

- authenticated DKG message envelope;
- Participant-wallet binding of DKG senders;
- transport encryption public-key binding;
- encrypted private evaluation transport;
- complaint/evidence format;
- qualification/disqualification state machine;
- canonical DKG transcript;
- crash-safe secret persistence;
- restart/reorg recovery;
- keyset activation lifecycle.
### Frozen Rabbit VRF authenticated DKG envelope foundation

Rabbit VRF V1 now freezes the authenticated envelope foundation for PUBLIC DKG
protocol objects.

Implementation files:

    consensus/lqc/rabbit_vrf_dkg_envelope_v1.go
    consensus/lqc/rabbit_vrf_dkg_envelope_v1_test.go

Envelope version:

    1

Current V1 public message type:

    1 = polynomial commitment

Canonical signature domain:

    RABBIT-VRF-DKG-ENVELOPE-SIGN-V1

Canonical equivocation-slot domain:

    RABBIT-VRF-DKG-ENVELOPE-SLOT-V1

Canonical exact-message identity domain:

    RABBIT-VRF-DKG-ENVELOPE-ID-V1

The signed envelope binds:

    envelope version
    canonical DKG SessionID
    message type
    immutable sender ShareID
    canonical sender Participant address
    canonical payload hash

The exact RLP signing payload is supplied to:

    accounts.Wallet.SignData

The wallet signs:

    Keccak256(canonical RLP signing bytes)

Verification recovers the secp256k1 signer and requires:

    recovered signer == canonical committee Participant

The P2P node key is NOT a substitute for the Participant wallet identity.

The sender mapping remains:

    immutable ShareID -> canonical committee Participant

where ShareID was fixed by the committed deterministic VRF committee and is
never renumbered by DKG participation, complaints, liveness or qualification.

For the current polynomial commitment message:

    PayloadHash = canonical dealer polynomial commitment root

The polynomial commitment is validated against the same DKG session before its
root is accepted as an envelope payload hash.

V1 uses two deterministic identities.

The singleton public-message slot identity binds:

    envelope version
    DKG SessionID
    message type
    sender ShareID

under:

    RABBIT-VRF-DKG-ENVELOPE-SLOT-V1

PayloadHash and Participant are deliberately excluded from SlotID.

Because ShareID has one canonical Participant, two otherwise valid signed
messages for the same:

    SessionID + message type + ShareID

occupy the same slot.

The exact envelope identity binds:

    SlotID
    Participant
    PayloadHash

under:

    RABBIT-VRF-DKG-ENVELOPE-ID-V1

Signature bytes are deliberately excluded from EnvelopeID.

Therefore:

- retransmitting the same authenticated object retains the same EnvelopeID;
- changing signature bytes does not change canonical object identity;
- changing the public payload changes EnvelopeID;
- two valid signed different payloads for the same singleton slot have the same
  SlotID and different EnvelopeIDs;
- those two signed objects can later serve as objective equivocation evidence.

No monotonic per-sender sequence number is required for this singleton public
polynomial-commitment message.

Cross-session and cross-chain replay are rejected because SessionID already
binds the canonical DKG context, including ChainID.

Tests cover:

- valid Participant signature;
- deterministic SlotID and EnvelopeID;
- signature-independent canonical identity;
- tampered session;
- tampered message type;
- tampered ShareID;
- tampered Participant;
- tampered payload;
- malformed signature;
- wrong canonical committee member;
- cross-session replay rejection;
- cross-chain replay rejection;
- two separately signed conflicting public payloads in one equivocation slot;
- real accounts.Wallet.SignData compatibility through a keystore wallet;
- full LQC regression suite.

This authenticated envelope is currently for PUBLIC DKG protocol objects only.

Raw private polynomial evaluation bytes MUST NOT be broadcast through this
public envelope.

Still OPEN:

- transport encryption key generation;
- Participant-signed transport encryption public-key binding;
- recipient-bound encrypted private evaluation envelope;
- confidential point-to-point private evaluation delivery;
- complaint/evidence rules;
- qualification/disqualification state machine;
- canonical DKG transcript;
- crash-safe secret persistence;
- restart/reorg recovery;
- keyset activation lifecycle.

## Frozen Rabbit VRF DKG transport-key binding foundation

The Rabbit VRF DKG transport-encryption public-key binding is now frozen for
the Testnet implementation slice.

Protocol semantics:

- authenticated DKG envelope message type `2` is the transport-key binding;
- transport-key binding version is `1`;
- transport scheme identifier `1` means the Rabbit VRF V1 transport profile:
  secp256k1 public key with geth ECIES using its secp256k1 default
  AES-128 / SHA-256 profile;
- the canonical public-key encoding is exactly 33-byte compressed secp256k1;
- the key is scoped to one DKG session and one immutable committee ShareID;
- the binding commits to:
  - domain;
  - version;
  - SessionID;
  - ShareID;
  - Participant wallet;
  - transport scheme;
  - canonical transport public key;
- the Participant wallet authenticates the binding through the existing
  Rabbit VRF DKG envelope;
- the transport private key is separate from the Participant wallet key;
- the transport private key must also remain separate from the P2P node key;
- direct reuse of the Participant wallet public key as the transport key is
  rejected by consensus validation;
- the same `(SessionID, message type, ShareID)` singleton envelope slot is used
  for transport-key publication, so two different authenticated transport keys
  for the same slot form objective equivocation evidence.

Validation coverage includes:

- canonical 33-byte public-key parsing;
- malformed, short and oversized public-key rejection;
- Participant wallet-key reuse rejection;
- deterministic transport-key commitment root;
- Participant / ShareID binding;
- SessionID and chain replay rejection;
- authenticated envelope verification;
- transport-key tampering rejection;
- signed transport-key equivocation identity;
- real `accounts.Wallet.SignData` compatibility;
- real geth ECIES encrypt/decrypt round-trip with the bound transport key;
- rejection of decryption using an unrelated transport private key.

The ECIES round-trip test proves compatibility between the canonical transport
public-key representation and geth's ECIES implementation. It does NOT yet
freeze the private-evaluation ciphertext protocol.

Still intentionally NOT frozen in this checkpoint:

- private polynomial-evaluation ciphertext object;
- recipient binding;
- exact ECIES `s1` / `s2` shared-information values;
- ciphertext payload domain;
- ciphertext replay / dedup identity;
- complaint evidence for failed private delivery;
- encrypted transport-key private-secret persistence;
- restart recovery semantics;
- point-to-point DKG P2P messages.

Raw private polynomial evaluations MUST NOT be broadcast or placed inside the
public authenticated DKG envelope.

## Frozen Rabbit VRF DKG encrypted private-evaluation transport foundation

Rabbit VRF V1 now freezes the private-evaluation ciphertext protocol-object
layer for Testnet implementation.

Canonical plaintext:

    DKGPolynomialEvaluationV1
    exactly 32 canonical BLS12-381 Fr bytes

Encrypted object version:

    1

The object binds:

- SessionID;
- dealer immutable ShareID;
- dealer Participant wallet;
- recipient immutable ShareID;
- recipient Participant wallet;
- dealer polynomial CommitmentRoot;
- authenticated recipient TransportKeyRoot;
- ciphertext;
- dealer Participant-wallet signature.

Encryption uses the recipient's authenticated session transport key and geth
ECIES with the Rabbit VRF V1 secp256k1 AES-128 / SHA-256 profile.

For the canonical 32-byte plaintext, Rabbit VRF V1 freezes the ciphertext size
at 145 bytes.

Exact domain separation:

    RABBIT-VRF-DKG-EVAL-ECIES-KDF-V1
    RABBIT-VRF-DKG-EVAL-ECIES-MAC-V1
    RABBIT-VRF-DKG-EVAL-CIPHERTEXT-SLOT-V1
    RABBIT-VRF-DKG-EVAL-CIPHERTEXT-ID-V1
    RABBIT-VRF-DKG-EVAL-CIPHERTEXT-SIGN-V1

The dealer Participant wallet signs canonical RLP metadata plus the exact
ciphertext hash.

Neither the P2P node key nor the recipient transport key may authenticate the
dealer.

ECIES randomness is intentionally not treated as semantic equivocation.

For the same semantic dealer-to-recipient delivery:

- SlotID is stable;
- independent encryptions may have different ciphertexts;
- MessageID changes with ciphertext hash;
- signature bytes do not alter MessageID.

Recipient processing verifies:

- canonical session;
- canonical dealer;
- dealer Participant-wallet signature;
- canonical recipient;
- authenticated transport-key binding;
- exact local transport private key;
- ECIES authentication;
- canonical Fr plaintext;
- Feldman polynomial-evaluation equation.

Deterministic vectors freeze:

- ECIES s1 context hash;
- ECIES s2 context hash;
- encrypted-evaluation SlotID;
- encrypted-evaluation MessageID.

Still OPEN after this foundation:

- encrypted crash-safe transport-private-key persistence;
- restart recovery of the exact previously bound transport key;
- confidential point-to-point DKG delivery;
- replay / duplicate state;
- complaint evidence;
- qualification / disqualification;
- canonical transcript;
- final secret-share aggregation;
- full DKG state recovery;
- Rabbit VRF keyset activation lifecycle.

Raw private polynomial evaluations MUST NOT be broadcast in public DKG gossip.

## Frozen Rabbit VRF DKG transport-key persistence foundation

Rabbit VRF V1 now includes an isolated encrypted persistence layer for the
session-scoped DKG transport private key.

The persisted record binds:

- canonical DKG SessionID;
- immutable ShareID;
- Participant wallet address;
- transport scheme;
- canonical compressed transport public key;
- exact encrypted private scalar.

The secret is encrypted using geth CryptoJSON V3 / scrypt primitives.

The storage layer provides:

- encrypted at-rest persistence;
- exact restart recovery;
- no silent overwrite;
- concurrent create-without-replacement protection;
- 0700 private directory permissions where supported;
- 0600 private file permissions where supported;
- temporary-file fsync;
- directory synchronization around publication;
- fail-closed wrong-password handling;
- fail-closed corruption handling;
- fail-closed metadata mismatch handling;
- fail-closed missing-state handling;
- rejection of Participant-wallet key reuse;
- rejection of inconsistent ECDSA private/public key pairs;
- Linux/Unix runtime tests;
- Go race-detector validation;
- Windows amd64 compile validation;
- full LQC regression validation.

The persistence package does not itself decide the Rabbit Core credential source
and does not announce DKG transport bindings.

Still OPEN:

- Rabbit Core runtime wiring;
- explicit runtime credential source;
- P2P node-key separation check;
- lifecycle ownership;
- restart behavior after an already-announced binding;
- confidential DKG P2P delivery;
- replay/duplicate handling;
- complaint evidence;
- dealer qualification;
- transcript aggregation;
- final keyset lifecycle.

Rabbit VRF remains disabled until the full lifecycle is implemented and tested.

### Frozen Rabbit VRF epoch and DKG preparation schedule

Rabbit VRF V1 aligns VRF epoch numbering one-for-one with Rabbit Work V1 epoch
boundaries. This numbering does not imply Rabbit VRF was active in historical
epochs before its protocol fork.

For a canonical CLOSED source Work epoch `N`:

    source Work epoch              = N
    committed/closed during        = N + 1
    committee source available     = N + 2
    DKG preparation epoch          = N + 2
    intended target VRF epoch      = N + 3

Therefore:

    TargetVRFEpoch = SourceWorkEpoch + 3

The `N + 2` interval provides exactly one complete Work epoch for DKG before the
intended target epoch begins.

With the current Work V1 epoch length of 128 blocks, source epoch 1 produces:

    DKG preparation epoch  = 3
    preparation start      = block 257
    target VRF epoch       = 4
    target start           = block 385

The DKG session remains permanently bound to its original `TargetVRFEpoch`.

A failed or unqualified ceremony MUST NOT silently mutate its target epoch and
MUST NOT lower its threshold to rescue activation.

This schedule freezes deterministic DKG session timing only. It does not by
itself activate a keyset. Qualification, failed-ceremony rollover, old-key
continuity and final live-keyset activation remain lifecycle rules that must be
frozen before public activation.
