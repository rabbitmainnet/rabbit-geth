# Rabbit Chain Reward Auditor

Independent, read-only auditor for Rabbit Chain. The program connects to a node
through IPC, HTTP, or WebSocket, traverses the canonical chain, and generates
Markdown, JSON, and CSV reports. It does not modify `consensus/lqc`, submit
transactions, or stop mining.

Version 1.4 reconstructs the permissionless registry from canonical headers.
Therefore, a participant is tracked from the block containing its `REGISTER`,
even before producing its first block or receiving its first committee share.
Every header is also checked for its `registryRoot`, operations, producer, and
snapshot continuity.

## Verified behavior

- producer and position in each block's deterministic queue;
- REGISTER, HEARTBEAT, and EXIT reconstructed by block hash;
- committee reconstructed from the canonical queue and the fixed or dynamic
  genesis size;
- 70/30 split and lossless remainder distribution down to every wei;
- reward for each era and exact halving boundaries;
- expected and observed issuance;
- immediate liquid credit for the producer and committee;
- legacy vesting storage, which must remain empty and unchanged;
- transaction effects, isolated with `debug_traceBlockByNumber` when necessary;
- differences by block, wallet, and role (producer or committee).

## 20-producer lab

With the lab running, execute this command from the repository root:

```bash
./scripts/rabbit-devnet/run-reward-audit.sh
```

The script compiles only `build/bin/rabbit-audit` and creates a new directory
under `audit-reports/`. In addition to the three reports, it creates
`binarios.txt`, containing the path, size, hash, command line, and metadata for
the running `geth` process. It also creates `execucao.txt`, even when the
audit stops before producing the reports. Exit code `2` means that the auditor
found a reward discrepancy or a critical architectural blocker; the reports
are still produced normally.

The 20-producer lab starts only node 1 with `--gcmode archive` and
`--history.state 0`. This is the audit node: it retains the history required
to compare every block from genesis. The other 19 nodes remain in full mode.
When the lab restarts, the previous directory is moved to a backup instead of
being deleted.

## Direct usage

```bash
go build -o build/bin/rabbit-audit ./cmd/rabbit-audit

build/bin/rabbit-audit \
  --rpc /tmp/rabbit-20nodes/node1/geth.ipc \
  --genesis /tmp/rabbit-20nodes/genesis-runtime.json \
  --from 1 \
  --to 0 \
  --summary audit-reports/summary.md \
  --json audit-reports/report.json \
  --csv audit-reports/blocks.csv
```

`--to 0` fixes the audit at the height observed when the program starts, even
if new blocks continue to be produced while it runs.

## Source of truth

For heights after `registryProtocolBlock` activation, the static
`bootstrapParticipants` list is used only to create the initial snapshot.
Afterward, every participant, queue, and committee is derived from the
versioned header envelopes. The auditor rejects any continuity break or invalid
`registryRoot`.
