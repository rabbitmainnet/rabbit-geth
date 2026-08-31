# Public RPC, explorer, and bootnode services

This is the deployment contract for official and community public services.
Exact host-specific commands and secrets belong in private operations records,
not in the public repository.

## Bootnodes

Run at least two independently hosted discovery nodes with persistent,
server-generated node keys. Publish both discovery-v4 enodes and discovery-v5
ENRs when supported. Verify advertised IP addresses and both TCP and UDP P2P
reachability from outside the hosting network.

Bootnodes should not host wallet keys, a faucet, or public administrative APIs.
Their failure must not stop already connected peers.

## Public HTTPS RPC

Expose only the namespaces required by public applications. A conservative
starting allowlist is `eth`, `net`, and `web3`, with any additional method
reviewed individually. Disable account management, signing, admin, debug,
personal, miner control, engine, and unrestricted tracing methods.

Required controls include TLS, origin and host validation, per-IP and global
rate limits, request-body limits, batch limits, timeouts, connection limits,
upstream health checks, metrics, and abuse logging that avoids collecting
secrets.

The official live address is `https://rpc-testnet.rabbitchain.org`.
Community RPC endpoints are equally valid as access providers but must publish
their own policy and operator identity.

## Public WebSocket RPC

WebSocket access requires separate connection, subscription, message-size, and
idle-time limits. It should use its own proxy policy and capacity budget rather
than inheriting HTTP defaults accidentally.

The official live address is `wss://rpc-testnet.rabbitchain.org/ws`.

## Explorer

The explorer is an untrusted index of canonical node data. Pin the Blockscout
backend and frontend image digests, isolate the database, use least-privilege
credentials, back up the database, and monitor indexing lag against at least
two independently queried nodes.

Wallet connection and “Add Rabbit Testnet” are convenience features. They must
request Chain ID 9280 and the canonical testnet RPC, show the exact network to
the user, and never request a seed phrase or raw private key.

The official live address is
`https://explorer-testnet.rabbitchain.org`.

## Faucet and status page

The faucet is an application, not a protocol component. Apply wallet/IP abuse
controls, transaction limits, a documented distribution policy, isolated
signing, and a strictly limited hot-wallet balance.

The status page should be served independently of the primary RPC path where
possible. It should report RPC, WS, explorer, bootnode reachability, indexing
lag, peer counts, chain head, and incidents without claiming that service
health determines consensus health.

Current availability:

- Faucet: planned, no official endpoint published
- Website status: `https://rabbitchain.org/status`

## Launch evidence

Before announcing activation, publish:

- genesis SHA-256 and release commit;
- binary and archive SHA-256 manifests;
- bootnode enodes/ENRs and geographic or provider diversity summary;
- public endpoint URLs and TLS status;
- observed chain head/hash from independent nodes;
- explorer indexing lag;
- allowed RPC namespaces and rate-limit policy;
- known limitations and incident contact.
