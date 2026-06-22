import { fallbackLabState } from "./fallbackData.js";

const defaultAPIBase =
  typeof window !== "undefined" && window.location.port !== "5173"
    ? window.location.origin
    : "http://127.0.0.1:8787";
const API_BASE = import.meta.env.VITE_API_BASE || defaultAPIBase;

export async function loadLabState({ exchange = "Binance", symbol = "BTCUSDT" } = {}) {
  try {
    const url = new URL(`${API_BASE}/api/lab-state`);
    url.searchParams.set("exchange", exchange);
    url.searchParams.set("symbol", symbol);
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error(`API returned ${response.status}`);
    }
    return { data: await response.json(), source: "api" };
  } catch (error) {
    return {
      data: {
        ...fallbackLabState,
        meta: { ...fallbackLabState.meta, dataSource: exchange, selectedMarket: symbol },
      },
      source: "fallback",
      error,
    };
  }
}

export async function loadAppInfo() {
  const response = await fetch(`${API_BASE}/api/app-info`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `app info returned ${response.status}`));
  }
  return response.json();
}

export async function loadPreflight() {
  const response = await fetch(`${API_BASE}/api/preflight`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `preflight returned ${response.status}`));
  }
  return response.json();
}

export async function loadAIProviders() {
  const response = await fetch(`${API_BASE}/api/ai-providers`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `AI providers returned ${response.status}`));
  }
  return response.json();
}

export async function simulateStep({ exchange = "Binance", symbol = "BTCUSDT", mode = "shadow" } = {}) {
  const url = new URL(`${API_BASE}/api/simulate/step`);
  url.searchParams.set("exchange", exchange);
  url.searchParams.set("symbol", symbol);
  url.searchParams.set("mode", mode);
  const response = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    let message = `simulate step returned ${response.status}`;
    try {
      const payload = await response.json();
      if (payload.error) message = payload.error;
    } catch {
      // Keep the status-based message when the server did not return JSON.
    }
    throw new Error(message);
  }
  return response.json();
}

export async function loadPaperExecutions({ limit = 8 } = {}) {
  const url = new URL(`${API_BASE}/api/paper-executions`);
  url.searchParams.set("limit", String(limit));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `paper executions returned ${response.status}`));
  }
  return response.json();
}

export async function loadPaperAccount() {
  const response = await fetch(`${API_BASE}/api/paper-account`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `paper account returned ${response.status}`));
  }
  return response.json();
}

export async function resetPaperExecutions(payload) {
  const response = await fetch(`${API_BASE}/api/paper-executions/reset`, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `paper reset returned ${response.status}`));
  }
  return response.json();
}

export async function loadAutopilot() {
  const response = await fetch(`${API_BASE}/api/autopilot`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `autopilot returned ${response.status}`));
  }
  return response.json();
}

export async function loadAutopilotRuns({ limit = 4 } = {}) {
  const url = new URL(`${API_BASE}/api/autopilot-runs`);
  url.searchParams.set("limit", String(limit));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `autopilot runs returned ${response.status}`));
  }
  return response.json();
}

export async function loadAutopilotSteps({ runId = 0, limit = 6 } = {}) {
  const url = new URL(`${API_BASE}/api/autopilot-steps`);
  url.searchParams.set("limit", String(limit));
  if (runId) url.searchParams.set("runId", String(runId));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `autopilot steps returned ${response.status}`));
  }
  return response.json();
}

export async function updateAutopilot(payload) {
  const response = await fetch(`${API_BASE}/api/autopilot`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `autopilot update returned ${response.status}`));
  }
  return response.json();
}

export async function loadRiskProfile() {
  const response = await fetch(`${API_BASE}/api/risk-profile`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `risk profile returned ${response.status}`));
  }
  return response.json();
}

export async function saveRiskProfile(payload) {
  const response = await fetch(`${API_BASE}/api/risk-profile`, {
    method: "PUT",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `risk profile save returned ${response.status}`));
  }
  return response.json();
}

export async function loadStrategyProfile() {
  const response = await fetch(`${API_BASE}/api/strategy-profile`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `strategy profile returned ${response.status}`));
  }
  return response.json();
}

export async function saveStrategyProfile(payload) {
  const response = await fetch(`${API_BASE}/api/strategy-profile`, {
    method: "PUT",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `strategy profile save returned ${response.status}`));
  }
  return response.json();
}

export async function runBacktest(payload) {
  const response = await fetch(`${API_BASE}/api/backtest/run`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload || {}),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `backtest returned ${response.status}`));
  }
  return response.json();
}

export async function loadBacktestRuns({ limit = 6 } = {}) {
  const url = new URL(`${API_BASE}/api/backtest-runs`);
  url.searchParams.set("limit", String(limit));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `backtest history returned ${response.status}`));
  }
  return response.json();
}

export async function loadCredentials() {
  const response = await fetch(`${API_BASE}/api/credentials`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `credentials returned ${response.status}`));
  }
  return response.json();
}

export async function saveCredential(payload) {
  const response = await fetch(`${API_BASE}/api/credentials`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `save credential returned ${response.status}`));
  }
  return response.json();
}

export async function deleteCredential(id) {
  const url = new URL(`${API_BASE}/api/credentials`);
  url.searchParams.set("id", String(id));
  const response = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `delete credential returned ${response.status}`));
  }
  return response.json();
}

export async function loadLiveGuard() {
  const response = await fetch(`${API_BASE}/api/live-guard`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `live guard returned ${response.status}`));
  }
  return response.json();
}

export async function loadKillSwitch() {
  const response = await fetch(`${API_BASE}/api/kill-switch`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `kill switch returned ${response.status}`));
  }
  return response.json();
}

export async function updateKillSwitch(payload) {
  const response = await fetch(`${API_BASE}/api/kill-switch`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `kill switch update returned ${response.status}`));
  }
  return response.json();
}

export async function updateLiveGuard(payload) {
  const response = await fetch(`${API_BASE}/api/live-guard`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `live guard update returned ${response.status}`));
  }
  return response.json();
}

export async function loadAuditLog({ limit = 20 } = {}) {
  const url = new URL(`${API_BASE}/api/audit-log`);
  url.searchParams.set("limit", String(limit));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `audit log returned ${response.status}`));
  }
  return response.json();
}

export async function executeLive(payload) {
  const response = await fetch(`${API_BASE}/api/live-execute`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `live execute returned ${response.status}`));
  }
  return response.json();
}

export async function loadLiveExecutions({ limit = 6 } = {}) {
  const url = new URL(`${API_BASE}/api/live-executions`);
  url.searchParams.set("limit", String(limit));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `live executions returned ${response.status}`));
  }
  return response.json();
}

export async function reconcileLiveExecution(payload) {
  const response = await fetch(`${API_BASE}/api/live-reconcile`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `live reconcile returned ${response.status}`));
  }
  return response.json();
}

export async function loadLiveReconciliations({ liveExecutionId, limit = 6 } = {}) {
  const url = new URL(`${API_BASE}/api/live-reconciliations`);
  url.searchParams.set("limit", String(limit));
  if (liveExecutionId) url.searchParams.set("liveExecutionId", String(liveExecutionId));
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `live reconciliations returned ${response.status}`));
  }
  return response.json();
}

export async function syncAccount(payload) {
  const response = await fetch(`${API_BASE}/api/account-sync`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `account sync returned ${response.status}`));
  }
  return response.json();
}

export async function loadAccountSnapshot({ credentialId, exchange, environment, symbol } = {}) {
  const url = new URL(`${API_BASE}/api/account-sync`);
  if (credentialId) url.searchParams.set("credentialId", String(credentialId));
  if (exchange) url.searchParams.set("exchange", exchange);
  if (environment) url.searchParams.set("environment", environment);
  if (symbol) url.searchParams.set("symbol", symbol);
  const response = await fetch(url, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `account snapshot returned ${response.status}`));
  }
  return response.json();
}

export async function exportWorkspace() {
  const response = await fetch(`${API_BASE}/api/export`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `workspace export returned ${response.status}`));
  }
  const disposition = response.headers.get("Content-Disposition") || "";
  const filename =
    disposition.match(/filename="([^"]+)"/)?.[1] ||
    `ccvar-quant-export-${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
  return {
    blob: await response.blob(),
    filename,
  };
}

export async function loadLocalData() {
  const response = await fetch(`${API_BASE}/api/local-data`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `local data returned ${response.status}`));
  }
  return response.json();
}

export async function pruneLocalData(payload) {
  const response = await fetch(`${API_BASE}/api/local-data/prune`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload || {}),
  });
  if (!response.ok) {
    throw new Error(await readAPIError(response, `local data prune returned ${response.status}`));
  }
  return response.json();
}

async function readAPIError(response, fallback) {
  try {
    const payload = await response.json();
    return payload.error || fallback;
  } catch {
    return fallback;
  }
}
