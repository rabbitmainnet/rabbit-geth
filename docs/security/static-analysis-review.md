# Static analysis and security review

This document records the public static-analysis state of Rabbit Chain.
It deliberately distinguishes source-code fixes from findings that were
accepted or classified as false positives. A passing quality gate is not
presented as proof that the implementation is defect-free.

## Snapshot identity

- Branch: `testnet-release-v1`
- Source commit: `ff849ac0246cdf5abe524ce53a64cc6ecbd76fcf`
- Report generated: `2026-08-25 00:55 UTC`
- Sonar project: `rabbitmainnet_rabbit-geth`
- Sonar data source: public SonarQube Cloud API

## Current public findings

| Quality | Status | Blocker | High | Medium | Low | Info | Total |
|---|---:|---:|---:|---:|---:|---:|---:|
| Security | Accepted | 0 | 8 | 0 | 0 | 0 | 8 |
| Security | False Positive | 0 | 0 | 74 | 26 | 0 | 100 |
| Reliability | Accepted | 0 | 87 | 0 | 0 | 0 | 87 |
| Reliability | False Positive | 1 | 0 | 40 | 33 | 0 | 74 |
| Maintainability | Open | 0 | 147 | 28 | 91 | 59 | 325 |
| Maintainability | False Positive | 0 | 175 | 0 | 0 | 0 | 175 |

The `Accepted` and `False Positive` rows are intentionally shown. Those
findings were reviewed or classified; they were not silently repaired.

## Changes that were actually implemented

- Updated vulnerable Go dependencies and migrated the legacy OpenPGP import.
- `govulncheck ./...` reported zero reachable vulnerabilities.
- Pinned the FreeBSD GitHub Action to a full commit SHA.
- Restricted workflow permissions to the required job.
- Replaced a predictable temporary log filename with `os.CreateTemp`.
- Added a regression test for secure temporary-file creation.
- Changed Docker runtime images to the non-root `geth` user.
- Corrected 34 Rabbit release-script reliability findings in source code.
- Classified test fixtures, benchmark tools, binary fuzz corpora and
  vendored dependency CI separately from Rabbit production code.

## Reviewed findings that were not source-code fixes

- Eight Security High findings concern inherited compatibility-sensitive
  cryptographic or protocol code. Their accepted status does not mean the
  implementation was rewritten.
- Remaining inherited Reliability High findings are concentrated in
  upstream or vendored compatibility code and scripts.
- The `libsecp256k1` scratch-object finding occurs through a negative-test
  path using an intentionally invalid object. If classified as a false
  positive in Sonar, that classification remains visible in the table.
- Medium, Low and Maintainability findings remain recorded rather than
  being closed merely to improve the displayed rating.

## Rabbit consensus scope

The static-analysis hardening did not change:

- LCQ consensus rules;
- RandomX or proof validation;
- block rewards or the 70/30 reward split;
- halving boundaries or terminal subsidy;
- genesis allocation or genesis hash;
- participant selection or mining eligibility.

Mining was not started as part of this work.

## Important limitations

Static analysis cannot prove consensus correctness, economic correctness,
network safety or decentralization. Rabbit Chain also requires reproducible
builds, consensus tests, multi-node restart tests, adversarial tests and
independent review.

Binary fuzz corpus files remain tracked and unmodified. They are excluded
from text analysis because arbitrary fuzz input is not required to be valid
UTF-8. This may still produce a scanner-level encoding warning before
exclusion rules are applied.

## Reproduce the public counts

The findings can be independently queried from the public Sonar API:

```text
https://sonarcloud.io/api/issues/search?componentKeys=rabbitmainnet_rabbit-geth&branch=testnet-release-v1&issueStatuses=OPEN,CONFIRMED,ACCEPTED,FALSE_POSITIVE&ps=500
```

No Sonar result should be interpreted as a substitute for an independent
professional security or consensus audit.
