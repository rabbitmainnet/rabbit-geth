$ErrorActionPreference = "Stop"

$RandomXRepository = if ($env:RANDOMX_REPOSITORY) { $env:RANDOMX_REPOSITORY } else { "https://github.com/tevador/RandomX.git" }
$RandomXCommit = if ($env:RANDOMX_COMMIT) { $env:RANDOMX_COMMIT } else { "7c761cf007c758056dcb6eb438a32f780f81bdbd" }
$ExpectedGenesis = if ($env:TESTNET_GENESIS_SHA256) { $env:TESTNET_GENESIS_SHA256 } else { "8562725483c8e139083d2858ff1c10cec0e1d09bc399439d5022d4cad9e5a4a7" }
$Target = $env:RABBIT_TARGET
$SourceCommit = (git rev-parse HEAD).Trim()

if ($LASTEXITCODE -ne 0 -or -not $SourceCommit) {
    throw "could not resolve source commit"
}

$TrackedStatus = git status --short --untracked-files=no
if ($LASTEXITCODE -ne 0) {
    throw "could not inspect tracked source state"
}
if ($null -ne $TrackedStatus) {
    throw "tracked source tree is not clean"
}

if ($Target -ne "windows-amd64") { throw "unsupported Windows target: $Target" }
if ((go version) -notmatch "go1\.25\.13 windows/amd64") { throw "Go 1.25.13 windows/amd64 is required" }
if ((Get-FileHash networks/rabbit-testnet/genesis.json -Algorithm SHA256).Hash.ToLowerInvariant() -ne $ExpectedGenesis) { throw "wrong testnet genesis" }

$Work = Join-Path $env:RUNNER_TEMP "rabbit-native-$([guid]::NewGuid())"
$RandomX = Join-Path $Work "RandomX"
$Package = "rabbit-core-testnet-v1-$Target"
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
$RandomXForGcc = $RandomX.Replace("\", "/")
if (-not $RandomXForGcc) { throw "could not prepare RandomX path for GCC" }
$env:CGO_CFLAGS = "-O2 -D__BLST_PORTABLE__ -I$RandomXForGcc/src"
$env:CGO_LDFLAGS = "-L$RandomXForGcc/build -lrandomx"
Write-Host "RABBIT_RANDOMX_GCC_PATH=$RandomXForGcc"
Write-Host "RABBIT_CGO_CFLAGS=$env:CGO_CFLAGS"
Write-Host "RABBIT_CGO_LDFLAGS=$env:CGO_LDFLAGS"

go test -tags "rabbit_workv1 rabbit_randomx" ./crypto/rabbitx ./cmd/rabbit-miner ./cmd/rabbit-core -count=1
if ($LASTEXITCODE -ne 0) { throw "Rabbit Windows tests failed" }

go build -tags "rabbit_workv1 rabbit_randomx" -trimpath -o "$Stage\rabbit-node.exe" ./cmd/geth
if ($LASTEXITCODE -ne 0) { throw "Rabbit node Windows build failed" }

go build -tags "rabbit_workv1 rabbit_randomx" -trimpath -o "$Stage\rabbit-miner.exe" ./cmd/rabbit-miner
if ($LASTEXITCODE -ne 0) { throw "Rabbit miner Windows build failed" }

go build -tags "rabbit_workv1 rabbit_randomx" -trimpath -o "$Stage\rabbit-core.exe" ./cmd/rabbit-core
if ($LASTEXITCODE -ne 0) { throw "Rabbit Core Windows build failed" }

Copy-Item networks/rabbit-testnet/genesis.json "$Stage\genesis.json"
Copy-Item docs/rabbit-core.md, docs/rabbit-miner.md $Stage
Copy-Item scripts/rabbit-release/NOTICE-TESTNET.txt "$Stage\NOTICE-TESTNET.txt"

@(
    "RABBIT_RELEASE=rabbit-core-testnet-v1"
    "SOURCE_REPOSITORY=https://github.com/rabbitmainnet/rabbit-geth"
    "SOURCE_COMMIT=$SourceCommit"
    "TARGET=$Target"
    "CHAIN_ID=9280"
    "NETWORK_ID=9280"
    "GENESIS_SHA256=$ExpectedGenesis"
    "GO_VERSION=go1.25.13"
    "RANDOMX_COMMIT=$RandomXCommit"
    "RANDOMX_DATASET_BASE_SIZE=1073741824"
    "BUILD_TAGS=rabbit_workv1 rabbit_randomx"
) | Set-Content -Encoding ASCII "$Stage\BUILD-METADATA.txt"
Set-Content -Encoding ASCII -NoNewline -Path "$Stage\bootnodes.txt" -Value "enode://867431475238a2da10b62aeb2197d00baa4880f66b14ca97ec99ef51d13143791cf89893a8f41e1fcf1bd0e0f1ef86081d0c28b268953f723e6dd3c18efc8a39@137.184.105.140:30303,enode://b345298a2e97c249e2e7987f7a7b9289d7f0f6bc02b06bba8d7b6c478ae62a293952c8187fb67c30d2ecf60332080b79a8ab3584d4d87d34bf549e6122208b07@162.243.49.184:30303`n"

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
