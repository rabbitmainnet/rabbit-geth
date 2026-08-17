#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
OUT="${2:-$ROOT/build/rabbit-mainnet-workv1-rc1}"
RANDOMX_ROOT="${RABBIT_RANDOMX_ROOT:-$HOME/.cache/rabbit-randomx-1g-v1}"
RANDOMX_LIB="$RANDOMX_ROOT/build/librandomx.a"
GENESIS="$ROOT/networks/rabbit-mainnet/genesis.json"
EXPECTED_GENESIS="ee0e6b167e1cd56162b55d385b998b5e75d68370fbf5959717d58f7695194f37"
EXPECTED_RANDOMX_COMMIT="7c761cf007c758056dcb6eb438a32f780f81bdbd"
EXPECTED_RANDOMX_HEADER="13e8432375a31c38fb2ff45b64efa5f55ed56a4b315b12d56b3cca7089cf958c"
EXPECTED_RANDOMX_LIB="24d26890d0a219cff758e29443432bb9e066abaff1b613e0720458b0a8b82b33"
EXPECTED_GO_VERSION="go1.25.13"
SECURITY_BASE_TAG="v1.17.5"
SECURITY_BASE_COMMIT="9621c6ad10934a01b5514886fb6fbd87640b6c05"
TAGS="rabbit_workv1 rabbit_randomx"

GO_BIN="${RABBIT_GO_BIN:-$(command -v go || true)}"
GOFMT_BIN="${RABBIT_GOFMT_BIN:-$(command -v gofmt || true)}"

fail() {
    printf 'ERRO: %s\n' "$*" >&2
    exit 1
}

expect_hash() {
    local expected="$1"
    local file="$2"
    local actual
    [ -f "$file" ] || fail "arquivo ausente: $file"
    actual="$(sha256sum "$file" | awk '{print $1}')"
    [ "$actual" = "$expected" ] || fail \
        "SHA-256 inesperado para $file (esperado $expected, atual $actual)"
}

[ "$(uname -s)" = "Linux" ] || fail "o release Work V1 atual é Linux-only"
[ "$(uname -m)" = "x86_64" ] || fail "o release Work V1 atual é amd64-only"
[ -n "$GO_BIN" ] && [ -x "$GO_BIN" ] || fail "Go não encontrado"
[ -n "$GOFMT_BIN" ] && [ -x "$GOFMT_BIN" ] || fail "gofmt não encontrado"
command -v readelf >/dev/null 2>&1 || fail "readelf não encontrado (instale binutils)"
command -v timeout >/dev/null 2>&1 || fail "timeout não encontrado (instale coreutils)"

GO_VERSION="$("$GO_BIN" version | awk '{print $3}')"
[ "$GO_VERSION" = "$EXPECTED_GO_VERSION" ] || fail \
    "Go inesperado: $GO_VERSION (esperado $EXPECTED_GO_VERSION)"

expect_hash "$EXPECTED_GENESIS" "$GENESIS"
expect_hash "$EXPECTED_RANDOMX_HEADER" "$RANDOMX_ROOT/src/randomx.h"
expect_hash "$EXPECTED_RANDOMX_LIB" "$RANDOMX_LIB"

if [ -d "$RANDOMX_ROOT/.git" ]; then
    RANDOMX_COMMIT="$(git -C "$RANDOMX_ROOT" rev-parse HEAD)"
    [ "$RANDOMX_COMMIT" = "$EXPECTED_RANDOMX_COMMIT" ] || fail \
        "commit RandomX inesperado: $RANDOMX_COMMIT"
else
    echo "RANDOMX_GIT_METADATA=ABSENT_SOURCE_HASHES_AUTHORITATIVE"
fi

mkdir -p "$OUT"
BIN="$OUT/geth-rabbit-mainnet-workv1-linux-amd64"
DEFAULT_BIN="$OUT/geth-default-guard-check"
TEST_LOG="$OUT/testes.log"
SMOKE_DIR="$(mktemp -d /tmp/rabbit-mainnet-workv1-smoke-XXXXXX)"
trap 'rm -rf -- "$SMOKE_DIR"' EXIT

export CGO_ENABLED=1
export CGO_CFLAGS="-O2 -g -D__BLST_PORTABLE__"
export CGO_LDFLAGS="-L$RANDOMX_ROOT/build -lrandomx -Wl,-z,noexecstack"

cd "$ROOT"

{
    echo "RABBIT_WORK_V1_PRODUCTION_RELEASE=START"
    echo "TAGS=$TAGS"
    echo "GENESIS_SHA256=$EXPECTED_GENESIS"
    echo "RANDOMX_COMMIT=$EXPECTED_RANDOMX_COMMIT"
    echo "RANDOMX_LIB_SHA256=$EXPECTED_RANDOMX_LIB"
    echo "GO_VERSION=$EXPECTED_GO_VERSION"
    echo "SECURITY_BASE_TAG=$SECURITY_BASE_TAG"
    echo "SECURITY_BASE_COMMIT=$SECURITY_BASE_COMMIT"

    "$GO_BIN" test -tags "$TAGS" ./consensus/lqc \
        -run '^(TestWorkV1EngineLabRuntimeReconstructsRequestedBranchByHash|TestWorkV1EngineLabAuthorizedFallbackReceivesProducerShare|TestWorkV1EngineLabVerifyHeadersUsesBatchParentRuntime|TestWorkV1EngineLabRelayContextReplaysHeaderV3AfterRestart|TestWorkV1EngineLabShortSeatPoolReservesCommitteeReward)$' \
        -count=1 -v
    "$GO_BIN" test -tags "$TAGS" ./eth \
        -run '^(TestLQCWorkV1ProductionActivationGate|TestLQCWorkV1PoolJournalPersistsPendingAcrossRestart|TestLQCWorkV1EnginePoolProviderLabReadmitsRemovedAfterReorg)$' \
        -count=1 -v
    "$GO_BIN" test ./miner \
        -run '^TestInsertLQCBlockIfParentCurrent' -count=1 -v
    "$GO_BIN" test ./consensus/lqc ./eth ./miner -count=1
    "$GO_BIN" test -tags "$TAGS" ./consensus/lqc ./eth ./miner -count=1
} 2>&1 | tee "$TEST_LOG"

"$GO_BIN" build -tags "$TAGS" -trimpath -o "$BIN" ./cmd/geth
"$GO_BIN" build -trimpath -o "$DEFAULT_BIN" ./cmd/geth

STACK_LINE="$(readelf -W -l "$BIN" | awk '$1 == "GNU_STACK" {print; found=1} END {if (!found) exit 1}')" || \
    fail "segmento GNU_STACK ausente"
printf 'PRODUCTION_GNU_STACK=%s\n' "$STACK_LINE"
printf '%s\n' "$STACK_LINE" | grep -Eq '(^|[[:space:]])RW([[:space:]]|$)' || \
    fail "binário de produção não está com GNU_STACK RW"
if printf '%s\n' "$STACK_LINE" | grep -Eq '(^|[[:space:]])RWE([[:space:]]|$)'; then
    fail "binário de produção possui pilha executável"
fi

mkdir -p "$SMOKE_DIR/production" "$SMOKE_DIR/default"
"$BIN" --datadir "$SMOKE_DIR/production" init "$GENESIS" \
    >"$OUT/init-production.log" 2>&1
"$DEFAULT_BIN" --datadir "$SMOKE_DIR/default" init "$GENESIS" \
    >"$OUT/init-default.log" 2>&1

set +e
timeout --foreground 8 "$BIN" \
    --datadir "$SMOKE_DIR/production" \
    --networkid 928 \
    --port 0 \
    --authrpc.port 0 \
    --nodiscover \
    --maxpeers 0 \
    --ipcdisable \
    >"$OUT/start-production.log" 2>&1
PRODUCTION_STATUS="$?"

timeout --foreground 8 "$DEFAULT_BIN" \
    --datadir "$SMOKE_DIR/default" \
    --networkid 928 \
    --port 0 \
    --authrpc.port 0 \
    --nodiscover \
    --maxpeers 0 \
    --ipcdisable \
    >"$OUT/start-default.log" 2>&1
DEFAULT_STATUS="$?"
set -e

if [ "$PRODUCTION_STATUS" -ne 124 ]; then
    echo "===== START PRODUCTION LOG =====" >&2
    sed -n '1,240p' "$OUT/start-production.log" >&2
    fail "smoke de produção terminou inesperadamente: $PRODUCTION_STATUS"
fi
grep -q 'LQC Work V1 RandomX transport enabled' "$OUT/start-production.log" || fail \
    "transporte Work V1 de produção não foi ativado"

if [ "$DEFAULT_STATUS" -eq 0 ] || [ "$DEFAULT_STATUS" -eq 124 ]; then
    echo "===== START DEFAULT-GUARD LOG =====" >&2
    sed -n '1,240p' "$OUT/start-default.log" >&2
    fail "binário default não recusou o genesis oficial"
fi
grep -q 'requires a production Work V1 build' "$OUT/start-default.log" || fail \
    "motivo de recusa do binário default não foi encontrado"

install -m 0644 "$GENESIS" "$OUT/genesis.json"
(
    cd "$OUT"
    sha256sum \
        geth-rabbit-mainnet-workv1-linux-amd64 \
        genesis.json \
        >SHA256SUMS
)
rm -f -- "$DEFAULT_BIN"

echo "WORK_V1_PRODUCTION_UNIT=PASS"
echo "WORK_V1_BRANCH_RECONSTRUCTION=PASS"
echo "WORK_V1_BATCH_VERIFY_HEADERS=PASS"
echo "WORK_V1_FALLBACK_AUTHOR_REWARD=PASS"
echo "WORK_V1_ZERO_WORK_NO_SUBSIDY=PASS"
echo "WORK_V1_INITIAL_DIFFICULTY_FROM_FROZEN_CONFIG=100000"
echo "WORK_V1_PRODUCTION_TRANSPORT=PASS"
echo "DEFAULT_BINARY_MAINNET_GUARD=PASS"
echo "PRODUCTION_BINARY_GNU_STACK_RW=PASS"
echo "MAINNET_GENESIS_UNCHANGED=YES"
echo "LONG_LIVE_LAB_RERUN=NO"
echo "SOFTWARE_RELEASE_CANDIDATE=PASS"
echo "PUBLIC_MAINNET_LAUNCH=WAITING_FOR_BOOTNODES_RPC_EXPLORER"
echo "RELEASE_DIR=$OUT"
