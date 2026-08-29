# Rabbit Work V1 — Production Profile

The Rabbit mainnet Linux/AMD64 release must be built with:

```text
rabbit_workv1 rabbit_randomx
```

The `rabbit_workv1_engine_lab` build remains restricted to an isolated
genesis. The default binary rejects the official
`RABBIT_MAINNET_GENESIS_V1` genesis, preventing an operator from accidentally
creating an incompatible chain without Work V1.

For the official genesis, the production build automatically enables the
`lqcw/1` transport; the `--lqc.worktickets.labtransport` flag remains
prohibited.

## Frozen rules

- initial Work difficulty: genesis `lqc.proofDifficulty` (`100000`);
- canonical retarget delayed by epoch parity;
- admission eligibility: historical snapshot of the challenge block;
- admission: RandomX work remains permissionless, but each wallet may hold at
  most one canonical WorkSeat in an epoch;
- selection: unique-wallet seats that remain eligible in the current canonical
  registry, each with equal weight;
- reward: 70% to the actual authorized author and 30% divided among unique
  committee wallets;
- an authorized fallback receives the producer share when it actually produces;
- with no eligible WorkSeats, the registry preserves liveness and the base
  subsidy is zero;
- runtime reconstruction and anchors follow the branch's own `(hash, number)`;
- the pending pool is persisted, and tickets removed by a reorganization are
  readmitted only while still valid.

## Official build

Use `scripts/rabbit-release/build-rabbit-mainnet-workv1.sh`. The script:

- verifies the frozen genesis;
- verifies the pinned commit and RandomX library;
- requires Linux/AMD64 and Go 1.24.0;
- runs default and production tests;
- creates a binary with `-trimpath`;
- enforces `-Wl,-z,noexecstack` and rejects `GNU_STACK RWE`;
- initializes disposable data directories with the official genesis;
- proves that the production binary enables Work V1;
- proves that the default binary rejects mainnet;
- writes `SHA256SUMS`.

Never use `geth --mainnet`: that flag belongs to the upstream Ethereum
mainnet. Rabbit uses `geth init networks/rabbit-mainnet/genesis.json` and
`--networkid 928`.

## Rabbit Testnet checkpoint

The approved pre-server Rabbit Testnet checkpoint is:

```text
SOURCE_COMMIT=e9875409dcf27e497812965296cece5d2e0f267a
CHAIN_ID=9280
GENESIS_SHA256=8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7
PACKAGE_SHA256=adf92851c22197a969216432cb53ef8ea629a91dfabc6a3e497f667537affc5d
MULTINODE_REPORT_SHA256=82fccde62a9352f602707b3dd5f25d3e3ebf0bdaa187b5ce250a3fae1c51a3ec
```

The clean-clone gate covers unique-wallet WorkSeats, concurrency, epoch
boundaries, restart/reorg reconstruction, RandomX and the 70/30 reward rule.
The separate live operational gate covers fresh three-node initialization,
full-mesh synchronization, block production, node restart/catch-up and complete
network shutdown/recovery.

The release package remains traceable to `SOURCE_COMMIT`; a later
documentation-only commit does not change its binaries or genesis.

The build completes the software release candidate. Public launch still
requires real bootnodes and ENRs, independent servers, RPC, an archive node, and
an explorer. No lab endpoint or node key may be reused.
