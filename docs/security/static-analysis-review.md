# Static analysis and security review

This document records the public static-analysis state of Rabbit Chain.
No finding in this snapshot is classified as `Accepted` or `False Positive`.
All findings remain visible for independent examination.

## Snapshot identity

- Branch: `testnet-release-v1`
- Source baseline: `8391068a6b76162deab7bca145d8f8b527383715`
- Report generated: `2026-08-25 01:21 UTC`
- Sonar project: `rabbitmainnet_rabbit-geth`
- Classified or hidden findings: `0`

## Complete open findings

| Quality | Status | Blocker | High | Medium | Low | Info | Total |
|---|---:|---:|---:|---:|---:|---:|---:|
| Security | Open | 0 | 8 | 74 | 26 | 0 | 108 |
| Reliability | Open | 1 | 87 | 40 | 33 | 0 | 161 |
| Maintainability | Open | 5 | 773 | 447 | 1045 | 170 | 2440 |

A Sonar issue may affect more than one software quality. These totals must not
be added together as though they represented unique issues.

## Changes actually implemented

- Vulnerable Go dependencies were updated.
- Legacy OpenPGP usage was migrated to a maintained implementation.
- `govulncheck ./...` reported zero reachable vulnerabilities.
- Workflow permissions and third-party Action pinning were hardened.
- Predictable temporary-file creation was replaced with `os.CreateTemp`.
- A temporary-file security regression test was added.
- Docker runtime images now use the non-root `geth` user.
- Thirty-four Rabbit release-script reliability findings were fixed in code.
- Test fixtures, benchmarks and binary fuzz inputs remain preserved.

## Open findings policy

All findings shown above remain open. Vendored, inherited, test-only and
compatibility-sensitive findings are not hidden merely to improve a rating.
An open static-analysis finding is not automatically a confirmed defect.
Future corrections or dispositions require public technical justification,
tests where applicable and a versioned commit.

## Consensus integrity

These analysis changes did not alter LCQ, RandomX, participant selection,
mining eligibility, rewards, the 70/30 split, halving, terminal subsidy or
genesis. Mining was not started.

## Limitations

Static analysis cannot prove consensus correctness, network safety,
decentralization or economic correctness. Reproducible builds, protocol
invariants, multi-node recovery tests, adversarial tests and independent
review remain necessary.

The binary transaction-fetcher fuzz corpus intentionally contains arbitrary
non-UTF-8 input. It remains tracked and unmodified, although Sonar may show an
encoding warning before applying exclusions.

## Public reproduction

```text
https://sonarcloud.io/api/issues/search?componentKeys=rabbitmainnet_rabbit-geth&branch=testnet-release-v1&issueStatuses=OPEN,CONFIRMED
```

No Sonar rating substitutes for an independent professional security or
consensus audit.
