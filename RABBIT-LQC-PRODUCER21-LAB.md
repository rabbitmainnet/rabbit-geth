# Rabbit Chain — Permissionless Producer 21 Lab

This lab creates a new temporary blockchain at
`/tmp/rabbit-permissionless-21nodes`. Twenty addresses form only the initial
genesis set. The `node21` address does not appear in the genesis, receives no
RAB, and connects to only three peers, simulating an external user.

The canonical protocol activates at block 1. The auditor automatically performs:

1. initial convergence of all 21 nodes;
2. confirmation that node21 is not registered and has a zero balance;
3. `REGISTER` with LightHash and a local signature;
4. canonical inclusion and eligibility on 21/21 nodes;
5. production of a block by the new participant;
6. a signed `HEARTBEAT`;
7. a signed `EXIT` followed by 30 blocks without production by the inactive address;
8. a new `REGISTER` without an administrator;
9. new production after returning;
10. final convergence and distribution.

Private keys and the password are not included in the report. The standard
20-node lab keeps activation disabled. The auditor stops processes from that
older lab to avoid excessive memory use, but does not delete its databases.

The test requires at least 20 GiB free both on the WSL virtual disk and on the
C: drive that stores that virtual disk. If the gate fails, no new database is
created.
