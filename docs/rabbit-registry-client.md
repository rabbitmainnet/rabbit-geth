# Rabbit Chain Permissionless Registry Client

`rabbit-registry` creates and submits LQC registry operations without exposing
the private key to the RPC endpoint. The LightHash proof and signature are
calculated on the participant's computer.

## Key sources

Use exactly one of these options:

- `--keystore FILE --password-file FILE`: geth JSON keystore;
- `--key FILE`: raw ECDSA key containing 64 hexadecimal characters.

Password files and raw keys must be stored on a Linux filesystem with `0600`
permissions. Passwords and keys must never be entered as command-line arguments
or sent to the RPC endpoint.

## Operations

```bash
build/bin/rabbit-registry \
  --rpc /path/to/geth.ipc \
  --keystore /path/to/UTC--... \
  --password-file /path/to/password.txt \
  --action register
```

After registration, use `--action heartbeat` to renew activity or
`--action exit` to leave. `--dry-run` signs and validates without submitting.
The client queries `lqc_registryParameters` and `lqc_registryParticipant`,
rejects reads from different heads, determines the correct sequence, and limits
validity to the maximum accepted by consensus.

The RPC endpoint receives only the already-signed public operation: version,
action, address, sequence, validity, proof nonce, and signature.
