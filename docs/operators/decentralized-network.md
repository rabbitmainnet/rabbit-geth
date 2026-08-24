# Decentralized network operation

Rabbit Chain is intended to outlive its original infrastructure. Official
services provide the first discovery and public-access paths; they are not
trusted consensus components.

## What anyone may operate

Without project permission, an independent operator may run:

- a full node or archive node;
- a producer and Rabbit Work V1 miner under the protocol's admission rules;
- a discovery bootnode;
- an HTTPS or WebSocket RPC service;
- a Blockscout or other compatible explorer;
- an indexer, monitoring service, faucet, snapshot mirror, or release mirror.

Third-party services should clearly identify themselves as community operated.
Users must be able to verify their results against an independently operated
full node.

## Trust boundaries

| Component | Consensus authority | Purpose |
| --- | --- | --- |
| Full node | Verifies locally | Validates blocks and state |
| Producer/miner | None beyond valid blocks | Proposes protocol-valid blocks/work |
| Bootnode | None | Introduces peers |
| RPC/WS endpoint | None | Provides remote node access |
| Explorer/indexer | None | Presents indexed chain data |
| Faucet | None | Distributes test coins under its own policy |
| Website/status page | None | Publishes information |

A bootnode cannot authorize blocks. An RPC can omit or misreport data but
cannot make an invalid block valid. An explorer is a view of the chain, not the
chain itself.

## Bootstrap phase

At public testnet activation, the project will publish multiple bootnode
identities and public endpoints. Persistent node keys must be generated on the
servers that will own them; laboratory node keys must never be reused.

The activation gate should require:

1. identical genesis and release checksums on every host;
2. independent bootnodes on different hosts or providers;
3. external TCP/UDP P2P reachability tests;
4. synchronized full nodes with matching canonical heads;
5. TLS-enabled RPC and WebSocket endpoints with restricted namespaces;
6. an explorer checked against more than one node;
7. documented backup, monitoring, rate-limit, and incident procedures;
8. a public activation manifest containing enodes, ENRs, endpoints, versions,
   hashes, and the activation time.

## Reducing project dependency

After activation, the project should accept community bootnode submissions,
publish reproducible operator instructions, maintain machine-readable network
metadata, and document how clients can use custom peers and endpoints. The
network should be tested periodically with individual official services
disabled.

The target resilience property is straightforward: if the official website,
RPC, explorer, and bootnodes disappear, already connected independent nodes
must continue validating and relaying the canonical chain. New nodes must have
documented alternative discovery paths supplied by the community.

## Operator security baseline

- Never expose `admin`, `debug`, `personal`, IPC, or authenticated engine APIs
  to the public internet.
- Put public RPC behind TLS, request-size limits, method allowlists, rate
  limits, connection limits, and monitoring.
- Keep producer keys separate from web-facing hosts.
- Run bootnodes, RPC, explorer, database, and monitoring with separate trust
  boundaries where practical.
- Pin container images and source revisions; do not deploy mutable `latest`
  tags in production.
- Back up node keys and databases according to their role, but never publish a
  private key or wallet secret.
- Publish security contact and incident status information.

See `docs/operators/public-services.md` for service-specific guidance.
