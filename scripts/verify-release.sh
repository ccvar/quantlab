#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/dist/desktop"
VERIFY_ADDR="${CCVAR_VERIFY_ADDR:-127.0.0.1:8790}"
VERIFY_URL="http://${VERIFY_ADDR}"
MOCK_ADDR="${CCVAR_VERIFY_MOCK_ADDR:-127.0.0.1:8791}"
MOCK_URL="http://${MOCK_ADDR}"
TMP_DIR="$(mktemp -d)"
SERVER_PID=""
MOCK_PID=""

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$MOCK_PID" ] && kill -0 "$MOCK_PID" >/dev/null 2>&1; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "CCVar release verify: ${VERIFY_URL}"

if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"${VERIFY_ADDR##*:}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "verify address already in use: ${VERIFY_ADDR}" >&2
  exit 1
fi

if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"${MOCK_ADDR##*:}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "mock exchange address already in use: ${MOCK_ADDR}" >&2
  exit 1
fi

go test ./...
npm run package:desktop
cp "$OUT_DIR/SHA256SUMS.txt" "$TMP_DIR/SHA256SUMS.first"
cp "$OUT_DIR/release-manifest.json" "$TMP_DIR/release-manifest.first.json"
npm run package:desktop
if ! cmp -s "$TMP_DIR/SHA256SUMS.first" "$OUT_DIR/SHA256SUMS.txt"; then
  echo "desktop package hashes are not reproducible across consecutive builds" >&2
  diff -u "$TMP_DIR/SHA256SUMS.first" "$OUT_DIR/SHA256SUMS.txt" >&2 || true
  exit 1
fi
if ! cmp -s "$TMP_DIR/release-manifest.first.json" "$OUT_DIR/release-manifest.json"; then
  echo "release manifest is not reproducible across consecutive builds" >&2
  diff -u "$TMP_DIR/release-manifest.first.json" "$OUT_DIR/release-manifest.json" >&2 || true
  exit 1
fi
echo "ok reproducible desktop package hashes and manifest"

required_files=(
  "$OUT_DIR/ccvar-quant-lab-macos-arm64.zip"
  "$OUT_DIR/ccvar-quant-lab-windows-amd64.zip"
  "$OUT_DIR/SHA256SUMS.txt"
  "$OUT_DIR/release-manifest.json"
)
for file in "${required_files[@]}"; do
  if [ ! -s "$file" ]; then
    echo "missing release artifact: $file" >&2
    exit 1
  fi
done

(
  cd "$OUT_DIR"
  LC_ALL=C LANG=C shasum -a 256 -c SHA256SUMS.txt
)

node <<'NODE'
const fs = require("fs");
const crypto = require("crypto");
const manifest = JSON.parse(fs.readFileSync("dist/desktop/release-manifest.json", "utf8"));

if (manifest.product !== "CCVar Quant Lab") throw new Error("release manifest product mismatch");
if (manifest.version !== "0.1.0") throw new Error("release manifest version mismatch");
if (manifest.manifestVersion !== 1) throw new Error("release manifest version marker mismatch");
if (!manifest.verification?.noProductionTrading) throw new Error("release manifest missing noProductionTrading flag");
if (!Array.isArray(manifest.artifacts) || manifest.artifacts.length !== 2) throw new Error("release manifest must contain two zip artifacts");
const requiredCriticalFiles = [
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

for (const artifact of manifest.artifacts) {
  const path = `dist/desktop/${artifact.file}`;
  const bytes = fs.readFileSync(path);
  const sha256 = crypto.createHash("sha256").update(bytes).digest("hex");
  if (sha256 !== artifact.sha256) throw new Error(`sha256 mismatch for ${artifact.file}`);
  if (bytes.length !== artifact.sizeBytes) throw new Error(`size mismatch for ${artifact.file}`);
}
if (!Array.isArray(manifest.criticalFiles)) throw new Error("release manifest missing criticalFiles");
const criticalByPath = new Map(manifest.criticalFiles.map((file) => [file.path, file]));
for (const relativePath of requiredCriticalFiles) {
  const entry = criticalByPath.get(relativePath);
  if (!entry) throw new Error(`release manifest missing critical file ${relativePath}`);
  const filePath = `dist/desktop/${relativePath}`;
  const bytes = fs.readFileSync(filePath);
  const sha256 = crypto.createHash("sha256").update(bytes).digest("hex");
  if (sha256 !== entry.sha256) throw new Error(`critical file sha256 mismatch for ${relativePath}`);
  if (bytes.length !== entry.sizeBytes) throw new Error(`critical file size mismatch for ${relativePath}`);
}

const text = JSON.stringify(manifest);
if (text.includes("/Users/") || /apiKey|secret|passphrase|ciphertext|salt|nonce/i.test(text)) {
  throw new Error("release manifest contains forbidden material");
}
console.log("ok release manifest");
NODE

if find "$OUT_DIR" -name ".DS_Store" -print | grep -q .; then
  echo "release directory contains .DS_Store" >&2
  exit 1
fi
echo "ok no .DS_Store"

if command -v unzip >/dev/null 2>&1; then
  if (unzip -l "$OUT_DIR/ccvar-quant-lab-macos-arm64.zip"; unzip -l "$OUT_DIR/ccvar-quant-lab-windows-amd64.zip") | rg "(__MACOSX|/\._|\.DS_Store)" >/dev/null; then
    echo "zip contains macOS auxiliary files" >&2
    exit 1
  fi
  if ! unzip -l "$OUT_DIR/ccvar-quant-lab-macos-arm64.zip" | rg "docs/operator-runbook\.zh-CN\.md" >/dev/null; then
    echo "macOS zip missing operator runbook" >&2
    exit 1
  fi
  if ! unzip -l "$OUT_DIR/ccvar-quant-lab-macos-arm64.zip" | rg "docs/safety\.md" >/dev/null; then
    echo "macOS zip missing safety contract" >&2
    exit 1
  fi
  if ! unzip -l "$OUT_DIR/ccvar-quant-lab-windows-amd64.zip" | rg "CCVar Quant Lab Windows x64/docs/operator-runbook\.zh-CN\.md" >/dev/null; then
    echo "Windows zip missing operator runbook" >&2
    exit 1
  fi
  if ! unzip -l "$OUT_DIR/ccvar-quant-lab-windows-amd64.zip" | rg "CCVar Quant Lab Windows x64/docs/safety\.md" >/dev/null; then
    echo "Windows zip missing safety contract" >&2
    exit 1
  fi
  echo "ok clean zip listings"

  EXTRACT_DIR="$TMP_DIR/extracted-zips"
  mkdir -p "$EXTRACT_DIR/macos" "$EXTRACT_DIR/windows"
  unzip -q "$OUT_DIR/ccvar-quant-lab-macos-arm64.zip" -d "$EXTRACT_DIR/macos"
  unzip -q "$OUT_DIR/ccvar-quant-lab-windows-amd64.zip" -d "$EXTRACT_DIR/windows"
  EXTRACT_DIR="$EXTRACT_DIR" node <<'NODE'
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");

const extractDir = process.env.EXTRACT_DIR;
const manifest = JSON.parse(fs.readFileSync("dist/desktop/release-manifest.json", "utf8"));

function hashFile(file) {
  const bytes = fs.readFileSync(file);
  return {
    sizeBytes: bytes.length,
    sha256: crypto.createHash("sha256").update(bytes).digest("hex"),
  };
}

function walk(dir, entries = []) {
  for (const name of fs.readdirSync(dir)) {
    const file = path.join(dir, name);
    const stat = fs.lstatSync(file);
    entries.push(file);
    if (stat.isDirectory()) walk(file, entries);
  }
  return entries;
}

function rootFor(relativePath) {
  if (relativePath === "README-macos.txt" || relativePath.startsWith("docs/") || relativePath.startsWith("CCVar Quant Lab.app/")) {
    return path.join(extractDir, "macos");
  }
  if (relativePath.startsWith("CCVar Quant Lab Windows x64/")) {
    return path.join(extractDir, "windows");
  }
  throw new Error(`cannot map critical file to extracted artifact: ${relativePath}`);
}

for (const extractedRoot of [path.join(extractDir, "macos"), path.join(extractDir, "windows")]) {
  for (const file of walk(extractedRoot)) {
    const relative = path.relative(extractedRoot, file);
    if (relative.includes("__MACOSX") || relative.includes(".DS_Store") || /(^|[/\\])\._/.test(relative)) {
      throw new Error(`extracted zip contains auxiliary file: ${relative}`);
    }
  }
}

for (const entry of manifest.criticalFiles || []) {
  const file = path.join(rootFor(entry.path), entry.path);
  if (!fs.existsSync(file)) throw new Error(`extracted zip missing critical file ${entry.path}`);
  const actual = hashFile(file);
  if (actual.sha256 !== entry.sha256) throw new Error(`extracted critical file sha256 mismatch for ${entry.path}`);
  if (actual.sizeBytes !== entry.sizeBytes) throw new Error(`extracted critical file size mismatch for ${entry.path}`);
}

for (const executable of [
  "CCVar Quant Lab.app/Contents/MacOS/ccvar-quant",
  "CCVar Quant Lab.app/Contents/MacOS/CCVar Quant Lab",
]) {
  const file = path.join(extractDir, "macos", executable);
  const mode = fs.statSync(file).mode;
  if ((mode & 0o111) === 0) throw new Error(`extracted macOS executable bit missing: ${executable}`);
}

for (const file of [
  "CCVar Quant Lab Windows x64/Start CCVar Quant Lab.cmd",
  "CCVar Quant Lab Windows x64/ccvar-quant.exe",
]) {
  if (!fs.existsSync(path.join(extractDir, "windows", file))) {
    throw new Error(`extracted Windows package missing ${file}`);
  }
}

console.log("ok extracted zip critical files");
NODE
fi

if command -v plutil >/dev/null 2>&1; then
  plutil -lint "$OUT_DIR/CCVar Quant Lab.app/Contents/Info.plist"
fi

if command -v file >/dev/null 2>&1; then
  file "$OUT_DIR/CCVar Quant Lab.app/Contents/MacOS/ccvar-quant" "$OUT_DIR/CCVar Quant Lab Windows x64/ccvar-quant.exe"
fi

if command -v go >/dev/null 2>&1; then
  go version -m "$OUT_DIR/CCVar Quant Lab.app/Contents/MacOS/ccvar-quant" > "$TMP_DIR/macos-buildinfo.txt"
  go version -m "$OUT_DIR/CCVar Quant Lab Windows x64/ccvar-quant.exe" > "$TMP_DIR/windows-buildinfo.txt"
  rg "path\\s+ccvar.com/web3quant/cmd/ccvar-quant" "$TMP_DIR/macos-buildinfo.txt" >/dev/null
  rg "GOOS=darwin" "$TMP_DIR/macos-buildinfo.txt" >/dev/null
  rg "GOARCH=arm64" "$TMP_DIR/macos-buildinfo.txt" >/dev/null
  rg "path\\s+ccvar.com/web3quant/cmd/ccvar-quant" "$TMP_DIR/windows-buildinfo.txt" >/dev/null
  rg "GOOS=windows" "$TMP_DIR/windows-buildinfo.txt" >/dev/null
  rg "GOARCH=amd64" "$TMP_DIR/windows-buildinfo.txt" >/dev/null
  echo "ok packaged Go build metadata"
fi

node <<'NODE'
const fs = require("fs");
const path = require("path");
const required = [
  "Production/mainnet trading is disabled",
  "Binance Spot Testnet/Demo",
  "OKX Demo Trading",
  "Withdrawal permission is rejected",
];
const i18nFiles = [
  ["src/i18n/zh-CN.js", "简体中文", "交易所密钥库"],
  ["src/i18n/en-US.js", "English", "Exchange Vault"],
  ["src/i18n/index.js", "makeTranslator", "DEFAULT_LOCALE"],
];
for (const [file, ...markers] of i18nFiles) {
  const text = fs.readFileSync(file, "utf8");
  for (const marker of markers) {
    if (!text.includes(marker)) throw new Error(`${file} missing i18n marker "${marker}"`);
  }
}
const embeddedAssets = fs.readdirSync("cmd/ccvar-quant/web/assets").filter((file) => /^index-.*\.js$/.test(file));
if (embeddedAssets.length !== 1) throw new Error(`expected one embedded index js asset, found ${embeddedAssets.length}`);
const embeddedJS = fs.readFileSync(path.join("cmd/ccvar-quant/web/assets", embeddedAssets[0]), "utf8");
for (const marker of ["简体中文", "Exchange Vault", "交易所密钥库"]) {
  if (!embeddedJS.includes(marker)) throw new Error(`embedded UI bundle missing i18n marker "${marker}"`);
}
console.log("ok i18n resources");
const readmes = [
  "dist/desktop/README-macos.txt",
  "dist/desktop/CCVar Quant Lab.app/Contents/Resources/README.txt",
  "dist/desktop/CCVar Quant Lab Windows x64/README.txt",
];
for (const file of readmes) {
  const text = fs.readFileSync(file, "utf8");
  for (const phrase of required) {
    if (!text.includes(phrase)) throw new Error(`${file} missing "${phrase}"`);
  }
  if (!text.includes("127.0.0.1:8787")) throw new Error(`${file} missing local URL`);
  if (!text.includes("CCVAR_ADDR") || !text.includes("CCVAR_DB_PATH")) throw new Error(`${file} missing environment overrides`);
  if (!text.includes("client.log")) throw new Error(`${file} missing log path`);
  if (!text.includes("docs/") && !text.includes("docs\\")) throw new Error(`${file} missing package docs reference`);
}
const windowsLauncher = fs.readFileSync("dist/desktop/CCVar Quant Lab Windows x64/Start CCVar Quant Lab.cmd", "utf8");
for (const phrase of ["LOG_PATH", "client.log", "cmd /c", ">> \"%LOG_PATH%\" 2>&1"]) {
  if (!windowsLauncher.includes(phrase)) throw new Error(`Windows launcher missing "${phrase}"`);
}
const docs = [
  ["dist/desktop/docs/operator-runbook.zh-CN.md", "CCVar Quant Lab 操作手册"],
  ["dist/desktop/docs/safety.md", "Safety Contract"],
  ["dist/desktop/CCVar Quant Lab.app/Contents/Resources/docs/operator-runbook.zh-CN.md", "CCVar Quant Lab 操作手册"],
  ["dist/desktop/CCVar Quant Lab.app/Contents/Resources/docs/safety.md", "Safety Contract"],
  ["dist/desktop/CCVar Quant Lab Windows x64/docs/operator-runbook.zh-CN.md", "CCVar Quant Lab 操作手册"],
  ["dist/desktop/CCVar Quant Lab Windows x64/docs/safety.md", "Safety Contract"],
];
for (const [file, phrase] of docs) {
  const text = fs.readFileSync(file, "utf8");
  if (!text.includes(phrase)) throw new Error(`${file} missing expected document marker`);
}
console.log("ok package readmes");
NODE

go build -o "$ROOT_DIR/bin/ccvar-quant" ./cmd/ccvar-quant

CCVAR_VERIFY_MOCK_ADDR="$MOCK_ADDR" node > "$TMP_DIR/mock-exchange.log" 2>&1 <<'NODE' &
const http = require("http");

const rawAddr = process.env.CCVAR_VERIFY_MOCK_ADDR || "127.0.0.1:8791";
const parsedAddr = new URL(`http://${rawAddr}`);
const host = parsedAddr.hostname.replace(/^\[(.*)\]$/, "$1");
const port = Number(parsedAddr.port || "8791");

function sendJSON(res, status, payload) {
  res.writeHead(status, { "content-type": "application/json" });
  res.end(`${JSON.stringify(payload)}\n`);
}

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
  });
}

function requireHeader(req, res, name) {
  if (!req.headers[name.toLowerCase()]) {
    sendJSON(res, 401, { code: "-1", msg: `missing ${name}` });
    return false;
  }
  return true;
}

function orderPayload(url) {
  const symbol = url.searchParams.get("symbol") || "BTCUSDT";
  const clientOrderId = url.searchParams.get("newClientOrderId") || url.searchParams.get("origClientOrderId") || "ccvar-mock";
  return {
    symbol,
    orderId: 99001,
    clientOrderId,
    status: "FILLED",
    price: "67000.00",
    executedQty: "0.00014925",
    cummulativeQuoteQty: "10.00",
    updateTime: Date.now(),
  };
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host || rawAddr}`);
  if (url.pathname === "/mock-health") {
    sendJSON(res, 200, { ok: true, service: "ccvar-mock-exchange" });
    return;
  }

  if (url.pathname.startsWith("/api/v3/")) {
    if (!requireHeader(req, res, "X-MBX-APIKEY")) return;
    if (req.method === "GET" && url.pathname === "/api/v3/account") {
      sendJSON(res, 200, {
        accountType: "SPOT",
        canTrade: true,
        updateTime: Date.now(),
        balances: [
          { asset: "USDT", free: "1000.00000000", locked: "0.00000000" },
          { asset: "BTC", free: "0.00100000", locked: "0.00000000" },
        ],
      });
      return;
    }
    if (req.method === "GET" && url.pathname === "/api/v3/openOrders") {
      sendJSON(res, 200, []);
      return;
    }
    if (req.method === "POST" && url.pathname === "/api/v3/order/test") {
      await readBody(req);
      sendJSON(res, 200, {});
      return;
    }
    if (req.method === "POST" && url.pathname === "/api/v3/order") {
      await readBody(req);
      sendJSON(res, 200, orderPayload(url));
      return;
    }
    if (req.method === "GET" && url.pathname === "/api/v3/order") {
      sendJSON(res, 200, orderPayload(url));
      return;
    }
  }

  if (url.pathname.startsWith("/api/v5/")) {
    if (!requireHeader(req, res, "OK-ACCESS-KEY")) return;
    if (req.headers["x-simulated-trading"] !== "1") {
      sendJSON(res, 400, { code: "51000", msg: "missing simulated trading header", data: [] });
      return;
    }
    if (req.method === "GET" && url.pathname === "/api/v5/account/balance") {
      sendJSON(res, 200, {
        code: "0",
        msg: "",
        data: [{
          uTime: String(Date.now()),
          details: [
            { ccy: "USDT", availBal: "1000", cashBal: "1000", eq: "1000", eqUsd: "1000" },
            { ccy: "BTC", availBal: "0.001", cashBal: "0.001", eq: "0.001", eqUsd: "67" },
          ],
        }],
      });
      return;
    }
    if (req.method === "GET" && url.pathname === "/api/v5/trade/orders-pending") {
      sendJSON(res, 200, { code: "0", msg: "", data: [] });
      return;
    }
    if (req.method === "POST" && url.pathname === "/api/v5/trade/order") {
      const body = await readBody(req);
      let parsed = {};
      try { parsed = JSON.parse(body); } catch {}
      sendJSON(res, 200, {
        code: "0",
        msg: "",
        data: [{ ordId: "okx-mock-99001", clOrdId: parsed.clOrdId || "ccvar-mock", sCode: "0", sMsg: "" }],
      });
      return;
    }
    if (req.method === "GET" && url.pathname === "/api/v5/trade/order") {
      sendJSON(res, 200, {
        code: "0",
        msg: "",
        data: [{ ordId: "okx-mock-99001", clOrdId: url.searchParams.get("clOrdId") || "ccvar-mock", state: "filled", fillNotionalUsd: "10" }],
      });
      return;
    }
  }

  sendJSON(res, 404, { code: "404", msg: `${req.method} ${url.pathname} not mocked` });
});

server.listen(port, host, () => {
  console.log(`ccvar mock exchange listening on http://${rawAddr}`);
});
NODE
MOCK_PID="$!"

for _ in $(seq 1 60); do
  if curl -fsS "$MOCK_URL/mock-health" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$MOCK_PID" >/dev/null 2>&1; then
    cat "$TMP_DIR/mock-exchange.log" >&2 || true
    echo "mock exchange exited early" >&2
    exit 1
  fi
  sleep 0.25
done
curl -fsS "$MOCK_URL/mock-health" >/dev/null

CCVAR_ENABLE_LOOPBACK_EXCHANGE_MOCKS=true \
CCVAR_BINANCE_PRIVATE_MOCK_URL="$MOCK_URL" \
CCVAR_OKX_PRIVATE_MOCK_URL="$MOCK_URL" \
"$ROOT_DIR/bin/ccvar-quant" --addr "$VERIFY_ADDR" --db "$TMP_DIR/ccvar_quant.db" > "$TMP_DIR/server.log" 2>&1 &
SERVER_PID="$!"

for _ in $(seq 1 60); do
  if curl -fsS "$VERIFY_URL/api/health" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    cat "$TMP_DIR/server.log" >&2 || true
    echo "verify server exited early" >&2
    exit 1
  fi
  sleep 0.25
done

curl -fsS "$VERIFY_URL/api/health" >/dev/null
CCVAR_SMOKE_URL="$VERIFY_URL" npm run smoke:local
CCVAR_ACCEPTANCE_URL="$VERIFY_URL" CCVAR_ACCEPTANCE_REPORT_PATH=none npm run acceptance:live
CCVAR_ACCEPTANCE_URL="$VERIFY_URL" \
CCVAR_ACCEPTANCE_EXCHANGE=Binance \
CCVAR_ACCEPTANCE_API_KEY=mock-binance-key \
CCVAR_ACCEPTANCE_API_SECRET=mock-binance-secret \
CCVAR_ACCEPTANCE_REPORT_PATH="$TMP_DIR/binance-mock-acceptance.json" \
npm run acceptance:live

node - "$TMP_DIR/binance-mock-acceptance.json" <<'NODE'
const fs = require("fs");
const report = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
if (report.result !== "passed") throw new Error(`Binance mock acceptance did not pass: ${report.result}`);
if (report.exchange !== "Binance" || report.environment !== "testnet") throw new Error("Binance mock acceptance target mismatch");
if (!report.artifacts?.accountSnapshot?.snapshotId) throw new Error("Binance mock acceptance missing account snapshot");
if (!report.artifacts?.execution?.ledgerId) throw new Error("Binance mock acceptance missing execution ledger");
if (JSON.stringify(report).match(/mock-binance-secret|ciphertext|nonce|salt|apiSecret|apiPassphrase|passphrase/)) {
  throw new Error("Binance mock acceptance report contains forbidden material");
}
console.log("ok Binance mock live acceptance");
NODE

CCVAR_ACCEPTANCE_URL="$VERIFY_URL" \
CCVAR_ACCEPTANCE_EXCHANGE=OKX \
CCVAR_ACCEPTANCE_API_KEY=mock-okx-key \
CCVAR_ACCEPTANCE_API_SECRET=mock-okx-secret \
CCVAR_ACCEPTANCE_API_PASSPHRASE=mock-okx-passphrase \
CCVAR_ACCEPTANCE_REPORT_PATH="$TMP_DIR/okx-mock-acceptance.json" \
npm run acceptance:live

node - "$TMP_DIR/okx-mock-acceptance.json" <<'NODE'
const fs = require("fs");
const report = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
if (report.result !== "passed") throw new Error(`OKX mock acceptance did not pass: ${report.result}`);
if (report.exchange !== "OKX" || report.environment !== "demo") throw new Error("OKX mock acceptance target mismatch");
if (!report.artifacts?.accountSnapshot?.snapshotId) throw new Error("OKX mock acceptance missing account snapshot");
if (!report.artifacts?.execution?.ledgerId) throw new Error("OKX mock acceptance missing execution ledger");
if (report.artifacts?.execution?.status !== "signed-preflight") throw new Error("OKX mock acceptance should validate signed preflight");
if (JSON.stringify(report).match(/mock-okx-secret|mock-okx-passphrase|ciphertext|nonce|salt|apiSecret|apiPassphrase|passphrase/)) {
  throw new Error("OKX mock acceptance report contains forbidden material");
}
console.log("ok OKX mock live acceptance");
NODE

curl -fsS "$VERIFY_URL/" | rg "CCVar Quant Lab|/assets/index-[^\"]+\\.js|/assets/index-[^\"]+\\.css" >/dev/null

find "$OUT_DIR" -name ".DS_Store" -delete
if find "$OUT_DIR" -name ".DS_Store" -print | grep -q .; then
  echo "release directory contains .DS_Store after final cleanup" >&2
  exit 1
fi
echo "ok final release directory clean"

echo "CCVar release verify passed"
