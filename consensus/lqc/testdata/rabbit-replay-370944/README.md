Historical replay fixture for Rabbit Testnet chain ID 9280.

- Work checkpoint and Registry snapshot: supplied by the operator on 2026-11-04.
- Configuration: supplied alongside that checkpoint.
- Headers 370432..371072: public eth_getBlockByNumber responses collected on
  2026-11-04 from https://rpc-testnet.rabbitchain.org/, batches <=20, concurrency 2.
- Each advertised header hash and the full fixture's parent linkage are checked.
- Replay starts from 370944 and validates 128 transitions through 371072,
  including 371008. Registry roots are checked for every replayed header.
- Requires the published Rabbit 1 GiB RandomX profile. The unmodified upstream
  2 GiB build produces different consensus entropy, even in light mode.

This fixture tests consensus side-state replay. It does not contain EVM state,
wallet keys, or a full database, and does not establish full-node sync success.
