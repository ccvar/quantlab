#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_DIR="${CCVAR_COMPLETION_ROOT_DIR:-$SCRIPT_ROOT}"
REPORT_PATH="${1:-${CCVAR_FINAL_AUDIT_REPORT_PATH:-$ROOT_DIR/dist/final-audit/final-audit-latest.json}}"

node - "$ROOT_DIR" "$REPORT_PATH" <<'NODE'
const fs = require("fs");
const crypto = require("crypto");
const path = require("path");

const root = process.argv[2];
const reportPath = process.argv[3];
const requiredExchanges = ["Binance", "OKX"];
const forbiddenPatterns = [/apiKey/i, /apiSecret/i, /"secret"/i, /apiPassphrase/i, /"passphrase"/i, /ciphertext/i, /"salt"/i, /"nonce"/i];

function fail(message, details) {
  if (details) {
    console.error(`CCVar final completion gate failed: ${message}`);
    console.error(JSON.stringify(details, null, 2));
  } else {
    console.error(`CCVar final completion gate failed: ${message}`);
  }
  process.exit(1);
}

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function assertNoForbiddenMaterial(label, value) {
  const text = typeof value === "string" ? value : JSON.stringify(value);
  for (const pattern of forbiddenPatterns) {
    if (pattern.test(text)) fail(`${label} contains forbidden material`, { pattern: String(pattern) });
  }
}

if (!fs.existsSync(reportPath)) {
  fail("final audit report is missing; run npm run audit:final first", { reportPath });
}

const reportText = fs.readFileSync(reportPath, "utf8");
assertNoForbiddenMaterial("final audit report", reportText);
const report = JSON.parse(reportText);

if (report.product !== "CCVar Quant Lab" || report.version !== "0.1.0") {
  fail("final audit report product/version mismatch", { product: report.product, version: report.version });
}

const commands = report.commands || {};
for (const [name, status] of Object.entries({
  shellSyntax: "passed",
  verifyRelease: "passed",
  currentInstanceSmoke: "passed",
})) {
  if (commands[name] !== status) {
    fail(`final audit command gate ${name} is not ${status}`, { commands });
  }
}

if (report.release?.verification?.noProductionTrading !== true) {
  fail("release verification did not prove production trading is disabled", report.release?.verification);
}

if (!Array.isArray(report.release?.artifacts) || report.release.artifacts.length !== 2) {
  fail("release artifact list must contain macOS and Windows zip files", report.release?.artifacts);
}
for (const artifact of report.release.artifacts) {
  const artifactPath = path.join(root, "dist/desktop", artifact.file || "");
  if (!fs.existsSync(artifactPath)) fail(`release artifact is missing: ${artifact.file}`);
  const stat = fs.statSync(artifactPath);
  const digest = sha256(artifactPath);
  if (stat.size !== artifact.sizeBytes || digest !== artifact.sha256) {
    fail(`release artifact does not match final audit report: ${artifact.file}`, {
      expected: artifact,
      actual: { sizeBytes: stat.size, sha256: digest },
    });
  }
}

if (!Array.isArray(report.docs?.source) || report.docs.source.length < 2) {
  fail("final audit report is missing source document evidence", report.docs);
}
if (!Array.isArray(report.docs?.packagedCriticalFiles) || report.docs.packagedCriticalFiles.length < 4) {
  fail("final audit report is missing packaged document evidence", report.docs);
}

const readiness = report.realSandboxCredentialReadiness;
if (!readiness || readiness.ready !== true) {
  fail("real Binance/OKX sandbox credential readiness is incomplete", {
    credentialReadiness: readiness || null,
    nextCommand: "npm run acceptance:env",
  });
}
if ((readiness.genericCredentialVariablesPresent || []).length > 0 || readiness.genericCredentialVariablesAllowed !== false) {
  fail("real sandbox readiness is not using exchange-specific credential variables", {
    genericCredentialVariablesAllowed: readiness.genericCredentialVariablesAllowed,
    genericCredentialVariablesPresent: readiness.genericCredentialVariablesPresent || [],
  });
}
const readinessByExchange = new Map((readiness.results || []).map((result) => [result.exchange, result]));
for (const exchange of requiredExchanges) {
  const result = readinessByExchange.get(exchange);
  if (!result || result.ready !== true) {
    fail(`${exchange} sandbox credential readiness is incomplete`, result || { exchange, ready: false });
  }
}

const real = report.realSandboxAcceptance || {};
const resultByExchange = new Map((real.results || []).map((result) => [result.exchange, result]));
const missing = [];
for (const exchange of requiredExchanges) {
  const result = resultByExchange.get(exchange);
  if (!result || result.status !== "passed") missing.push(exchange);
}
if (real.status !== "passed" || missing.length > 0 || (real.missingExchanges || []).length > 0) {
  fail("real Binance/OKX sandbox acceptance is incomplete", {
    status: real.status || "missing",
    requiredExchanges,
    missingExchanges: missing.length > 0 ? missing : real.missingExchanges || [],
    credentialReadiness: report.realSandboxCredentialReadiness || null,
    nextCommand:
      "CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true CCVAR_FINAL_AUDIT_REAL_EXCHANGES=Binance,OKX BINANCE_TESTNET_API_KEY=... BINANCE_TESTNET_API_SECRET=... OKX_DEMO_API_KEY=... OKX_DEMO_SECRET=... OKX_DEMO_API_PASSPHRASE=... npm run audit:final",
  });
}

for (const exchange of requiredExchanges) {
  const result = resultByExchange.get(exchange);
  if (!result?.reportPath) fail(`${exchange} sandbox acceptance is missing a report path`, result);
  if (!fs.existsSync(result.reportPath)) fail(`${exchange} sandbox acceptance report is missing`, result);
  const acceptanceText = fs.readFileSync(result.reportPath, "utf8");
  assertNoForbiddenMaterial(`${exchange} acceptance report`, acceptanceText);
  const acceptanceReport = JSON.parse(acceptanceText);
  if (acceptanceReport.result !== "passed" || acceptanceReport.exchange !== exchange) {
    fail(`${exchange} acceptance report did not pass`, {
      result: acceptanceReport.result,
      exchange: acceptanceReport.exchange,
      reportPath: result.reportPath,
    });
  }
}

console.log("CCVar final completion gate passed");
NODE
