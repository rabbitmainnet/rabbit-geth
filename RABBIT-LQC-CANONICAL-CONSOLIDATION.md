# Rabbit Chain — `consensus/lqc` Canonical Consolidation

Version: `1.0.2`

This package:

- activates `consensus/lqc` for every genesis containing `config.lqc`;
- retains `consensus/lqcv2` only as an inactive legacy implementation;
- fixes local resolution to use the queue for the next block;
- correctly validates parents included in the same synchronization batch;
- preserves the producer's real timestamp after the minimum slot boundary;
- prevents a zero-timestamp genesis from expiring every fallback window;
- reads `eth.blockNumber` as the native decimal number returned by the console;
- rejects invalid RPC responses instead of accepting them as checkpoint hashes;
- declares the lab ready only after validating peers, heights, a common hash, and diversity;
- includes an independent verifier that audits the current lab without restarting it;
- preserves the previously approved fork recovery behavior;
- makes auditors use the configured `fallbackCount` and `committeeSize`;
- keeps the first halving exactly at block `8,409,600`;
- aligns `RabbitChainConfig`, `RabbitDevnetChainConfig`, and the Rabbit mainnet genesis with that boundary.

Do not reuse a database produced with `lqcv2` to validate this version. After
local validation passes, start a new clean lab with the 20-producer script.
