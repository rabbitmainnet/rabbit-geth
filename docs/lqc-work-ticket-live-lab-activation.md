# LQC work-ticket live laboratory activation

This document records both the historical isolated transport laboratory and the
current production activation rules.

## Historical isolated laboratory

The laboratory-only transport is opt-in through
`--lqc.worktickets.labtransport`. The flag is non-persistent and cannot be
enabled by TOML. It remains forbidden on the frozen Rabbit mainnet and testnet
geneses.

With an isolated laboratory genesis, the backend registers the RPC service and
the `lqct/1` P2P protocol. A three-node live test creates fresh databases,
connects a full mesh, generates one ephemeral secp256k1 identity, computes its
portable Argon2id proof locally, submits the signed ticket to node 1 and requires
the same hash in exactly one pool entry on all three nodes.

That historical relay-only laboratory did not start a producer or create
blocks. Its old result must not be treated as the production launch gate.

## Production Work V1

Builds compiled with `rabbit_workv1 rabbit_randomx` automatically activate the
`lqcw/1` production transport on the frozen Rabbit public-network geneses. The
laboratory flag must not be supplied.

Production Work V1 integrates signed RandomX tickets with Header V3, canonical
snapshots, unique-wallet WorkSeats, producer/fallback/committee selection and
the 70/30 producer/committee reward rule.

The canonical invariant is one WorkSeat per wallet per epoch. Repeated tickets
from one wallet cannot create additional consensus weight. This invariant is
enforced at pool, relay, header, snapshot, selection and reward boundaries.

## Rabbit Testnet pre-server gate

The testnet checkpoint at commit
`e9875409dcf27e497812965296cece5d2e0f267a` passed:

- clean-clone default and production test suites;
- real RandomX tests;
- unique-wallet concurrency, epoch, restart and reorg tests;
- unique producer and committee reward tests for the 70/30 rule;
- a fresh three-node operational laboratory on chain ID 9280;
- full-mesh synchronization and canonical block agreement;
- single-node restart and catch-up;
