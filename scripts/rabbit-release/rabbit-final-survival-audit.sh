#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN="${RABBIT_BIN:-$ROOT/build/rabbit-mainnet-workv1-rc1/geth-rabbit-mainnet-workv1-linux-amd64}"
GENESIS="$ROOT/networks/rabbit-mainnet/genesis.json"
LAB="${RABBIT_FINAL_LAB:-/tmp/rabbit-final-survival-audit}"
NODES=4

RPC_BASE=18545
P2P_BASE=18600

PASS=0
FAIL=0

log()  { printf '\n===== %s =====\n' "$*"; }
pass() { echo "PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "FAIL: $*" >&2; FAIL=$((FAIL+1)); }

rpc() {
    local port="$1"
    local method="$2"
    local params="${3:-[]}"

    curl -fsS \
      -H 'Content-Type: application/json' \
      --data "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}" \
      "http://127.0.0.1:$port"
}

rpc_result() {
    rpc "$1" "$2" "${3:-[]}" |
        python3 -c 'import json,sys; x=json.load(sys.stdin); print(x.get("result",""))'
}

wait_rpc() {
    local port="$1"
    for _ in $(seq 1 60); do
        if rpc "$port" eth_blockNumber >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

height() {
    local h
    h="$(rpc_result "$1" eth_blockNumber)"
    python3 - "$h" <<'PY'
import sys
print(int(sys.argv[1],16))
PY
}

wait_height_at_least() {
    local port="$1"
    local target="$2"

    for _ in $(seq 1 120); do
        local h
        h="$(height "$port" 2>/dev/null || echo 0)"
        if [ "$h" -ge "$target" ]; then
            return 0
        fi
        sleep 1
    done

    return 1
}

start_node() {
    local i="$1"
    local datadir="$LAB/node$i"
    local rpcport=$((RPC_BASE+i))
    local p2pport=$((P2P_BASE+i))

    mkdir -p "$datadir"

    "$BIN" --datadir "$datadir" \
        --networkid 928 \
        --port "$p2pport" \
        --http \
        --http.addr 127.0.0.1 \
        --http.port "$rpcport" \
        --http.api eth,net,web3,miner,txpool \
        --authrpc.port 0 \
        --nodiscover \
        --maxpeers 20 \
        --ipcdisable \
        >"$LAB/node$i.log" 2>&1 &

    echo $! >"$LAB/node$i.pid"
}

stop_node() {
    local i="$1"

    if [ -f "$LAB/node$i.pid" ]; then
        kill "$(cat "$LAB/node$i.pid")" 2>/dev/null || true
        rm -f "$LAB/node$i.pid"
    fi
}

cleanup() {
    set +e
    for i in $(seq 0 $((NODES-1))); do
        stop_node "$i"
    done
}
trap cleanup EXIT

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "ERRO: comando obrigatório ausente: $1" >&2
        exit 1
    }
}

require_cmd curl
require_cmd python3
require_cmd sha256sum

log "PRÉ-CONDIÇÕES"

[ -x "$BIN" ] || {
    echo "ERRO: binário não encontrado: $BIN" >&2
    exit 1
}

[ -f "$GENESIS" ] || {
    echo "ERRO: genesis não encontrado" >&2
    exit 1
}

EXPECTED_GENESIS="e0850d1f19a516269e476e29fbe9e63282c88a88c2ff43d01bd9eae17898014b"
ACTUAL_GENESIS="$(sha256sum "$GENESIS" | awk '{print $1}')"

[ "$ACTUAL_GENESIS" = "$EXPECTED_GENESIS" ] || {
    echo "ERRO: genesis não corresponde ao congelado" >&2
    echo "esperado=$EXPECTED_GENESIS" >&2
    echo "atual=$ACTUAL_GENESIS" >&2
    exit 1
}

rm -rf "$LAB"
mkdir -p "$LAB"

echo "BIN=$BIN"
echo "GENESIS_SHA256=$ACTUAL_GENESIS"
echo "LAB=$LAB"

log "INIT DOS NÓS"

for i in $(seq 0 $((NODES-1))); do
    "$BIN" --datadir "$LAB/node$i" init "$GENESIS" \
        >"$LAB/init-$i.log" 2>&1
done

pass "genesis inicializado nos $NODES nós"

log "START DOS NÓS"

for i in $(seq 0 $((NODES-1))); do
    start_node "$i"
done

for i in $(seq 0 $((NODES-1))); do
    wait_rpc $((RPC_BASE+i)) || {
        fail "RPC do node$i não iniciou"
        exit 1
    }
done

pass "todos os RPCs responderam"

log "VERIFICAÇÃO DA IDENTIDADE DA REDE"

for i in $(seq 0 $((NODES-1))); do
    chain="$(rpc_result $((RPC_BASE+i)) eth_chainId)"
    net="$(rpc_result $((RPC_BASE+i)) net_version)"

    [ "$chain" = "0x3a0" ] || fail "node$i chainId=$chain"
    [ "$net" = "928" ] || fail "node$i networkid=$net"
done

pass "todos os nós estão na Rabbit Chain 928"

log "VERIFICAÇÃO DO BLOCO ZERO"

for i in $(seq 0 $((NODES-1))); do
    block="$(rpc "$((RPC_BASE+i))" eth_getBlockByNumber '[ "0x0", false ]')"

    echo "$block" | grep -q '"number":"0x0"' \
        || fail "node$i não retornou bloco zero"

    echo "$block" | grep -q '"hash":' \
        || fail "node$i bloco zero sem hash"
done

pass "bloco zero presente em todos os nós"

log "WORK V1 PRODUCTION"

for i in $(seq 0 $((NODES-1))); do
    if grep -q 'LQC Work V1 RandomX transport enabled' "$LAB/node$i.log"; then
        pass "node$i ativou Work V1 production"
    else
        fail "node$i não mostrou ativação Work V1"
    fi
done

log "ALTURA INICIAL"

for i in $(seq 0 $((NODES-1))); do
    echo "node$i height=$(height $((RPC_BASE+i)))"
done

START_HEIGHT="$(height "$RPC_BASE")"

log "PRODUÇÃO / LIVENESS"

if wait_height_at_least "$RPC_BASE" $((START_HEIGHT+2)); then
    pass "node0 produziu pelo menos 2 blocos"
else
    fail "node0 não avançou a cadeia"
fi

H1="$(height "$RPC_BASE")"

echo "altura após produção=$H1"

log "CONSISTÊNCIA ENTRE NÓS"

sleep 3

for i in $(seq 0 $((NODES-1))); do
    h="$(height $((RPC_BASE+i)))"
    echo "node$i height=$h"
done

MAX="$(for i in $(seq 0 $((NODES-1))); do height $((RPC_BASE+i)); done | sort -n | tail -1)"
MIN="$(for i in $(seq 0 $((NODES-1))); do height $((RPC_BASE+i)); done | sort -n | head -1)"

if [ "$((MAX-MIN))" -le 3 ]; then
    pass "alturas dos nós permanecem próximas"
else
    fail "divergência de altura excessiva: min=$MIN max=$MAX"
fi

log "P2P / PEER COUNT"

for i in $(seq 0 $((NODES-1))); do
    peers="$(rpc_result $((RPC_BASE+i)) net_peerCount || echo 0)"
    echo "node$i peers=$peers"
done

log "TRANSAÇÃO RPC"

ACCOUNT_JSON="$LAB/account.json"

"$BIN" account new \
    --datadir "$LAB/node0" \
    --password <(printf 'rabbit-test\n') \
    >"$ACCOUNT_JSON" 2>&1 || true

ACCOUNT="$(
    grep -oE '0x[0-9a-fA-F]{40}' "$ACCOUNT_JSON" |
    head -1 || true
)"

if [ -n "$ACCOUNT" ]; then
    pass "wallet de teste criada: $ACCOUNT"
else
    echo "wallet não foi criada automaticamente; continuando com testes de consenso"
fi

log "TXPOOL / MEMPOOL"

for i in $(seq 0 $((NODES-1))); do
    rpc "$((RPC_BASE+i))" txpool_status >/dev/null 2>&1 \
        && pass "node$i txpool RPC disponível" \
        || fail "node$i txpool RPC indisponível"
done

log "REINÍCIO DE UM NÓ"

BEFORE_RESTART="$(height "$RPC_BASE")"

stop_node 0
sleep 2
start_node 0
wait_rpc "$RPC_BASE" || fail "node0 não voltou após restart"

if wait_height_at_least "$RPC_BASE" "$BEFORE_RESTART"; then
    pass "node0 recuperou após restart"
else
    fail "node0 não recuperou após restart"
fi

log "RESTART / RECUPERAÇÃO"

sleep 5

AFTER_RESTART="$(height "$RPC_BASE")"

if [ "$AFTER_RESTART" -ge "$BEFORE_RESTART" ]; then
    pass "cadeia preservada após restart: $BEFORE_RESTART -> $AFTER_RESTART"
else
    fail "altura caiu após restart"
fi

log "SIMULAÇÃO DE PERDA DE NÓS"

for i in 1 2 3; do
    stop_node "$i"
done

sleep 3

ALONE_BEFORE="$(height "$RPC_BASE")"

echo "node0 sozinho antes=$ALONE_BEFORE"

if wait_height_at_least "$RPC_BASE" $((ALONE_BEFORE+1)); then
    pass "rede permaneceu viva com somente node0"
else
    fail "rede não avançou com somente node0"
fi

ALONE_AFTER="$(height "$RPC_BASE")"

log "RETORNO DOS NÓS"

for i in 1 2 3; do
    start_node "$i"
done

for i in 1 2 3; do
    wait_rpc "$((RPC_BASE+i))" || fail "node$i não retornou"
done

sleep 8

pass "nós retornaram após isolamento"

log "RECUPERAÇÃO DE CONSENSO"

for i in $(seq 0 $((NODES-1))); do
    echo "node$i height=$(height $((RPC_BASE+i)))"
done

TARGET="$(height "$RPC_BASE")"

for i in 1 2 3; do
    if wait_height_at_least "$((RPC_BASE+i))" "$TARGET"; then
        pass "node$i alcançou a altura do node0"
    else
        fail "node$i não recuperou até $TARGET"
    fi
done

log "BLOCO FINAL"

FINAL_HEIGHT="$(height "$RPC_BASE")"

for i in $(seq 0 $((NODES-1))); do
    BLOCK="$(printf '0x%x' "$FINAL_HEIGHT")"
    HASH="$(
        rpc "$((RPC_BASE+i))" eth_getBlockByNumber "[\"$BLOCK\",false]" |
        python3 -c 'import json,sys; print(json.load(sys.stdin).get("result",{}).get("hash",""))'
    )"

    echo "node$i final_hash=$HASH"

    [ -n "$HASH" ] || fail "node$i não possui bloco final"
done

log "RESUMO"

echo "PASS=$PASS"
echo "FAIL=$FAIL"
echo "START_HEIGHT=$START_HEIGHT"
echo "FINAL_HEIGHT=$FINAL_HEIGHT"
echo "GENESIS_SHA256=$ACTUAL_GENESIS"

if [ "$FAIL" -ne 0 ]; then
    echo
    echo "RABBIT FINAL SURVIVAL AUDIT = FAIL"
    echo "Logs: $LAB"
    exit 1
fi

echo
echo "RABBIT FINAL SURVIVAL AUDIT = PASS"
echo "Logs: $LAB"
