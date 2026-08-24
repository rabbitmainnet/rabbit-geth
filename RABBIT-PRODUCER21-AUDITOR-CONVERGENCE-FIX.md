# Rabbit Chain — Producer 21 Auditor Convergence Fix

Node21 was registered by 21/21 nodes, produced a block, and the network later
converged with a maximum delay of one block. The old auditor nevertheless
reported `FAIL` when the first candidate block was not confirmed by every node
within 60 seconds.

This fix:

- uses the entire global timeout for confirmation;
- discards a candidate that became orphaned and continues searching;
- requires canonical convergence after `REGISTER`, `HEARTBEAT`, `EXIT`, and re-entry;
- records every gate in the final report;
- does not modify any Go file or consensus rule.
