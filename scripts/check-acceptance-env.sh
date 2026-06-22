#!/usr/bin/env bash
set -euo pipefail

EXCHANGES="${CCVAR_ACCEPTANCE_ENV_CHECK_EXCHANGES:-${CCVAR_FINAL_AUDIT_REAL_EXCHANGES:-${CCVAR_ACCEPTANCE_EXCHANGE:-Binance,OKX}}}"
REPORT_PATH="${CCVAR_ACCEPTANCE_ENV_CHECK_REPORT_PATH:-}"
REQUIRE_READY="${CCVAR_ACCEPTANCE_ENV_CHECK_REQUIRE_READY:-false}"

EXCHANGES="$EXCHANGES" \
REPORT_PATH="$REPORT_PATH" \
REQUIRE_READY="$REQUIRE_READY" \
node <<'NODE'
const fs = require("fs");
const path = require("path");

const env = process.env;
const requested = String(env.EXCHANGES || "")
  .split(",")
  .map((item) => canonicalExchange(item))
  .filter(Boolean);
const exchanges = [...new Set(requested)];
const multiExchange = exchanges.length > 1;

if (exchanges.length === 0) {
  throw new Error("no exchanges requested; expected Binance, OKX, or Binance,OKX");
}

const genericVariables = [
  "CCVAR_ACCEPTANCE_API_KEY",
  "CCVAR_ACCEPTANCE_API_SECRET",
  "CCVAR_ACCEPTANCE_SECRET",
  "CCVAR_ACCEPTANCE_API_PASSPHRASE",
];
const genericPresent = genericVariables.filter((name) => Boolean(env[name]));

const results = exchanges.map((exchange) => readinessForExchange(exchange, multiExchange));
const missingExchanges = results.filter((item) => !item.ready).map((item) => item.exchange);
const report = {
  reportVersion: 1,
  generatedAt: new Date().toISOString(),
  ready: missingExchanges.length === 0 && !(multiExchange && genericPresent.length > 0),
  requestedExchanges: exchanges,
  missingExchanges,
  multiExchange,
  genericCredentialVariablesAllowed: !multiExchange,
  genericCredentialVariablesPresent: genericPresent,
  results,
  note:
    "This report lists environment variable names only. It never includes API keys, API signing material, API passphrases, or vault passphrases.",
};

if (multiExchange && genericPresent.length > 0) {
  report.ready = false;
  report.error =
    "Generic CCVAR_ACCEPTANCE_* credential variables are not allowed for multi-exchange final audit; use exchange-specific BINANCE_* and OKX_* variables.";
}

const text = JSON.stringify(report, null, 2);
if (env.REPORT_PATH) {
  fs.mkdirSync(path.dirname(env.REPORT_PATH), { recursive: true });
  fs.writeFileSync(env.REPORT_PATH, `${text}\n`);
}
console.log(text);

if (bool(env.REQUIRE_READY) && !report.ready) {
  process.exitCode = 1;
}

function readinessForExchange(exchange, multi) {
  const environment =
    exchange === "OKX"
      ? env.CCVAR_ACCEPTANCE_OKX_ENVIRONMENT || (multi ? "demo" : env.CCVAR_ACCEPTANCE_ENVIRONMENT || "demo")
      : env.CCVAR_ACCEPTANCE_BINANCE_ENVIRONMENT || (multi ? "testnet" : env.CCVAR_ACCEPTANCE_ENVIRONMENT || "testnet");
  if (exchange === "OKX" && environment !== "demo") {
    return {
      exchange,
      environment,
      ready: false,
      requirements: {},
      missing: ["okx_demo_environment"],
      note: "OKX acceptance supports demo only.",
    };
  }

  const requirements =
    exchange === "Binance"
      ? {
          api_key: choosePresent([
            ...(multi ? [] : ["CCVAR_ACCEPTANCE_API_KEY"]),
            `BINANCE_${environment.toUpperCase()}_API_KEY`,
            "BINANCE_TESTNET_API_KEY",
          ]),
          api_secret: choosePresent([
            ...(multi ? [] : ["CCVAR_ACCEPTANCE_API_SECRET", "CCVAR_ACCEPTANCE_SECRET"]),
            `BINANCE_${environment.toUpperCase()}_API_SECRET`,
            `BINANCE_${environment.toUpperCase()}_SECRET`,
            "BINANCE_TESTNET_API_SECRET",
            "BINANCE_TESTNET_SECRET",
          ]),
        }
      : {
          api_key: choosePresent([...(multi ? [] : ["CCVAR_ACCEPTANCE_API_KEY"]), "OKX_DEMO_API_KEY", "OKX_API_KEY"]),
          api_secret: choosePresent([
            ...(multi ? [] : ["CCVAR_ACCEPTANCE_API_SECRET", "CCVAR_ACCEPTANCE_SECRET"]),
            "OKX_DEMO_API_SECRET",
            "OKX_DEMO_SECRET",
            "OKX_API_SECRET",
            "OKX_SECRET",
          ]),
          api_passphrase: choosePresent([
            ...(multi ? [] : ["CCVAR_ACCEPTANCE_API_PASSPHRASE"]),
            "OKX_DEMO_API_PASSPHRASE",
            "OKX_API_PASSPHRASE",
          ]),
        };

  const missing = Object.entries(requirements)
    .filter(([, value]) => !value.present)
    .map(([name]) => name);

  return {
    exchange,
    environment,
    ready: missing.length === 0,
    requirements,
    missing,
  };
}

function choosePresent(candidates) {
  const uniqueCandidates = [...new Set(candidates)];
  const selected = uniqueCandidates.find((name) => Boolean(env[name]));
  return {
    present: Boolean(selected),
    selectedVariable: selected || "",
    acceptedVariables: uniqueCandidates,
  };
}

function canonicalExchange(value) {
  const normalized = String(value || "").trim().toLowerCase();
  if (normalized === "binance") return "Binance";
  if (normalized === "okx") return "OKX";
  if (normalized) throw new Error(`invalid exchange ${value}; expected Binance or OKX`);
  return "";
}

function bool(value) {
  return ["1", "true", "yes", "on"].includes(String(value || "").trim().toLowerCase());
}
NODE
