#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="${ROOT:-$HOME/projects/rabbit-geth}"
G="${G:-$ROOT/build/rabbit-mainnet-workv1-rc1/geth-rabbit-mainnet-workv1-linux-amd64}"
GEN="${GEN:-$ROOT/build/rabbit-mainnet-workv1-rc1/genesis.json}"
LAB="${LAB:-/tmp/rabbit-real-lab-v2}"
PW="${PW:-/tmp/rabbit-bootstrap-password}"

N=20
P=8
P2P=31600
RPC=19600
AUTH=29600
PASS=0
FAIL=0

declare -a PIDS=()
declare -a ADDR=()
declare -A PID RPCURL ENODE

ok(){ echo "PASS  $*"; PASS=$((PASS+1)); }
bad(){ echo "FAIL  $*"; FAIL=$((FAIL+1)); }
cleanup(){
  echo "=== FINAL PROCESS STATE ==="
  for i in $(seq 1 "$N"); do
    if [[ -n "${PID[$i]:-}" ]] && kill -0 "${PID[$i]}" 2>/dev/null; then
      echo "ALIVE n$i pid=${PID[$i]}"
    else
      echo "DEAD n$i"
    fi
  done
  for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null || true; done
}
trap cleanup EXIT INT TERM

call(){
  local i="$1" m="$2" p="${3:-[]}"
  curl -fsS --max-time 4 -H 'Content-Type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$m\",\"params\":$p}" \
    "${RPCURL[$i]}"
}
height(){
  call "$1" eth_blockNumber |
    python3 -c 'import json,sys;print(int(json.load(sys.stdin)["result"],16))'
}
peers(){
  call "$1" net_peerCount |
    python3 -c 'import json,sys;print(int(json.load(sys.stdin)["result"],16))'
}
blockhash(){
  call "$1" eth_getBlockByNumber '["latest",false]' |
    python3 -c 'import json,sys;print(json.load(sys.stdin)["result"]["hash"])'
}

[[ -x "$G" ]] || { echo "RC1 AUSENTE"; exit 1; }
[[ -f "$GEN" ]] || { echo "GENESIS AUSENTE"; exit 1; }
[[ -f "$PW" ]] || { printf '%s\n' rabbit-lab-2026 > "$PW"; chmod 600 "$PW"; }

rm -rf "$LAB"
mkdir -p "$LAB/keys" "$LAB/nodes"

echo "RABBIT REAL LAB V6 — 20 NÓS / P2P REAL / LQC V4"

# 1. 20 identities
for i in $(seq 1 "$N"); do
  mkdir -p "$LAB/keys/n$i"
  "$G" account new --datadir "$LAB/keys/n$i" --password "$PW" >/dev/null 2>&1
  a=$("$G" account list --datadir "$LAB/keys/n$i" 2>/dev/null |
      grep -oiE '[0-9a-f]{40}' | head -1)
  ADDR[$i]="0x$a"
done

# 2. Lab genesis: first 8 are real bootstrap producers, last 12 are Sybil identities.
python3 - "$GEN" "$LAB/genesis.json" "${ADDR[@]}" <<'PY'
import json,sys
src,dst,*a=sys.argv[1:]
with open(src) as f:g=json.load(f)
g["config"]["lqc"]["bootstrapParticipants"]=a[:8]
g.setdefault("alloc",{})
for x in a[:8]:
    g["alloc"][x]={"balance":"1000000000000000000000"}
with open(dst,"w") as f:
    json.dump(g,f,indent=2); f.write("\n")
PY
ok "genesis: 8 bootstrap + 12 Sybil"

# 3. Init
for i in $(seq 1 "$N"); do
  mkdir -p "$LAB/nodes/n$i"
  cp -a "$LAB/keys/n$i/keystore" "$LAB/nodes/n$i/"
  "$G" init --datadir "$LAB/nodes/n$i" "$LAB/genesis.json" >/dev/null 2>&1
  RPCURL[$i]="http://127.0.0.1:$((RPC+i))"
done

# 4. Start 20 real processes.
for i in $(seq 1 "$N"); do
  export RABBIT_LQC_COINBASE="${ADDR[$i]}"
  args=(
    --datadir "$LAB/nodes/n$i"
    --networkid 928
    --port $((P2P+i))
    --nat none
    --nodiscover
    --http
    --http.addr 127.0.0.1
    --http.port $((RPC+i))
    --http.api eth,net,web3,admin,personal,txpool
    --http.vhosts '*'
    --authrpc.port $((AUTH+i))
    --cache 96
    --verbosity 2
  )
  if (( i <= P )); then
    args+=(
      --mine
      --miner.etherbase "${ADDR[$i]}"
      --password "$PW"
    )
  fi
  "$G" "${args[@]}" >"$LAB/nodes/n$i.log" 2>&1 &
  PID[$i]=$!
  PIDS+=("${PID[$i]}")
done

# 5. RPC readiness.
for i in $(seq 1 "$N"); do
  good=0
  for _ in $(seq 1 180); do
    if call "$i" eth_blockNumber >/dev/null 2>&1; then good=1; break; fi
    sleep .25
  done
  if (( ! good )); then
    bad "RPC n$i"
    echo "=== DIAGNOSTIC n$i ==="
    tail -30 "$LAB/nodes/n$i.log" 2>/dev/null || true
    echo "=== PORT $((RPC+i)) ==="
    ss -ltnp 2>/dev/null | grep ":$((RPC+i)) " || true
    exit 2
  fi
done
(( FAIL==0 )) && ok "20 processos Rabbit reais ativos" || exit 2

# 6. Real P2P.
for i in $(seq 1 "$N"); do
  ENODE[$i]=$(call "$i" admin_nodeInfo |
    python3 -c 'import json,sys;print(json.load(sys.stdin)["result"]["enode"])')
done

for round in 1 2 3; do
  for i in $(seq 1 "$N"); do
    for j in $(seq 1 "$N"); do
      ((i==j)) && continue
      call "$i" admin_addPeer "[\"${ENODE[$j]}\"]" >/dev/null 2>&1 || true
    done
  done
  sleep 5
done

echo "=== P2P SNAPSHOT ==="
for i in $(seq 1 "$N"); do
  if ! kill -0 "${PID[$i]}" 2>/dev/null; then
    echo "DEAD n$i before P2P"
  fi
done

min=999
sum=0
for i in $(seq 1 "$N"); do
  x=$(peers "$i" 2>/dev/null || echo 0)
  echo "P2P n$i peers=$x"
  (( x < min )) && min=$x
  sum=$((sum+x))
done
avg=$((sum/N))

(( min >= 2 && avg >= 6 )) \
  && ok "P2P distribuído min=$min avg=$avg" \
  || bad "P2P distribuído min=$min avg=$avg"

# 7. Bootstrap + continuous production.
head=0
for _ in $(seq 1 55); do
  head=$(height 1 2>/dev/null || echo 0)
  ((head>=14)) && break
  sleep 1
done

((head>=2)) && ok "bloco 1 saiu do genesis head=$head" || bad "bloco 1"
((head>=14)) && ok "produção contínua atravessou V3 head=$head" || bad "produção contínua não atravessou V3"

# 8. Propagation: wait for all nodes to approach the producer head.
prop=0
for _ in $(seq 1 35); do
  head=$(height 1 2>/dev/null || echo "$head")
  lag=0
  for i in $(seq 1 "$N"); do
    h=$(height "$i" 2>/dev/null || echo 0)
    ((h < head-2)) && lag=1
  done
  if ((lag==0)); then prop=1; break; fi
  sleep 1
done

if (( prop )); then
  ok "propagação P2P convergiu head=$head"
else
  bad "propagação P2P"
  echo "=== NÓS ATRASADOS ==="
  for i in $(seq 1 "$N"); do
    h=$(height "$i" 2>/dev/null || echo -1)
    echo "n$i head=$h target=$head"
  done
fi

# 9. Producer rotation + Sybil resistance.
python3 - "$RPC" "$head" "${ADDR[@]}" > "$LAB/producers.txt" <<'PY'
import json,sys,urllib.request
base,head,*a=sys.argv[1:]
base=int(base); head=int(head)
valid={x.lower() for x in a[:8]}
seen=set(); sybil=False
for n in range(max(1,head-24),head+1):
    q=json.dumps({
      "jsonrpc":"2.0","id":1,
      "method":"eth_getBlockByNumber",
      "params":[hex(n),False]}).encode()
    try:
      u=urllib.request.Request(
        f"http://127.0.0.1:{base+1}",
        data=q,headers={"Content-Type":"application/json"})
      r=json.loads(urllib.request.urlopen(u,timeout=3).read())["result"]
      if r:
        m=r["miner"].lower()
        seen.add(m)
        if m not in valid: sybil=True
    except Exception:
      pass
print(len(seen),int(sybil))
PY

read unique sybil < "$LAB/producers.txt"
((unique>=2)) && ok "rotação de produtores unique=$unique" || bad "rotação unique=$unique"
((sybil==0)) && ok "Sybil não virou produtor" || bad "Sybil virou produtor"

# Observer RPC remains online while producers are stopped.
observer=10

# 10. Kill current producer and require continued production/fallback.
latest=$(call 1 eth_getBlockByNumber '["latest",false]')
miner=$(echo "$latest" | python3 -c 'import json,sys;print(json.load(sys.stdin)["result"]["miner"].lower())')
victim=0

for i in $(seq 1 "$P"); do
  [[ "${ADDR[$i],,}" == "$miner" ]] && victim=$i
done

if ((victim)); then
  old=$(height "$observer")
  kill "${PID[$victim]}" 2>/dev/null || true

  advanced=0
  for _ in $(seq 1 40); do
    h=$(height "$observer" 2>/dev/null || echo "$old")
    if ((h>=old+2)); then advanced=1; break; fi
    sleep 1
    if ! kill -0 "${PID[1]}" 2>/dev/null && (( i != victim )); then
      :
    fi
  done

  ((advanced)) && ok "fallback após perder n$victim" || bad "fallback"

  # Restart victim correctly.
  export RABBIT_LQC_COINBASE="${ADDR[$victim]}"
  "$G" \
    --datadir "$LAB/nodes/n$victim" \
    --networkid 928 \
    --port $((P2P+victim)) \
    --nat none \
    --nodiscover \
    --http \
    --http.addr 127.0.0.1 \
    --http.port $((RPC+victim)) \
    --http.api eth,net,web3,admin,personal,txpool \
    --http.vhosts '*' \
    --authrpc.port $((AUTH+victim)) \
    --mine \
    --miner.etherbase "${ADDR[$victim]}" \
    --password "$PW" \
    --cache 96 \
    --verbosity 2 \
    >>"$LAB/nodes/n$victim.log" 2>&1 &

  PID[$victim]=$!
  PIDS+=("${PID[$victim]}")

  target=$(height "$observer")
  caught=0
  for _ in $(seq 1 35); do
    vh=$(height "$victim" 2>/dev/null || echo 0)
    ((vh>=target-2)) && { caught=1; break; }
    sleep 1
  done

  ((caught)) && ok "produtor reiniciado fez catch-up" || bad "catch-up"
else
  bad "produtor atual não identificado"
fi

# 11. Real transaction path.
unlock="not applicable: raw transaction test pending"
tx=$(call 1 eth_sendTransaction \
  "[{\"from\":\"${ADDR[1]}\",\"to\":\"${ADDR[9]}\",\"value\":\"0xde0b6b3a7640000\",\"gas\":\"0x5208\"}]" || true)
txh=$(printf '%s' "$tx" | python3 -c '
import json,sys
try:
    obj=json.load(sys.stdin)
    print(obj.get("result","") or "")
except Exception:
    print("")
' 2>/dev/null || true)
mined=0

if [[ -n "$txh" ]]; then
  for _ in $(seq 1 35); do
    r=$(call 1 eth_getTransactionReceipt "[\"$txh\"]" || true)
    if echo "$r" | grep -q '"blockHash"'; then mined=1; break; fi
    sleep 1
  done
fi

if (( mined )); then
  ok "transação real + receipt"
else
  bad "transação"
  echo "TX_RPC=${tx:-<empty>}"
  echo "UNLOCK_RPC=${unlock:-<empty>}"
fi

# 12. Kill all producers. Chain must stop.
for i in $(seq 1 "$P"); do
  kill "${PID[$i]}" 2>/dev/null || true
done

sleep 12
z0=$(height "$observer" 2>/dev/null || echo 0)
sleep 8
z1=$(height "$observer" 2>/dev/null || echo 0)

((z1==z0)) \
  && ok "rede parou com zero produtores head=$z1" \
  || bad "rede avançou sem produtores $z0->$z1"

# 13. Bring one producer back. Chain must resume.
export RABBIT_LQC_COINBASE="${ADDR[1]}"

"$G" \
  --datadir "$LAB/nodes/n1" \
  --networkid 928 \
  --port $((P2P+1)) \
  --nat none \
  --nodiscover \
  --http \
  --http.addr 127.0.0.1 \
  --http.port $((RPC+1)) \
  --http.api eth,net,web3,admin,personal,txpool \
  --http.vhosts '*' \
  --authrpc.port $((AUTH+1)) \
  --mine \
  --miner.etherbase "${ADDR[1]}" \
  --password "$PW" \
  --cache 96 \
  --verbosity 2 \
  >>"$LAB/nodes/n1.log" 2>&1 &

PID[1]=$!
PIDS+=("${PID[1]}")

resumed=0
for _ in $(seq 1 45); do
  h=$(height 1 2>/dev/null || echo "$z1")
  ((h>z1)) && { resumed=1; break; }
  sleep 1
done

((resumed)) && ok "rede retomou com produtor novamente ativo" || bad "retomada"

# 14. Final convergence after stopping the last producer.
kill "${PID[1]}" 2>/dev/null || true
sleep 10

final=0
for i in $(seq 1 "$N"); do
  h=$(height "$i" 2>/dev/null || echo 0)
  ((h>final)) && final=$h
done

minh=999999999
maxh=0
hashes="$LAB/final-hashes.txt"
: > "$hashes"

for _ in $(seq 1 30); do
  final=0
  for i in $(seq 9 "$N"); do
    h=$(height "$i" 2>/dev/null || echo 0)
    ((h>final)) && final=$h
  done

  ready=1
  firsthash=""
  for i in $(seq 9 "$N"); do
    h=$(height "$i" 2>/dev/null || echo -1)
    hh=$(blockhash "$i" 2>/dev/null || echo "")
    ((h<final)) && ready=0
    [[ -z "$firsthash" ]] && firsthash="$hh"
    [[ -n "$hh" && -n "$firsthash" && "$hh" != "$firsthash" ]] && ready=0
  done

  ((ready)) && break
  sleep 1
done

minh=999999999
maxh=0
for i in $(seq 9 "$N"); do
  h=$(height "$i" 2>/dev/null || echo -1)
  hh=$(blockhash "$i" 2>/dev/null || echo "")
  ((h<minh)) && minh=$h
  ((h>maxh)) && maxh=$h
  echo "$i $h $hh" >> "$hashes"
done

((maxh-minh<=1)) \
  && ok "convergência final min=$minh max=$maxh" \
  || bad "convergência final min=$minh max=$maxh"

# 15. Fatal consensus/miner errors only.
fatal_errors=$(
  grep -HniE 'panic|Fatal:|missing lqc|block build failed|consensus.*error' \
    "$LAB"/nodes/*.log 2>/dev/null |
  grep -vi 'unauthorized lqc registry producer' || true
)

if [[ -n "$fatal_errors" ]]; then
  printf '%s\n' "$fatal_errors"
  bad "erros fatais LQC/miner"
else
  ok "sem erros fatais LQC/miner"
fi

echo
echo "==================== RABBIT LAB V2 ===================="
echo "PASS=$PASS FAIL=$FAIL HEAD=$final"
echo "LAB=$LAB"

if ((FAIL==0)); then
  echo "RESULT=PASS"
else
  echo "RESULT=FAIL"
  exit 2
fi
