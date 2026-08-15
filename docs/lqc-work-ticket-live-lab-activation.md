# LQC work-ticket live laboratory activation

The transport is opt-in through `--lqc.worktickets.labtransport`. The default is
disabled and the flag is non-persistent: it cannot be enabled by a TOML file.

The backend refuses activation when chain ID 928 is combined with the frozen
`RABBIT_MAINNET_GENESIS_V1` marker. This check is exercised both by unit tests
and by starting a disposable node against the real frozen mainnet genesis and
requiring a clear refusal.

The upstream `TestMultiProtoSynchronisation68Full` test has a hard three-second
deadline. If and only if it is the sole downloader failure and reports that
exact timeout, the validator requires five consecutive isolated passes. Every
other downloader failure remains blocking.

With an isolated laboratory genesis, the backend registers the RPC service and
the `lqct/1` P2P protocol. A three-node live test creates fresh databases,
connects a full mesh, generates one ephemeral secp256k1 identity, computes its
portable Argon2id proof locally, submits the signed ticket to node 1 and requires
the same hash in exactly one pool entry on all three nodes.

No producer is started and no block is created. Work tickets still do not enter
headers, producer/fallback/committee selection, rewards or canonical snapshots
in the active engine. Mainnet remains blocked pending controlled engine
integration and adversarial live tests.
