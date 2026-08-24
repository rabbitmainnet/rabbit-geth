# Public testnet launch and decentralization plan

Rabbit Testnet activation is a bootstrap event, not the creation of a permanent
central operator.

## Phase 0 — verified software

- Freeze and publish the genesis hash, source commit, dependency audit, tests,
  native builds, and checksums.
- Keep preview bootnode files empty until real server identities are validated.
- Document consensus and reward rules from code and genesis.

## Phase 1 — initial public infrastructure

- Deploy independent bootnodes, full nodes, RPC, WS, explorer, faucet, and
  monitoring.
- Generate production node keys on their owning servers.
- Validate external P2P reachability, cross-node head agreement, TLS, RPC
  restrictions, rate limits, and explorer indexing.
- Publish an activation release and machine-readable network manifest.

## Phase 2 — community replication

- Publish reproducible guides for nodes, miners, bootnodes, RPC, archive nodes,
  explorers, indexers, and mirrors.
- Accept and document independently operated bootnodes and endpoints.
- Provide custom-peer and custom-RPC instructions so users are not locked to
  project infrastructure.
- Encourage independent source builds and checksum verification.

## Phase 3 — resilience exercises

- Test loss of each official bootnode.
- Test loss of the official RPC and explorer.
- Confirm connected nodes continue producing, validating, and relaying blocks.
- Confirm a new node can join through documented community discovery paths.
- Publish results and remediate every centralized dependency found.

## Mainnet readiness

Testnet success is not automatically mainnet readiness. Mainnet requires a
separate frozen genesis, economic review, adversarial testing, key ceremony,
independent operators, reproducible builds, security review, incident process,
and a clearly published upgrade policy. Testnet parameters must not be assumed
to be final mainnet parameters.
