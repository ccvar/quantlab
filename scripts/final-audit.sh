#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${CCVAR_FINAL_AUDIT_OUT_DIR:-$ROOT_DIR/dist/final-audit}"
REPORT_PATH="${CCVAR_FINAL_AUDIT_REPORT_PATH:-$OUT_DIR/final-audit-latest.json}"
CURRENT_URL="${CCVAR_FINAL_AUDIT_URL:-http://127.0.0.1:8787}"
RUN_REAL_ACCEPTANCE="${CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE:-false}"
TMP_DIR="$(mktemp -d)"
REAL_ACCEPTANCE_RESULTS_PATH="$TMP_DIR/real-acceptance-results.json"
REAL_ACCEPTANCE_READINESS_PATH="$TMP_DIR/real-acceptance-readiness.json"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$(dirname "$REPORT_PATH")" "$OUT_DIR"
printf '[]\n' > "$REAL_ACCEPTANCE_RESULTS_PATH"

echo "CCVar final audit"
echo "report: $REPORT_PATH"

bash -n \
  "$ROOT_DIR/scripts/package-desktop.sh" \
  "$ROOT_DIR/scripts/smoke-local.sh" \
  "$ROOT_DIR/scripts/check-acceptance-env.sh" \
  "$ROOT_DIR/scripts/acceptance-live.sh" \
  "$ROOT_DIR/scripts/verify-release.sh" \
  "$ROOT_DIR/scripts/final-audit.sh" \
  "$ROOT_DIR/scripts/assert-final-complete.sh"
echo "ok shell syntax"

CCVAR_ACCEPTANCE_ENV_CHECK_EXCHANGES="${CCVAR_FINAL_AUDIT_REAL_EXCHANGES:-${CCVAR_ACCEPTANCE_EXCHANGE:-Binance,OKX}}" \
CCVAR_ACCEPTANCE_ENV_CHECK_REPORT_PATH="$REAL_ACCEPTANCE_READINESS_PATH" \
CCVAR_ACCEPTANCE_ENV_CHECK_REQUIRE_READY="$RUN_REAL_ACCEPTANCE" \
bash "$ROOT_DIR/scripts/check-acceptance-env.sh" >/dev/null
echo "ok acceptance env readiness"

npm run verify:release
echo "ok release verification"

CURRENT_SMOKE_STATUS="skipped_not_running"
CURRENT_APP_INFO_PATH=""
if curl -fsS "$CURRENT_URL/api/health" -o "$TMP_DIR/current-health.json" >/dev/null 2>&1; then
  CCVAR_SMOKE_URL="$CURRENT_URL" npm run smoke:local
  curl -fsS "$CURRENT_URL/api/app-info" -o "$TMP_DIR/current-app-info.json"
  CURRENT_SMOKE_STATUS="passed"
  CURRENT_APP_INFO_PATH="$TMP_DIR/current-app-info.json"
  echo "ok current instance smoke"
else
  echo "skip current instance smoke: $CURRENT_URL is not serving CCVar Quant"
fi

append_real_acceptance_result() {
  local exchange="$1"
  local status="$2"
  local report_path="$3"
  local note="$4"
  RESULTS_PATH="$REAL_ACCEPTANCE_RESULTS_PATH" \
  RESULT_EXCHANGE="$exchange" \
  RESULT_STATUS="$status" \
  RESULT_REPORT_PATH="$report_path" \
  RESULT_NOTE="$note" \
  node <<'NODE'
const fs = require("fs");
const path = process.env.RESULTS_PATH;
const results = JSON.parse(fs.readFileSync(path, "utf8"));
results.push({
  exchange: process.env.RESULT_EXCHANGE || null,
  status: process.env.RESULT_STATUS,
  reportPath: process.env.RESULT_REPORT_PATH || null,
  note: process.env.RESULT_NOTE || "",
});
fs.writeFileSync(path, `${JSON.stringify(results, null, 2)}\n`);
NODE
}

normalize_exchange() {
  local value
  value="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
  case "$value" in
    binance) printf 'Binance' ;;
    okx) printf 'OKX' ;;
    *) return 1 ;;
  esac
}

if [ "$RUN_REAL_ACCEPTANCE" = "true" ]; then
  real_acceptance_requested="${CCVAR_FINAL_AUDIT_REAL_EXCHANGES:-${CCVAR_ACCEPTANCE_EXCHANGE:-Binance,OKX}}"
  if [ -z "$real_acceptance_requested" ]; then
    echo "CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true requires CCVAR_FINAL_AUDIT_REAL_EXCHANGES=Binance,OKX or CCVAR_ACCEPTANCE_EXCHANGE=Binance/OKX" >&2
    exit 1
  fi

  IFS=',' read -r -a raw_real_acceptance_exchanges <<< "$real_acceptance_requested"
  real_acceptance_exchanges=()
  for raw_exchange in "${raw_real_acceptance_exchanges[@]}"; do
    if [ -z "$(printf '%s' "$raw_exchange" | tr -d '[:space:]')" ]; then
      continue
    fi
    if ! exchange="$(normalize_exchange "$raw_exchange")"; then
      echo "invalid final audit exchange: $raw_exchange (expected Binance or OKX)" >&2
      exit 1
    fi
    real_acceptance_exchanges+=("$exchange")
  done
  if [ "${#real_acceptance_exchanges[@]}" -eq 0 ]; then
    echo "no real sandbox exchanges requested" >&2
    exit 1
  fi

  if [ "${#real_acceptance_exchanges[@]}" -gt 1 ]; then
    for generic_secret_var in CCVAR_ACCEPTANCE_API_KEY CCVAR_ACCEPTANCE_API_SECRET CCVAR_ACCEPTANCE_SECRET CCVAR_ACCEPTANCE_API_PASSPHRASE; do
      if [ -n "${!generic_secret_var:-}" ]; then
        echo "multi-exchange final audit requires exchange-specific credential variables; unset $generic_secret_var" >&2
        exit 1
      fi
    done
  fi

  for exchange in "${real_acceptance_exchanges[@]}"; do
    report_exchange="$(printf '%s' "$exchange" | tr '[:upper:]' '[:lower:]')"
    if [ "${#real_acceptance_exchanges[@]}" -eq 1 ] && [ -n "${CCVAR_ACCEPTANCE_REPORT_PATH:-}" ]; then
      exchange_report_path="$CCVAR_ACCEPTANCE_REPORT_PATH"
    else
      exchange_report_path="$OUT_DIR/live-acceptance-${report_exchange}.json"
    fi
    (
      export CCVAR_ACCEPTANCE_EXCHANGE="$exchange"
      export CCVAR_ACCEPTANCE_URL="${CCVAR_ACCEPTANCE_URL:-$CURRENT_URL}"
      export CCVAR_ACCEPTANCE_REPORT_PATH="$exchange_report_path"
      if [ "$exchange" = "OKX" ]; then
        export CCVAR_ACCEPTANCE_ENVIRONMENT="${CCVAR_ACCEPTANCE_OKX_ENVIRONMENT:-demo}"
      else
        export CCVAR_ACCEPTANCE_ENVIRONMENT="${CCVAR_ACCEPTANCE_BINANCE_ENVIRONMENT:-${CCVAR_ACCEPTANCE_ENVIRONMENT:-testnet}}"
      fi
      npm run acceptance:live
    )
    append_real_acceptance_result "$exchange" "passed" "$exchange_report_path" "Operator-provided sandbox acceptance completed."
    echo "ok real sandbox acceptance ${exchange}"
  done
else
  CCVAR_ACCEPTANCE_REPORT_PATH=none npm run acceptance:live
  echo "ok real sandbox acceptance skipped by default"
fi

ROOT_DIR="$ROOT_DIR" \
REPORT_PATH="$REPORT_PATH" \
CURRENT_URL="$CURRENT_URL" \
CURRENT_SMOKE_STATUS="$CURRENT_SMOKE_STATUS" \
CURRENT_APP_INFO_PATH="$CURRENT_APP_INFO_PATH" \
REAL_ACCEPTANCE_RESULTS_PATH="$REAL_ACCEPTANCE_RESULTS_PATH" \
REAL_ACCEPTANCE_READINESS_PATH="$REAL_ACCEPTANCE_READINESS_PATH" \
node <<'NODE'
const fs = require("fs");
const crypto = require("crypto");
const path = require("path");

const root = process.env.ROOT_DIR;
const reportPath = process.env.REPORT_PATH;
const manifestPath = path.join(root, "dist/desktop/release-manifest.json");

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function fileInfo(relativePath) {
  const absolutePath = path.join(root, relativePath);
  const stat = fs.statSync(absolutePath);
  return {
    path: relativePath,
    sizeBytes: stat.size,
    sha256: sha256(absolutePath),
  };
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

const manifest = readJSON(manifestPath);
assert(manifest.product === "CCVar Quant Lab", "release manifest product mismatch");
assert(manifest.version === "0.1.0", "release manifest version mismatch");
assert(manifest.manifestVersion === 1, "release manifest version marker mismatch");
assert(manifest.verification?.noProductionTrading === true, "release manifest no-production flag missing");
assert(Array.isArray(manifest.artifacts) && manifest.artifacts.length === 2, "release manifest artifact list mismatch");
assert(Array.isArray(manifest.criticalFiles) && manifest.criticalFiles.length >= 12, "release manifest critical file list incomplete");

for (const artifact of manifest.artifacts) {
  const file = path.join(root, "dist/desktop", artifact.file);
  assert(fs.existsSync(file), `missing artifact ${artifact.file}`);
  assert(fs.statSync(file).size === artifact.sizeBytes, `artifact size mismatch ${artifact.file}`);
  assert(sha256(file) === artifact.sha256, `artifact hash mismatch ${artifact.file}`);
}

const sourceDocs = [
  fileInfo("docs/operator-runbook.zh-CN.md"),
  fileInfo("docs/safety.md"),
];
const qaEvidence = [
  "qa-app-info-docs.png",
  "qa-app-info-docs-mobile.png",
  "qa-comparison.png",
  "qa-live-setup-checklist.png",
  "qa-vault-connection-test.png",
].filter((relativePath) => fs.existsSync(path.join(root, relativePath))).map(fileInfo);

assert(sourceDocs.every((doc) => doc.sizeBytes > 0), "source docs missing");
assert(qaEvidence.length >= 2, "QA screenshot evidence missing");

let currentInstance = {
  url: process.env.CURRENT_URL,
  smokeStatus: process.env.CURRENT_SMOKE_STATUS,
};
if (process.env.CURRENT_APP_INFO_PATH) {
  const appInfo = readJSON(process.env.CURRENT_APP_INFO_PATH);
  currentInstance = {
    ...currentInstance,
    service: appInfo.service,
    version: appInfo.version,
    runtime: appInfo.runtime,
    docsAvailable: appInfo.docs?.available === true,
    localOriginOnly: appInfo.security?.localOriginOnly === true,
    productionTradingEnabled: appInfo.security?.productionTradingEnabled === true,
    productionAccountSyncEnabled: appInfo.security?.productionAccountSyncEnabled === true,
  };
  assert(currentInstance.service === "ccvar-quant", "current app-info service mismatch");
  assert(currentInstance.localOriginOnly, "current app local origin guard disabled");
  assert(!currentInstance.productionTradingEnabled, "current app reports production trading enabled");
  assert(!currentInstance.productionAccountSyncEnabled, "current app reports production account sync enabled");
  assert(currentInstance.docsAvailable, "current app docs unavailable");
}

const realSandboxResults = fs.existsSync(process.env.REAL_ACCEPTANCE_RESULTS_PATH || "")
  ? readJSON(process.env.REAL_ACCEPTANCE_RESULTS_PATH)
  : [];
const realSandboxCredentialReadiness = fs.existsSync(process.env.REAL_ACCEPTANCE_READINESS_PATH || "")
  ? readJSON(process.env.REAL_ACCEPTANCE_READINESS_PATH)
  : null;
const requiredRealSandboxExchanges = ["Binance", "OKX"];
const passedRealSandboxExchanges = new Set(
  realSandboxResults.filter((item) => item.status === "passed").map((item) => item.exchange),
);
const realSandboxAcceptance = {
  status:
    realSandboxResults.length === 0
      ? "not_run"
      : requiredRealSandboxExchanges.every((exchange) => passedRealSandboxExchanges.has(exchange))
        ? "passed"
        : "partial",
  requiredExchanges: requiredRealSandboxExchanges,
  results: realSandboxResults,
  missingExchanges: requiredRealSandboxExchanges.filter((exchange) => !passedRealSandboxExchanges.has(exchange)),
  note:
    realSandboxResults.length > 0
      ? "Operator-provided sandbox acceptance results are listed per exchange."
      : "Not run by default. Set CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true plus CCVAR_FINAL_AUDIT_REAL_EXCHANGES=Binance,OKX and exchange-specific sandbox environment variables to collect this external proof.",
};

const report = {
  generatedAt: new Date().toISOString(),
  product: manifest.product,
  version: manifest.version,
  commands: {
    shellSyntax: "passed",
    verifyRelease: "passed",
    currentInstanceSmoke: currentInstance.smokeStatus,
  },
  release: {
    manifestVersion: manifest.manifestVersion,
    generatedAt: manifest.generatedAt,
    verification: manifest.verification,
    artifacts: manifest.artifacts,
    criticalFileCount: manifest.criticalFiles.length,
    checksumFile: fileInfo("dist/desktop/SHA256SUMS.txt"),
  },
  docs: {
    source: sourceDocs,
    packagedCriticalFiles: manifest.criticalFiles.filter((item) => item.path.includes("docs/")),
  },
  qaEvidence,
  currentInstance,
  realSandboxCredentialReadiness,
  realSandboxAcceptance,
};

const text = JSON.stringify(report, null, 2);
for (const pattern of [/apiKey/i, /apiSecret/i, /"secret"/i, /apiPassphrase/i, /"passphrase"/i, /ciphertext/i, /"salt"/i, /"nonce"/i]) {
  if (pattern.test(text)) {
    throw new Error(`final audit report contains forbidden material: ${pattern}`);
  }
}

fs.mkdirSync(path.dirname(reportPath), { recursive: true });
fs.writeFileSync(reportPath, `${text}\n`);
console.log(`ok final audit report ${reportPath}`);
NODE

echo "CCVar final audit passed"
