# LQC work-ticket laboratory transport

Status: `LAB_TRANSPORT_ONLY`. This layer is deliberately disconnected from the
active Ethereum backend, LQC header, producer selection and mainnet genesis.

## RPC surface

The unregistered `LQCWorkTicketTransportAPI` accepts only an already generated
and signed ticket. It never receives a private key and exposes three laboratory
methods:

- `lqc_submitWorkTicket`
- `lqc_workTicketPoolStatus`
- `lqc_pendingWorkTickets`

The service is not returned by `Ethereum.APIs()`. A later isolated-laboratory
wiring stage must construct it explicitly.

## P2P protocol

The unregistered `lqct/1` protocol uses a handshake bound to protocol version,
network ID, genesis hash and chain ID. It relays at most eight tickets per 8 KiB
message and suppresses already-known tickets.

Argon2id validation is protected before expensive work by two fixed windows:

- 64 novel tickets per peer per 10 seconds;
- 128 novel tickets globally per node per 10 seconds.

The global allowance is twice the frozen 64-ticket block capacity. Duplicate
tickets already in the pool are removed before either validation budget is
charged. Initial pool synchronization is capped at 64 tickets.

## Non-activation guarantees

- `eth/backend.go` does not construct the transport.
- `Ethereum.APIs()` does not register the RPC service.
- `Ethereum.Protocols()` does not register `lqct/1`.
- no work ticket enters an LQC header or producer/committee selection.
- the frozen mainnet genesis remains byte-for-byte unchanged.

The next stage may wire this surface into a separate laboratory process. That
stage must still leave mainnet disabled and must pass live invalid-proof flood,
reconnect, duplicate, propagation and weak-PC resource tests before any engine
integration is considered.
