#!/usr/bin/env bash
set -euo pipefail

node <<'NODE'
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");

const env = process.env;
const baseURL = (env.CCVAR_ACCEPTANCE_URL || "http://127.0.0.1:8787").replace(/\/+$/, "");
const rawExchange = env.CCVAR_ACCEPTANCE_EXCHANGE || "";
const exchange = canonicalExchange(rawExchange);

if (rawExchange.trim() && !exchange) {
  console.error("CCVar live acceptance failed: CCVAR_ACCEPTANCE_EXCHANGE must be Binance or OKX.");
  process.exit(1);
}
if (!exchange) {
  console.log("CCVar live acceptance skipped: set CCVAR_ACCEPTANCE_EXCHANGE=Binance or OKX to run with testnet/demo credentials.");
  process.exit(0);
}

const environment = normalizeEnvironment(
  env.CCVAR_ACCEPTANCE_ENVIRONMENT || (exchange === "OKX" ? "demo" : "testnet"),
);
const symbol = (env.CCVAR_ACCEPTANCE_SYMBOL || "BTCUSDT").trim().toUpperCase();
const side = (env.CCVAR_ACCEPTANCE_SIDE || "buy").trim().toLowerCase();
const sizeUsdt = Number(env.CCVAR_ACCEPTANCE_SIZE_USDT || "10");
const validationOnly = parseBool(env.CCVAR_ACCEPTANCE_VALIDATION_ONLY, true);
const keepCredential = parseBool(env.CCVAR_ACCEPTANCE_KEEP_CREDENTIAL, false);
const keepGuard = parseBool(env.CCVAR_ACCEPTANCE_KEEP_GUARD, false);
const allowDemoSubmit = parseBool(env.CCVAR_ACCEPTANCE_ALLOW_DEMO_SUBMIT, false);
const reportPath = env.CCVAR_ACCEPTANCE_REPORT_PATH || "dist/acceptance/live-acceptance-latest.json";
const operator = env.CCVAR_ACCEPTANCE_OPERATOR || "acceptance";
const label = env.CCVAR_ACCEPTANCE_LABEL || `acceptance ${exchange} ${new Date().toISOString()}`;
const passphrase = env.CCVAR_ACCEPTANCE_VAULT_PASSPHRASE || `ccvar-acceptance-${crypto.randomBytes(12).toString("hex")}`;
const apiKey = credentialValue("API_KEY");
const secret = credentialValue("API_SECRET") || credentialValue("SECRET");
const apiPassphrase = credentialValue("API_PASSPHRASE");
const report = {
  reportVersion: 1,
  startedAt: new Date().toISOString(),
  finishedAt: "",
  result: "running",
  baseURL,
  exchange,
  environment,
  symbol,
  side,
  requestedSizeUsdt: sizeUsdt,
  validationOnly,
  demoSubmitAllowed: allowDemoSubmit,
  keepCredential,
  keepGuard,
  steps: [],
  cleanup: [],
  artifacts: {},
};

let createdCredentialId = 0;
let unlockedGuard = false;
let cleanupStarted = false;
let reportWritten = false;
let signalHandling = false;

main().catch((error) => {
  report.result = "failed";
  report.error = error.message;
  recordStep("failure", "failed", { message: error.message });
  console.error(`CCVar live acceptance failed: ${error.message}`);
  process.exitCode = 1;
});

async function main() {
  validateInputs();
  console.log(`CCVar live acceptance: ${exchange} ${environment} ${symbol} ${validationOnly ? "validation-only" : "demo/testnet submit"}`);

  const health = await requestJSON("/api/health");
  assert(health.service === "ccvar-quant" && health.ok === true, "health check failed");
  report.artifacts.health = {
    service: health.service,
    version: health.version || "",
  };
  recordStep("health", "ok", report.artifacts.health);
  console.log(`ok health ${health.version || ""}`.trim());

  const credential = await requestJSON("/api/credentials", {
    method: "POST",
    body: {
      exchange,
      label,
      apiKey,
      secret,
      apiPassphrase: exchange === "OKX" ? apiPassphrase : apiPassphrase || "",
      passphrase,
      permissions: { read: true, trade: true, withdraw: false },
    },
  });
  createdCredentialId = Number(credential.id || 0);
  assert(createdCredentialId > 0, "credential save did not return an id");
  report.artifacts.credential = {
    id: createdCredentialId,
    exchange: credential.exchange,
    apiKeyMask: credential.apiKeyMask || "",
  };
  recordStep("credential_save", "ok", report.artifacts.credential);
  console.log(`ok credential saved #${createdCredentialId} ${credential.exchange} ${credential.apiKeyMask || ""}`.trim());

  await assertCredentialRedaction(createdCredentialId);

  const snapshotResult = await requestJSON("/api/account-sync", {
    method: "POST",
    body: {
      operator,
      credentialId: createdCredentialId,
      passphrase,
      exchange,
      environment,
      symbol,
    },
  });
  assert(snapshotResult.snapshotId, "account sync did not persist a snapshot id");
  assert(snapshotResult.snapshot && snapshotResult.snapshot.environment === environment, "account sync returned unexpected environment");
  assert(snapshotResult.snapshot.canTrade === true, "account snapshot indicates trading is disabled; enable testnet/demo trade permission before live acceptance");
  report.artifacts.accountSnapshot = {
    snapshotId: snapshotResult.snapshotId,
    canTrade: Boolean(snapshotResult.snapshot.canTrade),
    balances: snapshotResult.snapshot.balances?.length || 0,
    openOrders: snapshotResult.snapshot.openOrders?.length || 0,
    syncedAt: snapshotResult.snapshot.syncedAt || "",
  };
  recordStep("account_sync", "ok", report.artifacts.accountSnapshot);
  console.log(
    `ok account sync #${snapshotResult.snapshotId} canTrade=${Boolean(snapshotResult.snapshot.canTrade)} balances=${snapshotResult.snapshot.balances?.length || 0} openOrders=${snapshotResult.snapshot.openOrders?.length || 0}`,
  );

  const guard = await requestJSON("/api/live-guard", {
    method: "POST",
    body: {
      action: "unlock",
      operator,
      environment,
      phrase: "ENABLE TESTNET LIVE",
      ttlSeconds: 300,
      maxOrderUsdt: Math.max(sizeUsdt * 2, 100),
      reason: "testnet/demo acceptance validation",
    },
  });
  unlockedGuard = Boolean(guard.unlocked);
  assert(unlockedGuard && guard.environment === environment, "live guard did not unlock expected environment");
  report.artifacts.liveGuard = {
    unlocked: Boolean(guard.unlocked),
    environment: guard.environment,
    ttlSeconds: guard.ttlSeconds || 0,
    maxOrderUsdt: guard.maxOrderUsdt || 0,
  };
  recordStep("live_guard_unlock", "ok", report.artifacts.liveGuard);
  console.log(`ok live guard ${guard.environment} ttl=${guard.ttlSeconds || 0}`);

  const preflight = await requestJSON("/api/preflight");
  const liveCheck = (preflight.checks || []).find((check) => check.id === "live_autopilot");
  assert(Number(preflight.block || 0) === 0 && liveCheck?.status !== "block", "preflight has blocking checks");
  report.artifacts.preflight = {
    overall: preflight.overall || "",
    ready: preflight.ready || 0,
    warn: preflight.warn || 0,
    block: preflight.block || 0,
    liveAutopilot: liveCheck?.status || "",
  };
  recordStep("preflight", "ok", report.artifacts.preflight);
  console.log(`ok preflight ${preflight.ready || 0}/${preflight.warn || 0}/${preflight.block || 0} live=${liveCheck?.status || "-"}`);

  const execution = await requestJSON("/api/live-execute", {
    method: "POST",
    body: {
      operator,
      credentialId: createdCredentialId,
      passphrase,
      exchange,
      symbol,
      side,
      sizeUsdt,
      validationOnly,
    },
  });
  assert(execution.ledgerId, "live execute did not persist a ledger row");
  assert(execution.decision?.approved === true, "live execute was not risk-approved");
  assert(execution.execution?.status, "live execute missing exchange execution status");
  report.artifacts.execution = {
    ledgerId: execution.ledgerId,
    riskApproved: Boolean(execution.decision?.approved),
    status: execution.execution.status,
    endpoint: execution.execution.endpoint,
    validationOnly: Boolean(execution.execution.validationOnly),
    clientOrderId: execution.execution.clientOrderId || execution.intent?.id || "",
  };
  recordStep("live_execute", "ok", report.artifacts.execution);
  console.log(`ok live execute ledger #${execution.ledgerId} status=${execution.execution.status} endpoint=${execution.execution.endpoint}`);

  const ledger = await requestJSON("/api/live-executions?limit=6");
  assert((ledger.records || []).some((record) => Number(record.id) === Number(execution.ledgerId)), "execution ledger row was not returned");
  recordStep("execution_ledger", "ok", { ledgerId: execution.ledgerId, returnedRecords: (ledger.records || []).length });
  console.log("ok execution ledger");

  if (!validationOnly) {
    const reconciliation = await requestJSON("/api/live-reconcile", {
      method: "POST",
      body: {
        operator,
        liveExecutionId: execution.ledgerId,
        passphrase,
      },
    });
    assert(reconciliation.reconciliation?.id, "reconciliation was not persisted");
    report.artifacts.reconciliation = {
      id: reconciliation.reconciliation.id,
      status: reconciliation.report?.status || "",
      filledUsdt: reconciliation.report?.filledUsdt || 0,
    };
    recordStep("reconciliation", "ok", report.artifacts.reconciliation);
    console.log(`ok reconciliation #${reconciliation.reconciliation.id} status=${reconciliation.report?.status || "-"}`);
  } else {
    recordStep("reconciliation", "skipped", { reason: "validation-only execution" });
    console.log("ok reconciliation skipped for validation-only execution");
  }

  await assertWorkspaceExportRedaction();

  const audit = await requestJSON("/api/audit-log?limit=12");
  assert(audit.verification?.valid !== false, "audit verification is invalid");
  report.artifacts.audit = {
    valid: audit.verification?.valid !== false,
    checked: audit.verification?.checked || 0,
    entries: (audit.entries || []).length,
  };
  recordStep("audit", "ok", report.artifacts.audit);
  console.log(`ok audit hash checked=${audit.verification?.checked || 0}`);
  report.result = "passed";
}

async function cleanup() {
  if (cleanupStarted) return;
  cleanupStarted = true;
  if (unlockedGuard && !keepGuard) {
    try {
      await requestJSON("/api/live-guard", {
        method: "POST",
        body: { action: "lock", operator, reason: "acceptance cleanup" },
      });
      report.cleanup.push({ action: "live_guard.lock", status: "ok" });
      console.log("ok live guard locked");
    } catch (error) {
      report.cleanup.push({ action: "live_guard.lock", status: "failed", message: error.message });
      console.error(`warn live guard cleanup failed: ${error.message}`);
    }
  } else if (unlockedGuard && keepGuard) {
    report.cleanup.push({ action: "live_guard.lock", status: "skipped", reason: "CCVAR_ACCEPTANCE_KEEP_GUARD=true" });
  }
  if (createdCredentialId > 0 && !keepCredential) {
    try {
      await requestJSON(`/api/credentials?id=${encodeURIComponent(String(createdCredentialId))}`, { method: "DELETE" });
      report.cleanup.push({ action: "credential.delete", status: "ok", credentialId: createdCredentialId });
      console.log(`ok credential deleted #${createdCredentialId}`);
    } catch (error) {
      report.cleanup.push({ action: "credential.delete", status: "failed", credentialId: createdCredentialId, message: error.message });
      console.error(`warn credential cleanup failed: ${error.message}`);
    }
  } else if (createdCredentialId > 0 && keepCredential) {
    report.cleanup.push({ action: "credential.delete", status: "skipped", credentialId: createdCredentialId, reason: "CCVAR_ACCEPTANCE_KEEP_CREDENTIAL=true" });
  }
  writeReport();
}

process.on("beforeExit", async () => {
  await cleanup();
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, async () => {
    if (signalHandling) return;
    signalHandling = true;
    report.result = "interrupted";
    report.error = `received ${signal}`;
    recordStep("signal", "failed", { signal });
    await cleanup();
    process.exit(signal === "SIGINT" ? 130 : 143);
  });
}

function validateInputs() {
  assert(environment === "testnet" || environment === "demo", "CCVAR_ACCEPTANCE_ENVIRONMENT must be testnet or demo");
  assert(exchange !== "OKX" || environment === "demo", "OKX acceptance supports demo only");
  assert(side === "buy" || side === "sell", "CCVAR_ACCEPTANCE_SIDE must be buy or sell");
  assert(Number.isFinite(sizeUsdt) && sizeUsdt > 0, "CCVAR_ACCEPTANCE_SIZE_USDT must be positive");
  assert(apiKey, missingMessage("api key"));
  assert(secret, missingMessage("api secret"));
  assert(exchange !== "OKX" || apiPassphrase, missingMessage("OKX api passphrase"));
  if (!validationOnly && !allowDemoSubmit) {
    throw new Error("set CCVAR_ACCEPTANCE_ALLOW_DEMO_SUBMIT=true before running with CCVAR_ACCEPTANCE_VALIDATION_ONLY=false");
  }
}

async function assertCredentialRedaction(id) {
  const payload = await requestJSON("/api/credentials");
  const credential = (payload.credentials || []).find((item) => Number(item.id) === Number(id));
  assert(credential, "saved credential was not listed");
  assertNoForbiddenKeys(payload, "credential list");
  recordStep("credential_redaction", "ok", { credentialId: id, listedCredentials: (payload.credentials || []).length });
  console.log("ok credential redaction");
}

async function assertWorkspaceExportRedaction() {
  const response = await fetch(`${baseURL}/api/export`, { headers: { Accept: "application/json" } });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`/api/export returned ${response.status}: ${text.trim()}`);
  }
  assert((response.headers.get("cache-control") || "").toLowerCase().includes("no-store"), "workspace export missing Cache-Control: no-store");
  const payload = JSON.parse(text);
  assertNoForbiddenKeys(payload, "workspace export");
  report.artifacts.workspaceExport = {
    noStore: true,
    filename: contentDispositionFilename(response.headers.get("content-disposition") || ""),
  };
  recordStep("workspace_export_redaction", "ok", report.artifacts.workspaceExport);
  console.log("ok sanitized workspace export");
}

function writeReport() {
  if (reportWritten || !reportPath || reportPath.toLowerCase() === "none") return;
  reportWritten = true;
  report.finishedAt = new Date().toISOString();
  assertNoForbiddenKeys(report, "acceptance report");
  fs.mkdirSync(path.dirname(reportPath), { recursive: true });
  fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  console.log(`ok acceptance report ${reportPath}`);
}

function recordStep(name, status, details = {}) {
  report.steps.push({
    name,
    status,
    at: new Date().toISOString(),
    details,
  });
}

function contentDispositionFilename(value) {
  return value.match(/filename="([^"]+)"/)?.[1] || "";
}

function assertNoForbiddenKeys(value, label) {
  const forbidden = new Set(["apiKey", "apiSecret", "secret", "apiPassphrase", "passphrase", "ciphertext", "salt", "nonce"]);
  const walk = (node, path = []) => {
    if (Array.isArray(node)) {
      node.forEach((item, index) => walk(item, path.concat(String(index))));
      return;
    }
    if (!node || typeof node !== "object") return;
    for (const [key, child] of Object.entries(node)) {
      if (forbidden.has(key)) {
        throw new Error(`${label} exposed forbidden key at ${path.concat(key).join(".")}`);
      }
      walk(child, path.concat(key));
    }
  };
  walk(value);
}

async function requestJSON(path, options = {}) {
  const response = await fetch(`${baseURL}${path}`, {
    method: options.method || "GET",
    headers: {
      Accept: "application/json",
      ...(options.body ? { "Content-Type": "application/json" } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  const text = await response.text();
  let payload = {};
  if (text.trim()) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = { body: text };
    }
  }
  if (!response.ok) {
    throw new Error(`${path} returned ${response.status}: ${payload.error || text.trim()}`);
  }
  return payload;
}

function credentialValue(suffix) {
  const generic = env[`CCVAR_ACCEPTANCE_${suffix}`];
  if (generic) return generic;
  if (exchange === "Binance") {
    return env[`BINANCE_${environment.toUpperCase()}_${suffix}`] || env[`BINANCE_TESTNET_${suffix}`] || "";
  }
  if (exchange === "OKX") {
    return env[`OKX_DEMO_${suffix}`] || env[`OKX_${suffix}`] || "";
  }
  return "";
}

function missingMessage(label) {
  if (exchange === "Binance") {
    return `missing Binance ${label}; set CCVAR_ACCEPTANCE_${labelEnv(label)} or BINANCE_${environment.toUpperCase()}_${labelEnv(label)}`;
  }
  return `missing OKX ${label}; set CCVAR_ACCEPTANCE_${labelEnv(label)} or OKX_DEMO_${labelEnv(label)}`;
}

function labelEnv(label) {
  return label.toUpperCase().replaceAll(" ", "_");
}

function canonicalExchange(value) {
  const normalized = value.trim().toLowerCase();
  if (normalized === "binance") return "Binance";
  if (normalized === "okx") return "OKX";
  return "";
}

function normalizeEnvironment(value) {
  return value.trim().toLowerCase();
}

function parseBool(value, fallback) {
  if (value === undefined || value === "") return fallback;
  return ["1", "true", "yes", "on"].includes(String(value).trim().toLowerCase());
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}
NODE
