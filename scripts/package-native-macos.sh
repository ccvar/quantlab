#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/dist/native/macos"
BIN_DIR="$ROOT_DIR/bin"
BUILD_DIR="$ROOT_DIR/dist/native/.build-macos"
APP_NAME="CCVar Quant Lab"
WEB_APP_NAME="CCVar Quant Lab Web"
APP_DIR="$OUT_DIR/${APP_NAME}.app"
WEB_APP_DIR="$OUT_DIR/${WEB_APP_NAME}.app"
NATIVE_MACOS_DIR="$APP_DIR/Contents/MacOS"
NATIVE_RESOURCES_DIR="$APP_DIR/Contents/Resources"
NATIVE_DOCS_DIR="$NATIVE_RESOURCES_DIR/docs"
WEB_MACOS_DIR="$WEB_APP_DIR/Contents/MacOS"
WEB_RESOURCES_DIR="$WEB_APP_DIR/Contents/Resources"
WEB_DOCS_DIR="$WEB_RESOURCES_DIR/docs"
ZIP_NAME="ccvar-quant-lab-macos-native.zip"

cd "$ROOT_DIR"
export LC_ALL=C
export LANG=C
export LC_CTYPE=C

if ! command -v swiftc >/dev/null 2>&1; then
  echo "swiftc is required to package the native macOS client" >&2
  exit 1
fi
if ! command -v xcrun >/dev/null 2>&1; then
  echo "xcrun is required to package the native macOS client" >&2
  exit 1
fi
if ! command -v lipo >/dev/null 2>&1; then
  echo "lipo is required to package the native macOS client" >&2
  exit 1
fi

rm -rf "$OUT_DIR"
rm -rf "$BUILD_DIR"
mkdir -p "$OUT_DIR" "$BIN_DIR" "$BUILD_DIR" "$NATIVE_MACOS_DIR" "$NATIVE_RESOURCES_DIR" "$NATIVE_DOCS_DIR" "$WEB_MACOS_DIR" "$WEB_RESOURCES_DIR" "$WEB_DOCS_DIR"

npm run build

for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" go build -trimpath -buildvcs=false -o "$BUILD_DIR/ccvar-quant-darwin-$arch" ./cmd/ccvar-quant
done
lipo -create "$BUILD_DIR/ccvar-quant-darwin-arm64" "$BUILD_DIR/ccvar-quant-darwin-amd64" -output "$BIN_DIR/ccvar-quant-darwin-universal"
cp "$BIN_DIR/ccvar-quant-darwin-universal" "$NATIVE_MACOS_DIR/ccvar-quant"
cp "$BIN_DIR/ccvar-quant-darwin-universal" "$WEB_MACOS_DIR/ccvar-quant"
chmod +x "$NATIVE_MACOS_DIR/ccvar-quant" "$WEB_MACOS_DIR/ccvar-quant"

for target in arm64-apple-macos12.0 x86_64-apple-macos12.0; do
  xcrun --sdk macosx swiftc desktop/macos/CCVarQuantLab.swift \
  -O \
  -target "$target" \
  -framework Cocoa \
  -framework WebKit \
    -o "$BUILD_DIR/$APP_NAME-$target"
done
lipo -create "$BUILD_DIR/$APP_NAME-arm64-apple-macos12.0" "$BUILD_DIR/$APP_NAME-x86_64-apple-macos12.0" -output "$NATIVE_MACOS_DIR/$APP_NAME"
chmod +x "$NATIVE_MACOS_DIR/$APP_NAME"

cat > "$APP_DIR/Contents/Info.plist" <<'PLIST'
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
  <string>com.ccvar.quantlab.native</string>
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
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsLocalNetworking</key>
    <true/>
  </dict>
</dict>
</plist>
PLIST

cat > "$WEB_MACOS_DIR/$WEB_APP_NAME" <<'APP'
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
chmod +x "$WEB_MACOS_DIR/$WEB_APP_NAME"

cat > "$WEB_APP_DIR/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>CCVar Quant Lab Web</string>
  <key>CFBundleExecutable</key>
  <string>CCVar Quant Lab Web</string>
  <key>CFBundleIdentifier</key>
  <string>com.ccvar.quantlab.weblauncher</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>CCVar Quant Lab Web</string>
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

cat > "$NATIVE_RESOURCES_DIR/README.txt" <<'README'
CCVar Quant Lab native macOS client

Open "CCVar Quant Lab.app" for the standalone desktop client. It starts the local Go API and renders the UI in a native WKWebView window.

Open "CCVar Quant Lab Web.app" if you prefer the browser launcher. It starts the same local Go API and opens http://127.0.0.1:8787/ in your default browser.

Safety boundary:
  Production/mainnet trading is disabled in this build.
  Guarded execution supports Binance Spot Testnet/Demo and OKX Demo Trading only.
  Vault credentials are encrypted locally; never add a production/mainnet API key.
  Withdrawal permission is rejected by the app.

Local data:
  ~/Library/Application Support/CCVar Quant Lab/ccvar_quant.db

Logs:
  ~/Library/Application Support/CCVar Quant Lab/logs/client.log

Environment overrides:
  CCVAR_ADDR
  CCVAR_DB_PATH
README
cp "$NATIVE_RESOURCES_DIR/README.txt" "$WEB_RESOURCES_DIR/README.txt"
cp docs/operator-runbook.zh-CN.md docs/safety.md "$NATIVE_DOCS_DIR/"
cp docs/operator-runbook.zh-CN.md docs/safety.md "$WEB_DOCS_DIR/"

find "$OUT_DIR" -name ".DS_Store" -delete
(
  cd "$OUT_DIR"
  rm -f "$ZIP_NAME" SHA256SUMS.txt
  COPYFILE_DISABLE=1 zip -qrX "$ZIP_NAME" "${APP_NAME}.app" "${WEB_APP_NAME}.app"
  shasum -a 256 "$ZIP_NAME" > SHA256SUMS.txt
)

rm -rf "$BUILD_DIR"

echo "Native macOS package written to $OUT_DIR/$ZIP_NAME"
