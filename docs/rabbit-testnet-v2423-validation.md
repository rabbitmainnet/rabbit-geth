# Rabbit Core Testnet v2.4.23 validation

Network: Rabbit Chain Testnet, chain ID 9280.

- GitHub Actions run 42214151736 passed for Linux amd64, Windows amd64,
  macOS amd64 and macOS arm64.
- Package metadata records source commit
  07b8072e575c6b9ff54c3a19fec50e116d93e047 (PR test merge).
- Windows package internal verification passed.
- Windows offline startup/restart responded in approximately 56 seconds.
- Graceful shutdown completed and persisted the journal.
- Bootstrap block: 385509.
- Bootstrap block hash:
  0xe2a3cb8f849c8d6fa6b0d28bd385e800b04cf74175db6b5043a139f6410194a7.
- Archive SHA256:
  4b51521255b44226c13900160f8009dc740645a49e252d60f821497f5ad0ff46.
- Eight parts, 8037733469 compressed bytes, 343 database files,
  11718696580 file-content bytes.
- Full assembled snapshot extraction and atomic installation passed locally.
- Windows abrupt-process interruption test: interrupted during synchronization
  at block 385574, restarted, and observed block 385600 within 34 seconds.
  Block 385598 matched the public RPC hash.
- Tests used disposable copies. No original wallet or original database reset.
- Abrupt process termination does not simulate all physical power-loss or
  storage-device failure modes.
- Public bootstrap download verification remains pending publication.
