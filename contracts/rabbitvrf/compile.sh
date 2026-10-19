#!/usr/bin/env bash

set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$ROOT/contracts/rabbitvrf/RabbitVRFCoordinatorV1.sol"
OUT="$ROOT/contracts/rabbitvrf/build"

SOLC_IMAGE="ethereum/solc@sha256:b116bf835554d40c501feab0b2c943a8c5eec003b804bcc5b326b85c93da00c2"

rm -rf "$OUT"
mkdir -p "$OUT"

docker run --rm \
  -v "$ROOT:/src:ro" \
  -v "$OUT:/out" \
  "$SOLC_IMAGE" \
  --evm-version london \
  --optimize \
  --optimize-runs 200 \
  --metadata-hash none \
  --no-cbor-metadata \
  --abi \
  --bin-runtime \
  -o /out \
  /src/contracts/rabbitvrf/RabbitVRFCoordinatorV1.sol

status=$?

if [ "$status" -ne 0 ]; then
  echo "ERROR: solc failed with status $status" >&2
  exit "$status"
fi

echo "SOLC_IMAGE=$SOLC_IMAGE"
echo "EVM_VERSION=london"
echo "OPTIMIZER=enabled"
echo "OPTIMIZER_RUNS=200"
