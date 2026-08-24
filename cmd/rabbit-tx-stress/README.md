# Rabbit Chain Transaction Stress Auditor

Auditor exclusively for the `/tmp/rabbit-20nodes` lab. It locally signs
EIP-1559 transactions using node 20's liquid test account and submits five
batches of 25 transfers by default.

The report validates:

- 125 consecutive nonces;
- receipts and status of every transaction;
- value, fee, tip, and burned base fee;
- sender, recipient, and producer balance deltas;
- canonical blocks across all 20 nodes;
- empty transaction pools on every node at the end;
- rejection of a duplicate transaction, stale nonce, insufficient balance, and
  incorrect chain ID.

The program does not modify genesis or use locked rewards. The liquid balance
used is the temporary balance present only in the lab's runtime genesis.
