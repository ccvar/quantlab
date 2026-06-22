#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CCVAR_SMOKE_URL:-http://127.0.0.1:8787}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fetch_json() {
  local path="$1"
  local out="$2"
  curl -fsS -H 'Accept: application/json' "${BASE_URL}${path}" -o "${TMP_DIR}/${out}"
}

echo "CCVar local smoke: ${BASE_URL}"

fetch_json "/api/health" "health.json"
node - "${TMP_DIR}/health.json" <<'NODE'
const fs = require("fs");
const payload = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
if (payload.service !== "ccvar-quant" || payload.ok !== true) {
  throw new Error(`health check failed: ${JSON.stringify(payload)}`);
}
console.log(`ok health ${payload.version || ""}`.trim());
NODE

fetch_json "/api/app-info" "app-info.json"
node - "${TMP_DIR}/app-info.json" <<'NODE'
const fs = require("fs");
const payload = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
if (payload.service !== "ccvar-quant" || !payload.version) {
  throw new Error(`app-info missing service/version: ${JSON.stringify(payload)}`);
}
if (payload.security?.productionTradingEnabled || payload.security?.productionAccountSyncEnabled) {
  throw new Error("app-info reports production trading/account sync enabled");
}
if (!payload.security?.localOriginOnly) {
  throw new Error("app-info reports local origin guard disabled");
}
if (!payload.docs?.runbook?.exists || !payload.docs?.safety?.exists) {
  throw new Error(`app-info docs are unavailable: ${JSON.stringify(payload.docs)}`);
}
const text = JSON.stringify(payload);
for (const pattern of [/apiSecret/i, /"secret"/i, /apiPassphrase/i, /"passphrase"/i, /ciphertext/i, /"salt"/i, /"nonce"/i]) {
  if (pattern.test(text)) {
    throw new Error(`app-info exposed forbidden material: ${pattern}`);
  }
}
console.log(`ok app-info docs ${payload.runtime?.goos || "-"}/${payload.runtime?.goarch || "-"}`);
NODE

curl -fsS "${BASE_URL}/" -o "${TMP_DIR}/index.html"
node - "${TMP_DIR}/index.html" <<'NODE'
const fs = require("fs");
const html = fs.readFileSync(process.argv[2], "utf8");
if (!html.includes("CCVar Quant Lab")) {
  throw new Error("index page is missing app title");
}
if (!/\/assets\/index-[^"]+\.js/.test(html) || !/\/assets\/index-[^"]+\.css/.test(html)) {
  throw new Error("index page is missing built asset links");
}
console.log("ok embedded ui assets");
NODE

node - "${TMP_DIR}/index.html" "${TMP_DIR}/asset-path.txt" <<'NODE'
const fs = require("fs");
const html = fs.readFileSync(process.argv[2], "utf8");
const outputPath = process.argv[3];
const match = html.match(/\/assets\/index-[^"]+\.js/);
if (!match) throw new Error("index page is missing built js asset");
fs.writeFileSync(outputPath, match[0]);
NODE
asset_path="$(cat "${TMP_DIR}/asset-path.txt")"
curl -fsS "${BASE_URL}${asset_path}" -o "${TMP_DIR}/ui.js"
node - "${TMP_DIR}/ui.js" <<'NODE'
const fs = require("fs");
const js = fs.readFileSync(process.argv[2], "utf8");
for (const marker of ["简体中文", "English", "交易所密钥库", "Exchange Vault"]) {
  if (!js.includes(marker)) throw new Error(`embedded UI asset missing i18n marker "${marker}"`);
}
console.log("ok embedded i18n resources");
NODE

fetch_json "/api/preflight" "preflight.json"
node - "${TMP_DIR}/preflight.json" <<'NODE'
const fs = require("fs");
const payload = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const checks = Array.isArray(payload.checks) ? payload.checks : [];
const live = checks.find((check) => check.id === "live_autopilot");
if (!live) {
  throw new Error("preflight is missing live_autopilot check");
}
if (Number(payload.block || 0) > 0 || live.status === "block") {
  throw new Error(`preflight has blocking checks: ${JSON.stringify({ overall: payload.overall, block: payload.block, live })}`);
}
console.log(`ok preflight ${payload.ready || 0}/${payload.warn || 0}/${payload.block || 0} live=${live.status}`);
NODE

fetch_json "/api/credentials" "credentials.json"
node - "${TMP_DIR}/credentials.json" <<'NODE'
const fs = require("fs");
const payload = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const text = JSON.stringify(payload);
for (const pattern of [/apiSecret/i, /"secret"/i, /apiPassphrase/i, /"passphrase"/i, /ciphertext/i, /"salt"/i, /"nonce"/i]) {
  if (pattern.test(text)) {
    throw new Error(`credential list exposed forbidden material: ${pattern}`);
  }
}
console.log(`ok credential list ${Array.isArray(payload.credentials) ? payload.credentials.length : 0}`);
NODE

curl -fsS -H 'Accept: application/json' -D "${TMP_DIR}/export.headers" "${BASE_URL}/api/export" -o "${TMP_DIR}/export.json"
node - "${TMP_DIR}/export.json" "${TMP_DIR}/export.headers" <<'NODE'
const fs = require("fs");
const payload = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const headers = fs.readFileSync(process.argv[3], "utf8").toLowerCase();
if (!headers.includes("cache-control: no-store")) {
  throw new Error("workspace export is missing Cache-Control: no-store");
}
const forbiddenKeys = new Set(["apiKey", "apiSecret", "secret", "apiPassphrase", "passphrase", "ciphertext", "salt", "nonce"]);
function walk(value, path = []) {
  if (Array.isArray(value)) {
    value.forEach((item, index) => walk(item, path.concat(String(index))));
    return;
  }
  if (!value || typeof value !== "object") return;
  for (const [key, child] of Object.entries(value)) {
    if (forbiddenKeys.has(key)) {
      throw new Error(`workspace export exposed forbidden key at ${path.concat(key).join(".")}`);
    }
    walk(child, path.concat(key));
  }
}
walk(payload);
console.log("ok sanitized workspace export");
NODE

echo "CCVar local smoke passed"
