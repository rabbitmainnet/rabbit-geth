# Rabbit Chain — Permissionless Registry Audit

This audit verifies whether independent nodes starting from the same canonical
head calculate exactly the same participant set.

It covers three mandatory properties:

1. two producers with different local memory states must derive the same queue;
2. a producer outside the genesis must have a deterministic admission path;
3. a new node must reconstruct the registry solely from canonical data.

The 20-producer lab uses `registryMode=bootstrap`. Therefore, the successful
convergence, transaction, resilience, and reward tests do not demonstrate
permissionless admission.

The mainnet genesis uses `registryMode=native`. In the audited implementation,
native mode queries `runtimeRegistry`, which exists only in each process's
memory. Until it is replaced with deterministic data derived from the chain,
the public launch must remain blocked.
