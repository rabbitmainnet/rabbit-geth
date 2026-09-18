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

V0.1 is intended to use native Testnet tRAB.

Preferred initial product model:

    pay-per-request

A request SHOULD NOT require a subscription for the first public Testnet
version.

The VRF-specific request fee is separate from ordinary transaction gas.

The fee model MUST be deterministic and publicly inspectable.

OPEN:
- Freeze the Testnet VRF base fee.
- Decide whether callback gas is prepaid separately.
- Decide whether the fee changes with committee size or threshold.
- Decide whether fee changes require a protocol fork.
- Decide whether unused callback budget is refunded.
- Decide whether failed rounds receive full, partial or zero refund.

No production Mainnet fee is defined by this document.

## 8. Reward model

Rabbit VRF rewards MUST incentivize correct and timely participation without
creating an advantage for withholding or selective participation.

Possible reward recipients include:

- participants that submitted valid partial signatures;
- the final block producer performing deterministic inclusion;
- other protocol roles explicitly defined by consensus.

OPEN:
- Freeze who earns a VRF reward.
- Freeze whether only valid contributors earn or the full eligible committee
  shares the reward.
- Freeze reward split.
- Freeze treatment of late partial signatures.
- Freeze treatment of invalid partial signatures.
- Freeze treatment of offline participants.
- Decide whether VRF rewards are independent from existing block reward rules.
- Define anti-withholding incentives.
- Define anti-spam economics.

The existing Rabbit block reward split MUST NOT be implicitly reused for VRF
without an explicit protocol decision.

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
- Reward recipients and split.
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
