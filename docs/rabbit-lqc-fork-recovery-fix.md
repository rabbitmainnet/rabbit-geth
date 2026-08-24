# Rabbit Chain — LQC Fork Recovery Fix

## Lab evidence

During the 20-producer test, seven nodes were shut down at block 191. The 13
active nodes advanced together and confirmed a transaction at block 195. After
returning, each stopped node produced its own branch starting at block 192.

The previous LQC synchronizer requested only the next header by number. Because
the local block 192 belonged to another branch, remote header 193 had no known
ancestor. The logs repeatedly recorded `unknown ancestor`. In addition,
constructing `types.NewBlockWithHeader` discarded bodies, transactions, and
receipts.

## Fix

- the `BlockRangeUpdate` announcement selects the chain with the greater height;
- height ties use the lower hash as a deterministic tie-breaker;
- the target header is requested using its announced hash;
- `downloader.BeaconSync` in full mode finds the common ancestor, downloads
  headers and bodies, and performs the canonical reorganization;
- only one LQC recovery runs at a time;
- `lqcv2.VerifyHeaders` now runs `VerifyHeader` for every header in the batch,
  preventing backfill from bypassing producer validation.

The fix does not change rewards, the committee, vesting, halvings, or genesis
files. The absence of a cryptographic block signature in the active `lqcv2`
engine remains a separate architectural finding and blocks a mainnet release
until it is resolved or the definitive LQC engine is activated through an
audited process.
