$ErrorActionPreference = "Stop"

$RandomXRepository = if ($env:RANDOMX_REPOSITORY) { $env:RANDOMX_REPOSITORY } else { "https://github.com/tevador/RandomX.git" }
$RandomXCommit = if ($env:RANDOMX_COMMIT) { $env:RANDOMX_COMMIT } else { "7c761cf007c758056dcb6eb438a32f780f81bdbd" }
$ExpectedGenesis = if ($env:TESTNET_GENESIS_SHA256) { $env:TESTNET_GENESIS_SHA256 } else { "8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7" }
$Target = $env:RABBIT_TARGET

if ($Target -ne "windows-amd64") { throw "unsupported Windows target: $Target" }
if ((go version) -notmatch "go1\.25\.13 windows/amd64") { throw "Go 1.25.13 windows/amd64 is required" }
if ((Get-FileHash networks/rabbit-testnet/genesis.json -Algorithm SHA256).Hash.ToLowerInvariant() -ne $ExpectedGenesis) { throw "wrong testnet genesis" }

$Work = Join-Path $env:RUNNER_TEMP "rabbit-native-$([guid]::NewGuid())"
$RandomX = Join-Path $Work "RandomX"
$Package = "rabbit-core-testnet-$Target-preview"
$Stage = Join-Path $Work $Package
$Dist = Join-Path $PWD "dist"

New-Item -ItemType Directory -Force -Path $Work, $Stage, $Dist | Out-Null

git clone --filter=blob:none $RandomXRepository $RandomX
git -C $RandomX checkout --detach $RandomXCommit
if ((git -C $RandomX rev-parse HEAD).Trim() -ne $RandomXCommit) { throw "wrong RandomX commit" }
git -C $RandomX apply --check "$PWD/scripts/rabbit-release/randomx/rabbit-randomx-1g-profile.patch"
git -C $RandomX apply "$PWD/scripts/rabbit-release/randomx/rabbit-randomx-1g-profile.patch"
git -C $RandomX diff --check
if (-not (Select-String -Quiet -Path "$RandomX\src\configuration.h" -Pattern '^#define RANDOMX_DATASET_BASE_SIZE  1073741824$')) { throw "Rabbit RandomX 1 GiB profile was not applied" }

$Bash = "C:\msys64\usr\bin\bash.exe"
if (-not (Test-Path $Bash)) { throw "MSYS2 is not installed on the Windows runner" }

$BuildScript = @'
set -Eeuo pipefail
/usr/bin/pacman --noconfirm -Sy --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-cmake mingw-w64-x86_64-make nasm zip
export PATH="/mingw64/bin:$PATH"
export RANDOMX_POSIX="$(cygpath -u "$RANDOMX_NATIVE")"
cd "$RANDOMX_POSIX"
cmake -S . -B build -G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF
cmake --build build --parallel 2
test -s build/librandomx.a
'@

$env:RANDOMX_NATIVE = $RandomX
$BuildScriptPath = Join-Path $Work "build-randomx.sh"
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText(
    $BuildScriptPath,
    $BuildScript.Replace("`r`n", "`n"),
    $Utf8NoBom
)
& $Bash $BuildScriptPath
if ($LASTEXITCODE -ne 0) { throw "RandomX Windows build failed" }

$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CC = "C:\msys64\mingw64\bin\gcc.exe"
$env:CXX = "C:\msys64\mingw64\bin\g++.exe"
$env:CGO_ENABLED = "1"
$RandomXPosix = (& $Bash -lc 'cygpath -u "$RANDOMX_NATIVE"').Trim()
if (-not $RandomXPosix) { throw "could not convert RandomX path for MinGW" }
$env:CGO_CFLAGS = "-O2 -D__BLST_PORTABLE__ -I$RandomXPosix/src"
$env:CGO_LDFLAGS = "-L$RandomXPosix/build -lrandomx"

go test -tags "rabbit_workv1 rabbit_randomx" ./crypto/rabbitx ./cmd/rabbit-miner ./cmd/rabbit-core -count=1
go build -tags "rabbit_workv1 rabbit_randomx" -trimpath -o "$Stage\rabbit-node.exe" ./cmd/geth
go build -tags "rabbit_workv1 rabbit_randomx" -trimpath -o "$Stage\rabbit-miner.exe" ./cmd/rabbit-miner
go build -tags "rabbit_workv1 rabbit_randomx" -trimpath -o "$Stage\rabbit-core.exe" ./cmd/rabbit-core

Copy-Item networks/rabbit-testnet/genesis.json "$Stage\genesis.json"
Copy-Item docs/rabbit-core.md, docs/rabbit-miner.md $Stage
Copy-Item scripts/rabbit-release/NOTICE-PREVIEW.txt "$Stage\NOTICE-PREVIEW.txt"
New-Item -ItemType File -Force -Path "$Stage\bootnodes.txt" | Out-Null

@'
@echo off
cd /d "%~dp0"
rabbit-core.exe %*
pause
'@ | Set-Content -Encoding ASCII "$Stage\Start-Rabbit-Core.cmd"

Push-Location $Stage
try {
    Get-ChildItem -File | Where-Object Name -ne "SHA256SUMS.txt" | Sort-Object Name | ForEach-Object {
        $hash = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($_.Name)"
    } | Set-Content -Encoding ASCII SHA256SUMS.txt
    & .\rabbit-core.exe --check
    if ($LASTEXITCODE -ne 0) { throw "Rabbit Core --check failed" }
} finally {
    Pop-Location
}

$Zip = Join-Path $Dist "$Package.zip"
Compress-Archive -Path $Stage -DestinationPath $Zip -CompressionLevel Optimal
$ZipHash = (Get-FileHash $Zip -Algorithm SHA256).Hash.ToLowerInvariant()
"$ZipHash  $Package.zip" | Set-Content -Encoding ASCII "$Zip.sha256"
