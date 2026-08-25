#!/usr/bin/env bash
set -Eeuo pipefail

RANDOMX_REPOSITORY="${RANDOMX_REPOSITORY:-https://github.com/tevador/RandomX.git}"
RANDOMX_COMMIT="${RANDOMX_COMMIT:-7c761cf007c758056dcb6eb438a32f780f81bdbd}"
EXPECTED_GENESIS="${TESTNET_GENESIS_SHA256:-8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7}"
TARGET="${RABBIT_TARGET:?RABBIT_TARGET is required}"

case "$TARGET" in
  linux-amd64) expected_os=linux; expected_arch=x86_64 ;;
  macos-amd64) expected_os=darwin; expected_arch=x86_64 ;;
  macos-arm64) expected_os=darwin; expected_arch=arm64 ;;
  *) echo "unsupported Unix target: $TARGET" >&2; exit 1 ;;
esac

actual_os="$(go env GOOS)"
actual_arch="$(uname -m)"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

[[ "$actual_os" == "$expected_os" ]]
[[ "$actual_arch" == "$expected_arch" ]]
[[ "$(go version | awk '{print $3}')" == "go1.25.13" ]]
[[ "$(hash_file networks/rabbit-testnet/genesis.json)" == "$EXPECTED_GENESIS" ]]

work="$(mktemp -d)"
cleanup() { rm -rf -- "$work"; }
trap cleanup EXIT

git clone --filter=blob:none "$RANDOMX_REPOSITORY" "$work/RandomX"
git -C "$work/RandomX" checkout --detach "$RANDOMX_COMMIT"
[[ "$(git -C "$work/RandomX" rev-parse HEAD)" == "$RANDOMX_COMMIT" ]]
git -C "$work/RandomX" apply --check "$PWD/scripts/rabbit-release/randomx/rabbit-randomx-1g-profile.patch"
git -C "$work/RandomX" apply "$PWD/scripts/rabbit-release/randomx/rabbit-randomx-1g-profile.patch"
git -C "$work/RandomX" diff --check
grep -q '^#define RANDOMX_DATASET_BASE_SIZE  1073741824$' "$work/RandomX/src/configuration.h"

# GNU/ELF assembly must explicitly declare that it does not require an
# executable process stack. This changes binary hardening metadata only.
if [[ "$expected_os" == linux ]]; then
  stack_asm="$work/RandomX/src/jit_compiler_x86_static.S"
  if ! grep -Fq '.section .note.GNU-stack' "$stack_asm"; then
    printf '\n.section .note.GNU-stack,"",@progbits\n' >> "$stack_asm"
  fi
  grep -Fqx '.section .note.GNU-stack,"",@progbits' "$stack_asm"
fi

cmake -S "$work/RandomX" -B "$work/RandomX/build" \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_SHARED_LIBS=OFF
cmake --build "$work/RandomX/build" --config Release --parallel 2

library="$work/RandomX/build/librandomx.a"
[[ -s "$library" ]]

export CGO_ENABLED=1
export CGO_CFLAGS="-O2 -D__BLST_PORTABLE__ -I$work/RandomX/src"
export CGO_LDFLAGS="-L$work/RandomX/build -lrandomx"

go test -tags 'rabbit_workv1 rabbit_randomx' ./crypto/rabbitx ./cmd/rabbit-miner ./cmd/rabbit-core -count=1

package="rabbit-core-testnet-$TARGET-preview"
stage="$work/$package"
mkdir -p "$stage" dist

go build -tags 'rabbit_workv1 rabbit_randomx' -trimpath -o "$stage/rabbit-node" ./cmd/geth
go build -tags 'rabbit_workv1 rabbit_randomx' -trimpath -o "$stage/rabbit-miner" ./cmd/rabbit-miner
go build -tags 'rabbit_workv1 rabbit_randomx' -trimpath -o "$stage/rabbit-core" ./cmd/rabbit-core

if [[ "$expected_os" == linux ]]; then
  command -v readelf >/dev/null
  for binary in rabbit-node rabbit-miner rabbit-core; do
    stack_flags="$(readelf -W -l "$stage/$binary" | awk '$1 == "GNU_STACK" {print $7}')"
    printf '%s GNU_STACK=%s\n' "$binary" "$stack_flags"
    [[ "$stack_flags" == RW ]]
  done
fi

cp networks/rabbit-testnet/genesis.json "$stage/genesis.json"
cp docs/rabbit-core.md docs/rabbit-miner.md "$stage/"
cp scripts/rabbit-release/NOTICE-PREVIEW.txt "$stage/NOTICE-PREVIEW.txt"
: > "$stage/bootnodes.txt"

cat > "$stage/start-rabbit-core.command" <<'LAUNCHER'
#!/usr/bin/env bash
set -Eeuo pipefail
cd -- "$(dirname -- "$0")"
exec ./rabbit-core "$@"
LAUNCHER
chmod 755 "$stage/rabbit-core" "$stage/rabbit-node" "$stage/rabbit-miner" "$stage/start-rabbit-core.command"

(
  cd "$stage"
  : > SHA256SUMS.txt
  for file in *; do
    [[ -f "$file" ]] || continue
    [[ "$file" == SHA256SUMS.txt ]] && continue
    printf '%s  %s\n' "$(hash_file "$file")" "$file" >> SHA256SUMS.txt
  done
  ./rabbit-core --check
)

tar -C "$work" -czf "dist/$package.tar.gz" "$package"
printf '%s  %s\n' \
  "$(hash_file "dist/$package.tar.gz")" \
  "$package.tar.gz" \
  > "dist/$package.tar.gz.sha256"
