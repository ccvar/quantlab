$ErrorActionPreference = "Stop"

$RootDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$OutDir = Join-Path $RootDir "dist/native/windows"
$PackageDir = Join-Path $OutDir "CCVar Quant Lab"
$DocsDir = Join-Path $PackageDir "docs"
$ProjectDir = Join-Path $RootDir "desktop/windows/CCVarQuantLab"
$ZipPath = Join-Path $OutDir "ccvar-quant-lab-windows-native.zip"

Set-Location $RootDir

if (Test-Path $OutDir) {
  Remove-Item $OutDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $PackageDir, $DocsDir | Out-Null

npm run build
go build -trimpath -buildvcs=false -o (Join-Path $PackageDir "ccvar-quant.exe") ./cmd/ccvar-quant
dotnet publish $ProjectDir -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true -o $PackageDir

@"
@echo off
setlocal
set "APP_NAME=CCVar Quant Lab"
set "DATA_DIR=%APPDATA%\%APP_NAME%"
set "LOG_DIR=%DATA_DIR%\logs"
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"
set "LOG_PATH=%LOG_DIR%\client.log"

set "ADDR=127.0.0.1:8787"
if not "%CCVAR_ADDR%"=="" set "ADDR=%CCVAR_ADDR%"

set "DB_PATH=%DATA_DIR%\ccvar_quant.db"
if not "%CCVAR_DB_PATH%"=="" set "DB_PATH=%CCVAR_DB_PATH%"

echo Starting CCVar Quant Lab Web on http://%ADDR%/
>> "%LOG_PATH%" echo [%DATE% %TIME%] Starting CCVar Quant Lab Web on http://%ADDR%/
start "CCVar Quant Lab Web" /D "%~dp0" cmd /c ""%~dp0ccvar-quant.exe" --addr "%ADDR%" --db "%DB_PATH%" --open >> "%LOG_PATH%" 2>&1"
"@ | Set-Content -Encoding ASCII (Join-Path $PackageDir "Start CCVar Quant Lab Web.cmd")

@"
CCVar Quant Lab native Windows client

Open "CCVar Quant Lab.exe" for the standalone desktop client. It starts the local Go API and renders the UI in a native WebView2 window.

The standalone client prefers http://127.0.0.1:8787/ and automatically chooses the next available loopback port when that default is already occupied, unless CCVAR_ADDR is set.

Open "Start CCVar Quant Lab Web.cmd" if you prefer the browser launcher. It starts the same local Go API and opens http://127.0.0.1:8787/ in your default browser.

Desktop runtime:
  The standalone client uses Microsoft Edge WebView2 Runtime. Most Windows 10/11 machines already have it. If the app cannot create the WebView2 window, install the Evergreen WebView2 Runtime from Microsoft.

Safety boundary:
  Production/mainnet trading is disabled in this build.
  Guarded execution supports Binance Spot Testnet/Demo and OKX Demo Trading only.
  Vault credentials are encrypted locally; never add a production/mainnet API key.
  Withdrawal permission is rejected by the app.

Local data:
  %APPDATA%\CCVar Quant Lab\ccvar_quant.db

Logs:
  %APPDATA%\CCVar Quant Lab\logs\client.log

Environment overrides:
  CCVAR_ADDR
  CCVAR_DB_PATH
  CCVAR_EXCHANGE_PROXY

Network note:
  Exchange API calls use HTTP_PROXY / HTTPS_PROXY / ALL_PROXY when present.
  Set CCVAR_EXCHANGE_PROXY=http://127.0.0.1:7897 to force a proxy, or CCVAR_EXCHANGE_PROXY=direct to force direct exchange connections.
"@ | Set-Content -Encoding ASCII (Join-Path $PackageDir "README.txt")

Copy-Item (Join-Path $RootDir "docs/operator-runbook.zh-CN.md") $DocsDir
Copy-Item (Join-Path $RootDir "docs/safety.md") $DocsDir

if (Test-Path $ZipPath) {
  Remove-Item $ZipPath -Force
}
Compress-Archive -Path (Join-Path $PackageDir "*") -DestinationPath $ZipPath -Force
$Hash = Get-FileHash -Algorithm SHA256 $ZipPath
"$($Hash.Hash.ToLowerInvariant())  $(Split-Path $ZipPath -Leaf)" | Set-Content -Encoding ASCII (Join-Path $OutDir "SHA256SUMS.txt")

Write-Host "Native Windows package written to $ZipPath"
