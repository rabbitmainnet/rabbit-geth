# Rabbit Core native release

The manual `Rabbit Core native preview` workflow compiles and tests Rabbit Core,
the Rabbit node and the Work V1 miner on native Linux amd64, Windows amd64,
macOS amd64 and macOS arm64 runners.

RandomX is checked out at commit
`7c761cf007c758056dcb6eb438a32f780f81bdbd`, then the Rabbit 1 GiB profile is
applied before compilation. The workflow never follows a moving RandomX tag.

Preview packages deliberately contain an empty `bootnodes.txt`. Rabbit Core
therefore validates with `--check`, but refuses to start mining. Do not publish
preview artifacts as an official release.

Before the final public release:

1. Deploy the preserved bootnode identity on a stable public server.
2. Validate external P2P connectivity.
3. Place the validated IP-based enode in `bootnodes.txt`.
4. Run the multi-platform preview workflow again.
5. Run the final launch rehearsal from fresh data directories.
6. Sign and publish the final checksums.
