#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
OUT_DIR="$ROOT_DIR/dist/desktop"
MAC_APP_NAME="CCVar Quant Lab"
MAC_APP_DIR="$OUT_DIR/${MAC_APP_NAME}.app"
MAC_CONTENTS_DIR="$MAC_APP_DIR/Contents"
MACOS_DIR="$MAC_CONTENTS_DIR/MacOS"
MAC_RESOURCES_DIR="$MAC_CONTENTS_DIR/Resources"
MAC_RESOURCES_DOCS_DIR="$MAC_RESOURCES_DIR/docs"
MAC_README="$OUT_DIR/README-macos.txt"
TOP_DOCS_DIR="$OUT_DIR/docs"
WIN_DIR="$OUT_DIR/CCVar Quant Lab Windows x64"
WIN_DOCS_DIR="$WIN_DIR/docs"
SHA256SUMS="$OUT_DIR/SHA256SUMS.txt"
RELEASE_MANIFEST="$OUT_DIR/release-manifest.json"
PACKAGE_TOUCH_TIME="${CCVAR_PACKAGE_TOUCH_TIME:-202601010000.00}"

make_zip() {
  local zip_name="$1"
  shift
  local list_file="$OUT_DIR/.${zip_name}.filelist"
  (
    cd "$OUT_DIR"
    find "$@" -type f -print | LC_ALL=C sort > "$list_file"
    COPYFILE_DISABLE=1 zip -qX "$zip_name" -@ < "$list_file"
    rm -f "$list_file"
  )
}

normalize_package_timestamps() {
  find "$OUT_DIR" -exec touch -t "$PACKAGE_TOUCH_TIME" {} +
}

cd "$ROOT_DIR"

rm -rf "$OUT_DIR"
mkdir -p "$BIN_DIR" "$OUT_DIR" "$MACOS_DIR" "$MAC_RESOURCES_DIR" "$MAC_RESOURCES_DOCS_DIR" "$TOP_DOCS_DIR" "$WIN_DIR" "$WIN_DOCS_DIR"

npm run build

GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false -o "$BIN_DIR/ccvar-quant-darwin-arm64" ./cmd/ccvar-quant
GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -o "$BIN_DIR/ccvar-quant-windows-amd64.exe" ./cmd/ccvar-quant

cp "$BIN_DIR/ccvar-quant-darwin-arm64" "$MACOS_DIR/ccvar-quant"
chmod +x "$MACOS_DIR/ccvar-quant"

cat > "$MACOS_DIR/$MAC_APP_NAME" <<'APP'
#!/bin/sh
APP_NAME="CCVar Quant Lab"
APP_BIN="$(dirname "$0")/ccvar-quant"
DATA_DIR="${HOME}/Library/Application Support/${APP_NAME}"
LOG_DIR="${DATA_DIR}/logs"
ADDR="${CCVAR_ADDR:-127.0.0.1:8787}"
DB_PATH="${CCVAR_DB_PATH:-${DATA_DIR}/ccvar_quant.db}"

mkdir -p "$DATA_DIR" "$LOG_DIR"
exec "$APP_BIN" --addr "$ADDR" --db "$DB_PATH" --open >> "${LOG_DIR}/client.log" 2>&1
APP
chmod +x "$MACOS_DIR/$MAC_APP_NAME"

cat > "$MAC_CONTENTS_DIR/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>CCVar Quant Lab</string>
  <key>CFBundleExecutable</key>
  <string>CCVar Quant Lab</string>
  <key>CFBundleIdentifier</key>
  <string>com.ccvar.quantlab</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>CCVar Quant Lab</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>0.1.0</string>
  <key>CFBundleVersion</key>
  <string>0.1.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
PLIST

cat > "$MAC_RESOURCES_DIR/README.txt" <<'README'
CCVar Quant Lab for macOS

Open "CCVar Quant Lab.app" to start the local AI quant client.
The app runs a local-only API on http://127.0.0.1:8787/ and opens that URL in your default browser.

Safety boundary:
  Production/mainnet trading is disabled in this build.
  Guarded execution supports Binance Spot Testnet/Demo and OKX Demo Trading only.
  Vault credentials are encrypted locally; never add a production/mainnet API key.
  Withdrawal permission is rejected by the app.

Before distribution:
  Verify the package checksums from the dist/desktop folder:
    LC_ALL=C LANG=C shasum -a 256 -c SHA256SUMS.txt
  Full release verification is:
    npm run verify:release

Local data:
  ~/Library/Application Support/CCVar Quant Lab/ccvar_quant.db

Logs:
  ~/Library/Application Support/CCVar Quant Lab/logs/client.log

Environment overrides:
  CCVAR_ADDR
  CCVAR_DB_PATH

Operator runbook:
  docs/operator-runbook.zh-CN.md

Safety contract:
  docs/safety.md
README

cp "$MAC_RESOURCES_DIR/README.txt" "$MAC_README"
cp docs/operator-runbook.zh-CN.md docs/safety.md "$MAC_RESOURCES_DOCS_DIR/"
cp docs/operator-runbook.zh-CN.md docs/safety.md "$TOP_DOCS_DIR/"

cp "$BIN_DIR/ccvar-quant-windows-amd64.exe" "$WIN_DIR/ccvar-quant.exe"

cat > "$WIN_DIR/Start CCVar Quant Lab.cmd" <<'CMD'
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

echo Starting CCVar Quant Lab on http://%ADDR%/
echo Local data: %DB_PATH%
echo Logs: %LOG_PATH%
>> "%LOG_PATH%" echo [%DATE% %TIME%] Starting CCVar Quant Lab on http://%ADDR%/
start "CCVar Quant Lab" /D "%~dp0" cmd /c ""%~dp0ccvar-quant.exe" --addr "%ADDR%" --db "%DB_PATH%" --open >> "%LOG_PATH%" 2>&1"
CMD

cat > "$WIN_DIR/README.txt" <<'README'
CCVar Quant Lab for Windows x64

Double-click "Start CCVar Quant Lab.cmd" to start the local AI quant client.
The app runs a local-only API on http://127.0.0.1:8787/ and opens that URL in your default browser.

Safety boundary:
  Production/mainnet trading is disabled in this build.
  Guarded execution supports Binance Spot Testnet/Demo and OKX Demo Trading only.
  Vault credentials are encrypted locally; never add a production/mainnet API key.
  Withdrawal permission is rejected by the app.

Before distribution:
  Verify the package checksums from the dist\desktop folder:
    certutil -hashfile ccvar-quant-lab-windows-amd64.zip SHA256
  Full release verification from the source workspace is:
    npm run verify:release

Local data:
  %APPDATA%\CCVar Quant Lab\ccvar_quant.db

Logs:
  %APPDATA%\CCVar Quant Lab\logs\client.log

Environment overrides:
  CCVAR_ADDR
  CCVAR_DB_PATH

Operator runbook:
  docs\operator-runbook.zh-CN.md

Safety contract:
  docs\safety.md
README
cp docs/operator-runbook.zh-CN.md docs/safety.md "$WIN_DOCS_DIR/"

find "$OUT_DIR" -name ".DS_Store" -delete
normalize_package_timestamps

if command -v zip >/dev/null 2>&1; then
  make_zip "ccvar-quant-lab-macos-arm64.zip" "CCVar Quant Lab.app" "README-macos.txt" "docs"
elif command -v ditto >/dev/null 2>&1; then
  COPYFILE_DISABLE=1 ditto -c -k --keepParent "$MAC_APP_DIR" "$OUT_DIR/ccvar-quant-lab-macos-arm64.zip"
fi

if command -v zip >/dev/null 2>&1; then
  make_zip "ccvar-quant-lab-windows-amd64.zip" "CCVar Quant Lab Windows x64"
fi

find "$OUT_DIR" -name ".DS_Store" -delete

node <<'NODE'
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");

const root = process.cwd();
const outDir = path.join(root, "dist", "desktop");
const files = [
  "ccvar-quant-lab-macos-arm64.zip",
  "ccvar-quant-lab-windows-amd64.zip",
].map((name) => path.join(outDir, name));
const criticalFilePaths = [
  "README-macos.txt",
  "docs/operator-runbook.zh-CN.md",
  "docs/safety.md",
  "CCVar Quant Lab.app/Contents/Info.plist",
  "CCVar Quant Lab.app/Contents/MacOS/ccvar-quant",
  "CCVar Quant Lab.app/Contents/MacOS/CCVar Quant Lab",
  "CCVar Quant Lab.app/Contents/Resources/README.txt",
  "CCVar Quant Lab.app/Contents/Resources/docs/operator-runbook.zh-CN.md",
  "CCVar Quant Lab.app/Contents/Resources/docs/safety.md",
  "CCVar Quant Lab Windows x64/ccvar-quant.exe",
  "CCVar Quant Lab Windows x64/Start CCVar Quant Lab.cmd",
  "CCVar Quant Lab Windows x64/README.txt",
  "CCVar Quant Lab Windows x64/docs/operator-runbook.zh-CN.md",
  "CCVar Quant Lab Windows x64/docs/safety.md",
];

const artifacts = files.map((file) => {
  const bytes = fs.readFileSync(file);
  return {
    file: path.basename(file),
    sizeBytes: bytes.length,
    sha256: crypto.createHash("sha256").update(bytes).digest("hex"),
  };
});
const criticalFiles = criticalFilePaths.map((relativePath) => {
  const file = path.join(outDir, relativePath);
  const bytes = fs.readFileSync(file);
  return {
    path: relativePath,
    sizeBytes: bytes.length,
    sha256: crypto.createHash("sha256").update(bytes).digest("hex"),
  };
});

function releaseGeneratedAt() {
  if (process.env.CCVAR_RELEASE_GENERATED_AT) return process.env.CCVAR_RELEASE_GENERATED_AT;
  if (process.env.SOURCE_DATE_EPOCH) {
    const epochSeconds = Number(process.env.SOURCE_DATE_EPOCH);
    if (!Number.isFinite(epochSeconds) || epochSeconds < 0) {
      throw new Error("SOURCE_DATE_EPOCH must be a non-negative Unix timestamp");
    }
    return new Date(epochSeconds * 1000).toISOString();
  }
  return "2026-01-01T00:00:00.000Z";
}

const manifest = {
  manifestVersion: 1,
  product: "CCVar Quant Lab",
  version: "0.1.0",
  generatedAt: releaseGeneratedAt(),
  artifacts,
  criticalFiles,
  verification: {
    appURL: "http://127.0.0.1:8787/",
    noProductionTrading: true,
    supportedPrivateEnvironments: ["Binance testnet/demo", "OKX demo"],
  },
};

fs.writeFileSync(
  path.join(outDir, "SHA256SUMS.txt"),
  artifacts.map((artifact) => `${artifact.sha256}  ${artifact.file}`).join("\n") + "\n",
);
fs.writeFileSync(path.join(outDir, "release-manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
NODE

find "$OUT_DIR" -name ".DS_Store" -delete

echo "Desktop packages written to $OUT_DIR"
