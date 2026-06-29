import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Brain,
  Check,
  ChevronDown,
  Clock3,
  Copy,
  Database,
  Download,
  FlaskConical,
  Gauge,
  KeyRound,
  Languages,
  LoaderCircle,
  LockKeyhole,
  Maximize2,
  Minimize2,
  Moon,
  Pause,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Square,
  Sun,
  Trash2,
  X,
  Zap,
} from "lucide-react";
import {
  AreaSeries,
  CandlestickSeries,
  HistogramSeries,
  LineSeries,
  createChart,
  createSeriesMarkers,
} from "lightweight-charts";
import {
  deleteCredential,
  executeLive,
  exportWorkspace,
  loadAIProviders,
  loadAppInfo,
  loadAccountSnapshot,
  loadAuditLog,
  loadAutopilot,
  loadAutopilotRuns,
  loadAutopilotSteps,
  loadBacktestRuns,
  loadCredentials,
  loadKillSwitch,
  loadLabState,
  loadLiveGuard,
  loadLiveExecutions,
  loadLiveReconciliations,
  loadLocalData,
  loadPaperAccount,
  loadPaperExecutions,
  loadPreflight,
  loadRiskProfile,
  loadStrategyProfile,
  pruneLocalData,
  reconcileLiveExecution,
  resetPaperExecutions,
  runBacktest,
  saveCredential,
  saveRiskProfile,
  saveStrategyProfile,
  simulateStep,
  syncAccount,
  updateAutopilot,
  updateKillSwitch,
  updateLiveGuard,
} from "./api.js";
import { fallbackLabState } from "./fallbackData.js";
import { choiceLabel, makeTranslator, resolveLocale } from "./i18n/index.js";

const numberFormat = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 2,
});

const moneyFormat = new Intl.NumberFormat("en-US", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

function formatMoney(value) {
  return moneyFormat.format(value);
}

function normalizeSignedValue(value, digits = 2) {
  const number = Number(value) || 0;
  const threshold = 0.5 * (10 ** -digits);
  return Math.abs(number) < threshold ? 0 : number;
}

function formatSignedMoney(value) {
  const normalized = normalizeSignedValue(value, 2);
  const sign = normalized > 0 ? "+" : normalized < 0 ? "-" : "";
  return `${sign}${moneyFormat.format(Math.abs(normalized))}`;
}

function formatSignedPct(value) {
  const normalized = normalizeSignedValue(value, 2);
  const sign = normalized > 0 ? "+" : normalized < 0 ? "-" : "";
  return `${sign}${Math.abs(normalized).toFixed(2)}%`;
}

function formatSignedNumber(value, digits = 1) {
  const normalized = normalizeSignedValue(value, digits);
  const sign = normalized > 0 ? "+" : normalized < 0 ? "-" : "";
  return `${sign}${Math.abs(normalized).toFixed(digits)}`;
}

function formatMarketDataSource(value) {
  const normalized = String(value || "").toLowerCase();
  if (normalized.includes("seed")) return "seed";
  if (normalized.includes("public") || normalized.includes("live")) return "live";
  return value || "-";
}

function marketDataSourceLabel(t, value) {
  const source = formatMarketDataSource(value);
  if (source === "seed" || source === "live" || source === "replay") return marketDataModeShortLabel(t, source);
  return source;
}

function classNames(...values) {
  return values.filter(Boolean).join(" ");
}

const statusMessageKeys = new Map([
  ["Ready", "status.ready"],
  ["Loading", "status.loading"],
  ["Loaded", "status.loaded"],
  ["Local vault", "status.local_vault"],
  ["Not tested", "status.not_tested"],
  ["Not synced", "status.not_synced"],
  ["No checks", "status.no_checks"],
  ["Guarded", "status.guarded"],
  ["Encrypting", "status.encrypting"],
  ["Saved encrypted", "status.saved_encrypted"],
  ["Deleting", "status.deleting"],
  ["Deleted", "status.deleted"],
  ["Select key", "status.select_key"],
  ["Passphrase required", "status.passphrase_required"],
  ["Can trade", "status.can_trade"],
  ["Read only", "status.read_only"],
  ["Unlocked", "status.unlocked"],
  ["Locked", "status.locked"],
  ["kill switch clear", "status.kill_switch_clear"],
  ["Saving", "status.saving"],
  ["Saved", "status.saved"],
  ["Draft", "status.draft"],
  ["Run needed", "status.run_needed"],
  ["Running", "status.running"],
  ["Idle", "status.idle"],
  ["API unavailable", "status.api_unavailable"],
  ["Market data timeout", "status.market_data_timeout"],
  ["Network error", "status.network_error"],
  ["Connection failed", "status.connection_failed"],
  ["Exporting", "status.exporting"],
  ["Failed", "status.failed"],
  ["Pruning", "status.pruning"],
  ["Resetting", "status.resetting"],
  ["Confirm reset", "status.confirm_reset"],
  ["Phrase mismatch", "status.phrase_mismatch"],
  ["Stop AI first", "status.stop_ai_first"],
  ["Syncing", "status.syncing"],
  ["Executing", "status.executing"],
  ["Updating", "status.updating"],
  ["Unlocking", "status.unlocking"],
  ["Locking", "status.locking"],
  ["Updated", "status.updated"],
  ["Unavailable", "status.unavailable"],
  ["kill switch unavailable", "status.kill_switch_unavailable"],
  ["Guard locked", "status.guard_locked"],
]);

function normalizeStatusValue(value) {
  if (!value) return value;
  const raw = String(value);
  const normalized = raw.toLowerCase();
  if (normalized.includes("returned 404") || normalized.includes("404 not found") || normalized === "404") {
    return "API unavailable";
  }
  if (normalized.includes("context deadline exceeded") || normalized.includes("client.timeout") || normalized.includes("timeout")) {
    return "Market data timeout";
  }
  if (normalized.includes("failed to fetch") || normalized.includes("network") || normalized.includes("connection refused")) {
    return "Network error";
  }
  const aliases = {
    idle: "Idle",
    running: "Running",
    unavailable: "Unavailable",
    failed: "Failed",
  };
  if (aliases[normalized]) return aliases[normalized];
  return value;
}

function statusText(t, value) {
  if (!value) return "-";
  const normalizedValue = normalizeStatusValue(value);
  const key = statusMessageKeys.get(normalizedValue);
  return key ? t(key, normalizedValue) : normalizedValue;
}

function rawErrorMessage(error) {
  if (!error) return "";
  if (typeof error === "string") return error;
  return error.message || String(error);
}

function sanitizeErrorMessage(message) {
  return String(message || "")
    .replace(/((?:api[_\s-]?key|api[_\s-]?secret|secret|passphrase|token)\s*[:=]\s*)(["']?)[^\s,;]+/gi, "$1$2***")
    .trim();
}

function exchangeNameFromError(message) {
  const raw = String(message || "").toLowerCase();
  if (raw.includes("binance")) return "BINANCE";
  if (raw.includes("okx")) return "OKX";
  return "";
}

function exchangePayloadFromError(message) {
  const raw = String(message || "");
  const start = raw.indexOf("{");
  const end = raw.lastIndexOf("}");
  if (start < 0 || end <= start) return null;
  try {
    const payload = JSON.parse(raw.slice(start, end + 1));
    return {
      code: payload?.code === undefined ? "" : String(payload.code),
      message: String(payload?.msg || payload?.message || payload?.error || "").trim(),
    };
  } catch {
    return null;
  }
}

function isBinanceTimestampError(raw, exchangePayload) {
  const text = `${raw || ""} ${exchangePayload?.message || ""}`.toLowerCase();
  return exchangePayload?.code === "-1021" || text.includes("recvwindow") || text.includes("outside receive window");
}

function isRestrictedLocationError(raw, exchangePayload) {
  const text = `${raw || ""} ${exchangePayload?.message || ""}`.toLowerCase();
  return text.includes("restricted location") || text.includes("eligibility");
}

function isExchangeNetworkError(raw) {
  const text = String(raw || "").toLowerCase();
  return (
    text.includes("network unavailable") ||
    text.includes("dial tcp") ||
    text.includes("host is down") ||
    text.includes("no such host") ||
    text.includes("connection refused") ||
    text.includes("connection reset")
  );
}

const localizedErrorRules = [
  [/^method not allowed$/i, "errors.methodNotAllowed", "This action uses an unsupported request method."],
  [/^origin not allowed$/i, "errors.originNotAllowed", "This page origin is not allowed by the local API."],
  [/^unsupported exchange$/i, "errors.unsupportedExchange", "This exchange is not supported yet."],
  [/^valid id is required$/i, "errors.validIdRequired", "The selected record is invalid. Refresh and try again."],
  [/^exchange is required$/i, "errors.exchangeRequired", "Choose an exchange."],
  [/^api key is required$/i, "errors.apiKeyRequired", "Enter the exchange API Key."],
  [/^api secret is required$/i, "errors.apiSecretRequired", "Enter the exchange API Secret."],
  [/^okx api passphrase is required$/i, "errors.okxPassphraseRequired", "OKX requires an API Passphrase."],
  [/^passphrase must be at least 12 characters$/i, "errors.passphraseMinLength", "The vault passphrase must be at least 12 characters."],
  [/^withdrawal permission is not allowed$/i, "errors.withdrawalForbidden", "Withdrawal permission is not allowed for safety."],
  [/^unsupported kdf\b/i, "errors.unsupportedVaultFormat", "This saved key uses an unsupported vault format."],
  [/message authentication failed|cipher: message authentication failed|decrypt/i, "errors.credentialDecryptFailed", "Vault decrypt failed. Check the passphrase."],
  [/^credential id is required$/i, "errors.credentialIdRequired", "Choose a saved trading key."],
  [/^credential passphrase is required$/i, "errors.credentialPassphraseRequired", "Enter the vault passphrase."],
  [/^credential exchange does not match request$/i, "errors.credentialExchangeMismatch", "The selected key does not match the current exchange."],
  [/^credential exchange does not match execution$/i, "errors.credentialExchangeMismatch", "The selected key does not match the current exchange."],
  [/^credential trade permission is required$/i, "errors.credentialTradePermission", "This key has no trade permission."],
  [/^credential \d+ not found$/i, "errors.credentialNotFound", "The selected trading key was not found. Refresh the vault."],
  [/^live guard is locked$/i, "errors.liveGuardLocked", "Live Guard is locked."],
  [/^unlock phrase must be ENABLE TESTNET LIVE$/i, "errors.unlockPhraseRequired", "Type ENABLE TESTNET LIVE to unlock testnet live mode."],
  [/^production live trading is disabled$/i, "errors.productionLiveDisabled", "Production live trading is disabled in this release."],
  [/^unlock ttl must be between 60 and 900 seconds$/i, "errors.unlockTtlRange", "Unlock TTL must be between 60 and 900 seconds."],
  [/^action must be unlock or lock$/i, "errors.guardActionInvalid", "Guard action must be unlock or lock."],
  [/^action must be activate or resume$/i, "errors.killSwitchActionInvalid", "Kill Switch action must be activate or resume."],
  [/^action must be start, stop, or step$/i, "errors.autopilotActionInvalid", "Autopilot action must be start, stop, or step."],
  [/^kill switch is active$/i, "errors.killSwitchActive", "Kill Switch is active. Resume it before continuing."],
  [/^autopilot is already running$/i, "errors.autopilotAlreadyRunning", "Autopilot is already running."],
  [/^autopilot is not running$/i, "errors.autopilotNotRunning", "Autopilot is not running."],
  [/^autopilot mode must be shadow, paper, or live$/i, "errors.autopilotModeInvalid", "Autopilot mode must be Shadow, Paper, or Live."],
  [/^live autopilot requires credential id and vault passphrase$/i, "errors.liveAutopilotCredentialRequired", "Live Autopilot requires a trading key and vault passphrase."],
  [/^simulation runner is not configured$/i, "errors.simulationRunnerMissing", "Simulation runner is not configured."],
  [/^live executor is not configured$/i, "errors.liveExecutorMissing", "Live executor is not configured."],
  [/^recent account snapshot is required before live execution$/i, "errors.accountSnapshotRequired", "Sync an account snapshot before live execution."],
  [/^account snapshot is stale; sync account before live execution$/i, "errors.accountSnapshotStale", "The account snapshot is stale. Sync again before execution."],
  [/^account snapshot indicates trading is disabled$/i, "errors.accountCannotTrade", "The synced account indicates trading is disabled."],
  [/^account sync supports only testnet or demo$/i, "errors.accountSyncEnvironment", "Account sync only supports testnet or demo."],
  [/^live reconciliation supports only testnet or demo$/i, "errors.reconcileEnvironment", "Live reconciliation only supports testnet or demo."],
  [/^validation-only execution has no exchange order to reconcile$/i, "errors.validationOnlyNoOrder", "Validation-only execution has no exchange order to reconcile."],
  [/^live execution was not submitted to an exchange$/i, "errors.executionNotSubmitted", "This execution was not submitted to an exchange."],
  [/^live execution id is required$/i, "errors.liveExecutionIdRequired", "Choose a live execution record first."],
  [/^live execution record not found$/i, "errors.liveExecutionNotFound", "The live execution record was not found."],
  [/^unsupported account sync exchange$/i, "errors.unsupportedAccountSyncExchange", "This exchange is not supported for account sync."],
  [/^unsupported live execution exchange$/i, "errors.unsupportedLiveExecutionExchange", "This exchange is not supported for live execution."],
  [/^unsupported live reconciliation exchange$/i, "errors.unsupportedLiveReconciliationExchange", "This exchange is not supported for reconciliation."],
  [/^binance api key and secret are required$/i, "errors.binanceAuthRequired", "Binance requires API Key and API Secret."],
  [/^okx api key, secret, and api passphrase are required$/i, "errors.okxAuthRequired", "OKX requires API Key, API Secret, and API Passphrase."],
  [/^binance .* supports only testnet or demo$/i, "errors.binanceEnvironment", "Binance private operations only support testnet or demo here."],
  [/^okx .* supports demo environment only$/i, "errors.okxEnvironment", "OKX private operations only support demo here."],
  [/^backtest requires at least slow window plus two candles$/i, "errors.backtestNotEnoughCandles", "Backtest needs more candle data for the selected windows."],
  [/^stop autopilot before resetting paper ledger$/i, "errors.stopAutopilotBeforePaperReset", "Stop Autopilot before resetting the paper ledger."],
  [/^confirmation phrase must be RESET PAPER$/i, "errors.resetPaperPhrase", "Type RESET PAPER to reset the paper ledger."],
  [/^stop autopilot before pruning local data$/i, "errors.stopAutopilotBeforePrune", "Stop Autopilot before pruning local data."],
  [/^confirmation phrase must be PRUNE LOCAL DATA$/i, "errors.prunePhrase", "Type PRUNE LOCAL DATA before pruning local research data."],
  [/^unsupported prune table$/i, "errors.unsupportedPruneTable", "This local data table cannot be pruned from the UI."],
  [/^exchange and symbol are required$/i, "errors.exchangeSymbolRequired", "Choose an exchange and symbol."],
  [/^order size must be positive$/i, "errors.orderSizePositive", "Order size must be greater than 0."],
  [/^order size [\d.,]+ exceeds [\d.,]+$/i, "errors.orderSizeExceedsRisk", "Order size exceeds the current risk limit."],
  [/^invalid risk limits$/i, "errors.invalidRiskLimits", "Risk limits are invalid. Review max order, drawdown, confidence, and spread."],
  [/^invalid strategy defaults$/i, "errors.invalidStrategyDefaults", "Strategy defaults are invalid. Review exchange, symbol, side, model route, and order size."],
  [/failed to fetch|network|connection refused/i, "errors.network", "Cannot reach the local API. Check that the client service is running."],
  [/timeout|context deadline exceeded|client\.timeout/i, "errors.timeout", "The request timed out. Check the exchange connection and try again."],
];

function localizedErrorDetail(t, error) {
  const raw = sanitizeErrorMessage(rawErrorMessage(error));
  if (!raw) return t("errors.generic", "Something went wrong. Try again.");

  const exchangeName = exchangeNameFromError(raw);
  const exchangePayload = exchangePayloadFromError(raw);
  if (exchangeName === "BINANCE" && isBinanceTimestampError(raw, exchangePayload)) {
    return t("errors.binanceTimestampSkew", "Binance rejected the request timestamp. Turn on automatic system time sync, then try again.");
  }
  if (exchangeName && isRestrictedLocationError(raw, exchangePayload)) {
    return t("errors.exchangeRestrictedLocation", "{exchange} blocked this connection from the current network or region. Try a supported network or switch exchange environment.", { exchange: exchangeName });
  }
  if (exchangeName && isExchangeNetworkError(raw)) {
    return t("errors.exchangeNetworkUnavailable", "{exchange} is unreachable from this network. Check DNS/proxy/VPN settings and try again.", { exchange: exchangeName });
  }
  if (exchangeName && exchangePayload) {
    return t("errors.exchangeReturned", "{exchange} returned {code}: {message}", {
      exchange: exchangeName,
      code: exchangePayload.code || "-",
      message: exchangePayload.message || "-",
    });
  }

  const jsonField = raw.match(/^json: unknown field "([^"]+)"$/i);
  if (jsonField) {
    return t("errors.unknownField", "Unsupported field: {field}. Refresh and try again.", { field: jsonField[1] });
  }

  const apiStatus = raw.match(/^(?:api|[\w\s-]+) returned (\d{3})$/i);
  if (apiStatus) {
    return t("errors.httpStatus", "Local API returned HTTP {status}.", { status: apiStatus[1] });
  }

  const exchangeCode = raw.match(/^(binance|okx).*(?:returned|rejected)(?: code)?\s+([^:]+)(?::\s*(.+))?$/i);
  if (exchangeCode) {
    return t("errors.exchangeReturned", "{exchange} returned {code}: {message}", {
      exchange: exchangeCode[1].toUpperCase(),
      code: exchangeCode[2].trim(),
      message: (exchangeCode[3] || "-").trim(),
    });
  }

  for (const [pattern, key, fallback] of localizedErrorRules) {
    if (pattern.test(raw)) return t(key, fallback);
  }

  return t("errors.unknownWithDetail", "Unexpected error: {message}", { message: raw });
}

function localizedErrorMessage(t, error, contextKey = "generic") {
  const detail = localizedErrorDetail(t, error);
  return t(`errors.context.${contextKey}`, "{message}", { message: detail });
}

async function copyTextToClipboard(text) {
  if (!text) return false;
  const previousFocus = typeof document !== "undefined" && document.activeElement instanceof HTMLElement
    ? document.activeElement
    : null;
  if (navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the synchronous textarea path for desktop shells and restricted browser contexts.
    }
  }
  if (typeof document === "undefined") return false;
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.left = "-1000px";
  textarea.style.opacity = "0";
  textarea.style.pointerEvents = "none";
  document.body.appendChild(textarea);
  textarea.focus({ preventScroll: true });
  textarea.select();
  try {
    return document.execCommand?.("copy") === true;
  } catch {
    return false;
  } finally {
    textarea.remove();
    if (previousFocus && document.contains(previousFocus)) {
      previousFocus.focus?.({ preventScroll: true });
    }
  }
}

function notifyError(notify, t, error, contextKey) {
  notify(localizedErrorMessage(t, error, contextKey), "danger");
}

function compactStatusText(t, value, maxLength = 64) {
  const text = statusText(t, value);
  if (!value) return text;
  return text.length > maxLength ? `${text.slice(0, Math.max(0, maxLength - 3))}...` : text;
}

function compactStatusTitle(t, value) {
  const fullText = statusText(t, value);
  const compactText = compactStatusText(t, value);
  return fullText !== compactText ? fullText : undefined;
}

function sourceStatusText(t, value) {
  switch (value) {
    case "api":
      return t("top.apiOk", "OK");
    case "fallback":
      return t("top.apiFallback", "Offline");
    case "loading":
      return t("top.apiChecking", "Checking");
    default:
      return value ? String(value) : "-";
  }
}

function eventText(t, value) {
  if (value === undefined || value === null || value === "") return "";
  const text = String(value);
  const bidAskSpread = text.match(/^bid\s+(.+)\s+\/\s+ask\s+(.+)\s+\/\s+spread\s+(.+)$/i);
  if (bidAskSpread) {
    return t("events.bidAskSpread", "bid {bid} / ask {ask} / spread {spread}", {
      bid: bidAskSpread[1],
      ask: bidAskSpread[2],
      spread: bidAskSpread[3],
    });
  }
  if (text.startsWith("Conf:")) {
    return text.replace(/^Conf:/, t("events.confPrefix", "Conf:"));
  }
  return choiceLabel(t, text);
}

function reasoningText(t, value) {
  const text = String(value || "");
  if (text === "Local AI Policy evaluated public market features for AI Momentum Pro; BUY intent uses 500 USDT with 78% confidence. Spread and liquidity are supportive while funding pressure remains contained.") {
    return t("sample.reasoningMomentum", text);
  }
  const policyReasoning = text.match(/^(Local AI Policy)(?:\s+(\S+))?\s+evaluated public market features for (.*?);\s+([A-Z]+) intent uses ([\d.,]+\s+USDT) with ([\d.]+)% confidence\. Spread ([^,]+), liquidity ([^.]+)\. Simulation policy only unless routed through Live Guard\.$/);
  if (policyReasoning) {
    const [, policy, version, strategy, side, size, confidence, spread, liquidity] = policyReasoning;
    return t("sample.localPolicyReasoning", "{policy}{version} evaluated public market features for {strategy}; {side} intent uses {size} with {confidence}% confidence. Spread {spread}, liquidity {liquidity}. Simulation policy only unless routed through Live Guard.", {
      policy: policy === "Local AI Policy" ? t("aiConfig.providerTitles.local_policy", "Local AI Policy") : policy,
      version: version ? ` ${version}` : "",
      strategy,
      side: choiceLabel(t, side),
      size,
      confidence,
      spread,
      liquidity,
    });
  }
  return text || "-";
}

function modelMetaText(t, value) {
  return String(value || "-").replace(/\btick\b/g, t("units.tick", "tick"));
}

function latencyToneClass(value) {
  const latency = Number(value || 0);
  if (latency > 8000) return "negative-text";
  if (latency > 1500) return "warn-text";
  return "positive-text";
}

function readStoredLocale() {
  if (typeof window === "undefined") return "";
  try {
    return window.localStorage?.getItem("ccvar.locale") || "";
  } catch {
    return "";
  }
}

function writeStoredLocale(locale) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage?.setItem("ccvar.locale", locale);
  } catch {
    // Some embedded/private browser contexts disable localStorage.
  }
}

function resolveTheme(value) {
  return value === "light" ? "light" : "dark";
}

function readStoredTheme() {
  if (typeof window === "undefined") return "dark";
  try {
    return resolveTheme(window.localStorage?.getItem("ccvar.theme"));
  } catch {
    return "dark";
  }
}

function writeStoredTheme(theme) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage?.setItem("ccvar.theme", resolveTheme(theme));
  } catch {
    // Some embedded/private browser contexts disable localStorage.
  }
}

function useEscapeToClose(open, onClose) {
  useEffect(() => {
    if (!open || typeof window === "undefined") return undefined;
    const handleKeyDown = (event) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);
}

function useDismissOnOutside(open, ref, onClose) {
  useEffect(() => {
    if (!open || typeof window === "undefined") return undefined;
    const handlePointerDown = (event) => {
      if (!ref.current?.contains(event.target)) {
        onClose();
      }
    };
    window.addEventListener("pointerdown", handlePointerDown);
    return () => window.removeEventListener("pointerdown", handlePointerDown);
  }, [open, ref, onClose]);
}

function focusFirstMenuItem(container) {
  window.requestAnimationFrame(() => {
    const firstItem = container?.querySelector?.('[role="menuitem"], [role="menuitemcheckbox"], [role="option"]');
    firstItem?.focus?.({ preventScroll: true });
  });
}

function focusMenuTrigger(container) {
  window.requestAnimationFrame(() => {
    const trigger = container?.querySelector?.("[aria-haspopup]");
    trigger?.focus?.({ preventScroll: true });
  });
}

function handleMenuTriggerKeyDown(event, openMenu) {
  if (!["ArrowDown", "Enter", " "].includes(event.key)) return false;
  event.preventDefault();
  openMenu();
  return true;
}

function handleMenuListKeyDown(event, { container, closeMenu }) {
  const items = Array.from(container?.querySelectorAll?.('[role="menuitem"], [role="menuitemcheckbox"], [role="option"]') || []);
  if (event.key === "Escape") {
    event.preventDefault();
    closeMenu();
    focusMenuTrigger(container);
    return true;
  }
  if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key) || items.length === 0) return false;
  event.preventDefault();
  const currentIndex = Math.max(0, items.indexOf(document.activeElement));
  const nextIndex = event.key === "Home"
    ? 0
    : event.key === "End"
      ? items.length - 1
      : event.key === "ArrowUp"
        ? (currentIndex - 1 + items.length) % items.length
        : (currentIndex + 1) % items.length;
  items[nextIndex]?.focus?.({ preventScroll: true });
  return true;
}

function useDialogFocus(open) {
  const dialogRef = useRef(null);

  useEffect(() => {
    if (!open || typeof window === "undefined") return undefined;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const focusableSelector = [
      ".icon-close",
      "a[href]",
      "button:not(:disabled)",
      "input:not(:disabled)",
      "select:not(:disabled)",
      "textarea:not(:disabled)",
      "[tabindex]:not([tabindex='-1'])",
    ].join(",");
    const getFocusableElements = () => {
      const dialog = dialogRef.current;
      if (!dialog) return [];
      return Array.from(dialog.querySelectorAll(focusableSelector)).filter((element) => {
        if (!(element instanceof HTMLElement)) return false;
        if (element.getAttribute("aria-hidden") === "true") return false;
        const style = window.getComputedStyle(element);
        if (style.display === "none" || style.visibility === "hidden") return false;
        const rect = element.getBoundingClientRect();
        return rect.width > 0 && rect.height > 0;
      });
    };
    const focusFrame = window.requestAnimationFrame(() => {
      const target = dialogRef.current?.querySelector("[data-autofocus]") || getFocusableElements()[0];
      target?.focus?.({ preventScroll: true });
    });
    const handleKeyDown = (event) => {
      if (event.key !== "Tab") return;
      const dialog = dialogRef.current;
      if (!dialog) return;
      const focusable = getFocusableElements();
      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (!active || !dialog.contains(active)) {
        event.preventDefault();
        first.focus({ preventScroll: true });
        return;
      }
      if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus({ preventScroll: true });
        return;
      }
      if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus({ preventScroll: true });
      }
    };
    document.addEventListener("keydown", handleKeyDown, true);

    return () => {
      window.cancelAnimationFrame(focusFrame);
      document.removeEventListener("keydown", handleKeyDown, true);
      if (!previousFocus || !document.contains(previousFocus)) return;
      window.requestAnimationFrame(() => {
        previousFocus.focus?.({ preventScroll: true });
      });
    };
  }, [open]);

  return dialogRef;
}

function formatDateTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("en-GB", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function formatClock(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function formatReplayTimestamp(seconds) {
  if (!seconds) return "-";
  const date = new Date(Number(seconds) * 1000);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("en-GB", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function buildReplayOptions(t, labState, backtestResult) {
  const options = [];
  const seen = new Set();
  const add = (option) => {
    if (!option?.value || seen.has(option.value)) return;
    seen.add(option.value);
    options.push(option);
  };
  const candles = labState?.candles || [];
  const markers = [
    { index: 18, tag: t("panels.buySignal", "BUY signal") },
    { index: 34, tag: t("panels.sellSignal", "SELL signal") },
    { index: 54, tag: t("panels.buySignal", "BUY signal") },
    { index: 72, tag: t("panels.sellSignal", "SELL signal") },
    { index: 90, tag: t("panels.buySignal", "BUY signal") },
  ];
  markers.forEach(({ index, tag }) => {
    const candle = candles[index];
    if (!candle?.time) return;
    add({
      value: `candle:${candle.time}`,
      label: formatReplayTimestamp(candle.time),
      tag,
    });
  });
  const latest = candles[candles.length - 1];
  if (latest?.time) {
    add({
      value: `candle:${latest.time}`,
      label: formatReplayTimestamp(latest.time),
      tag: t("panels.latestCandle", "Latest candle"),
    });
  }
  (backtestResult?.trades || []).slice(-3).reverse().forEach((trade) => {
    add({
      value: `trade:${trade.id}`,
      label: formatDateTime(trade.openedAt),
      tag: `${String(trade.side || "").toUpperCase()} ${trade.id}`,
    });
  });
  return options.slice(0, 7);
}

function normalizeAutopilotResult(result) {
  if (!result) return null;
  if (typeof result === "string") {
    try {
      return JSON.parse(result);
    } catch {
      return null;
    }
  }
  return result;
}

function formatConfidence(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return "-";
  const percent = numeric <= 1 ? numeric * 100 : numeric;
  return `${Math.round(percent)}%`;
}

function autopilotStepIntent(record) {
  const result = normalizeAutopilotResult(record?.result) || {};
  return result.aiPlan?.intent || result.intent || result.execution?.intent || {};
}

function autopilotStepDecision(record) {
  const result = normalizeAutopilotResult(record?.result) || {};
  return result.execution?.decision || result.decision || {};
}

function autopilotStepOutcome(record) {
  const result = normalizeAutopilotResult(record?.result) || {};
  if (result.execution?.execution?.status) return result.execution.execution.status;
  if (result.fill?.orderId) return "filled";
  if (Array.isArray(record?.events) && record.events.length > 0) {
    return record.events[record.events.length - 1].result || record.status || "-";
  }
  return record?.status || "-";
}

function fileName(value) {
  const text = String(value || "");
  if (!text) return "-";
  return text.split(/[\\/]/).filter(Boolean).pop() || text;
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

const defaultCredentialForm = {
  exchange: "Binance",
  label: "Main trading key",
  apiKey: "",
  secret: "",
  apiPassphrase: "",
  passphrase: "",
  permissions: {
    read: true,
    trade: true,
    withdraw: false,
  },
};

const defaultLiveGuardForm = {
  operator: "local",
  environment: "testnet",
  phrase: "",
  ttlSeconds: 600,
  maxOrderUsdt: 1000,
  reason: "testnet validation only",
};

const defaultLiveExecutionForm = {
  credentialId: "",
  passphrase: "",
  exchange: "Binance",
  symbol: "BTCUSDT",
  side: "buy",
  sizeUsdt: 100,
  validationOnly: true,
};

const defaultAutopilotState = {
  running: false,
  runId: 0,
  mode: "shadow",
  exchange: "Binance",
  symbol: "BTCUSDT",
  intervalSeconds: 15,
  completedSteps: 0,
  lastStatus: "idle",
  message: "autopilot idle",
};

const defaultRiskProfile = {
  name: "Local Guardrails",
  minConfidence: 0.65,
  maxOrderUsdt: 1000,
  maxSymbolExposureUsdt: 8000,
  maxTotalExposureUsdt: 12000,
  maxDailyDrawdownPct: 3,
  maxConsecutiveLosses: 3,
  maxSpreadPct: 0.08,
  requireLiveUnlock: true,
};

const defaultStrategyProfile = {
  name: "AI Momentum Pro",
  exchange: "Binance",
  symbol: "BTCUSDT",
  side: "buy",
  modelProfile: "local_policy",
  modelFallback: "local_policy",
  concurrency: 2,
  orderSizeUsdt: 500,
  intervalSeconds: 15,
  maxSteps: 0,
};

const defaultPaperAccount = {
  startingCapitalUsdt: 100000,
  cashUsdt: 100000,
  equityUsdt: 100000,
  realizedPnlUsdt: 0,
  unrealizedPnlUsdt: 0,
  totalPnlUsdt: 0,
  returnPct: 0,
  openNotionalUsdt: 0,
  feesUsdt: 0,
  filledCount: 0,
  rejectedCount: 0,
  winCount: 0,
  lossCount: 0,
  positions: [],
  updatedAt: "",
};

const defaultLocalData = {
  summary: {
    backtestRuns: 0,
    autopilotRuns: 0,
    autopilotSteps: 0,
    paperExecutions: 0,
    accountSnapshots: 0,
    liveExecutions: 0,
    liveReconciliations: 0,
    auditEntries: 0,
    credentials: 0,
  },
  keep: {
    keepBacktestRuns: 30,
    keepAutopilotRuns: 20,
    keepPaperExecutions: 500,
    keepAccountSnapshots: 50,
  },
  protected: [],
};

const defaultAppInfo = {
  service: "ccvar-quant",
  version: "0.1.0",
  address: "127.0.0.1:8787",
  url: "http://127.0.0.1:8787",
  startedAt: "",
  runtime: {
    goos: "",
    goarch: "",
    goVersion: "",
  },
  database: {
    path: "",
    dir: "",
    exists: false,
    sizeBytes: 0,
  },
  docs: {
    available: false,
    runbook: { path: "", exists: false, sizeBytes: 0 },
    safety: { path: "", exists: false, sizeBytes: 0 },
  },
  security: {
    localOriginOnly: true,
    productionTradingEnabled: false,
    productionAccountSyncEnabled: false,
    liveEnvironments: ["testnet", "demo"],
  },
  exchanges: ["Binance", "OKX"],
};

const defaultPreflight = {
  generatedAt: "",
  overall: "warn",
  ready: 0,
  warn: 0,
  block: 0,
  checks: [],
};

const defaultAIProviders = {
  generatedAt: "",
  providers: [
    {
      id: "local_policy",
      label: "Local AI Policy",
      kind: "local",
      state: "ok",
      source: "built-in",
      model: "v0.2.0",
      detail: "Deterministic local policy is available for shadow, paper, and guarded live validation.",
      guidance: "No external model credentials are required for the first release.",
    },
    {
      id: "codex_cli",
      label: "Codex CLI / ChatGPT subscription",
      kind: "subscription_cli",
      state: "unknown",
      command: "codex login",
      model: "gpt-5",
      detail: "Provider detection has not run yet.",
      guidance: "Refresh AI configuration to detect the local Codex CLI login.",
    },
    {
      id: "claude_cli",
      label: "Claude CLI / Claude subscription",
      kind: "subscription_cli",
      state: "unknown",
      command: "claude setup-token",
      model: "claude-sonnet-4",
      detail: "Provider detection has not run yet.",
      guidance: "Refresh AI configuration to detect the local Claude subscription token.",
    },
    {
      id: "compatible_endpoint",
      label: "OpenAI-compatible / local endpoint",
      kind: "endpoint",
      state: "unknown",
      command: "set OPENAI_BASE_URL",
      model: "local-model",
      detail: "Provider detection has not run yet.",
      guidance: "Set a local/private endpoint after model-routing guardrails are enabled.",
    },
  ],
};

const strategyModelRoutes = [
  { value: "local_policy", labelKey: "modelRoutes.localPolicy", fallback: "Local Policy", badgeKey: "modelRoutes.localPolicyShort", badge: "Local" },
  { value: "codex_cli", labelKey: "modelRoutes.codexCli", fallback: "Codex CLI", badgeKey: "modelRoutes.codexCliShort", badge: "Codex" },
  { value: "claude_cli", labelKey: "modelRoutes.claudeCli", fallback: "Claude CLI", badgeKey: "modelRoutes.claudeCliShort", badge: "Claude" },
  { value: "compatible_endpoint", labelKey: "modelRoutes.compatibleEndpoint", fallback: "Local Endpoint", badgeKey: "modelRoutes.compatibleEndpointShort", badge: "Endpoint" },
];

const marketDataModes = [
  {
    value: "live",
    labelKey: "dataModes.live",
    fallback: "Live public",
    shortKey: "dataModes.liveShort",
    short: "Live",
    detailKey: "dataModes.liveDetail",
    detail: "Exchange API stream",
  },
  {
    value: "seed",
    labelKey: "dataModes.seed",
    fallback: "Local seed",
    shortKey: "dataModes.seedShort",
    short: "Seed",
    detailKey: "dataModes.seedDetail",
    detail: "Offline demo data",
  },
  {
    value: "replay",
    labelKey: "dataModes.replay",
    fallback: "Replay cache",
    shortKey: "dataModes.replayShort",
    short: "Replay",
    detailKey: "dataModes.replayDetail",
    detail: "Pause on historical cursor",
  },
];

function normalizeStrategyProfile(profile) {
  return { ...defaultStrategyProfile, ...(profile || {}) };
}

function modelRouteMeta(value) {
  return strategyModelRoutes.find((route) => route.value === value) || strategyModelRoutes[0];
}

function modelRouteLabel(t, value) {
  const route = modelRouteMeta(value);
  return t(route.labelKey, route.fallback);
}

function modelRouteBadge(t, value) {
  const route = modelRouteMeta(value);
  return t(route.badgeKey, route.badge);
}

function runModelProfile(run, index, fallback = defaultStrategyProfile.modelProfile) {
  const sampleRoutes = [fallback, "codex_cli", "local_policy", "claude_cli", "compatible_endpoint"];
  return run?.modelProfile || run?.modelRoute || sampleRoutes[index % sampleRoutes.length];
}

function marketDataModeMeta(value) {
  return marketDataModes.find((item) => item.value === value) || marketDataModes[0];
}

function marketDataModeLabel(t, value) {
  const item = marketDataModeMeta(value);
  return t(item.labelKey, item.fallback);
}

function marketDataModeShortLabel(t, value) {
  const item = marketDataModeMeta(value);
  return t(item.shortKey, item.short);
}

function marketDataModeDetail(t, value) {
  const item = marketDataModeMeta(value);
  return t(item.detailKey, item.detail);
}

const defaultVaultTestForm = {
  credentialId: "",
  passphrase: "",
  environment: "testnet",
  symbol: "BTCUSDT",
};

const chartTimeframes = ["1m", "5m", "15m", "1h", "4h", "1D"];
const scrollbarOverflowPattern = /^(auto|scroll|overlay)$/;
const overlayScrollbarIgnoreSelector = ".wide-table-scroll, .wide-table-rail, .custom-scrollbar-layer, .custom-scrollbar-layer *";

function GlobalScrollbars() {
  const [bars, setBars] = useState([]);
  const activeElementRef = useRef(null);
  const hideTimerRef = useRef(null);
  const frameRef = useRef(0);
  const dragRef = useRef(null);
  const updateRef = useRef(() => {});

  useEffect(() => {
    if (typeof document === "undefined") return undefined;

    const hide = () => {
      if (hideTimerRef.current) {
        window.clearTimeout(hideTimerRef.current);
        hideTimerRef.current = null;
      }
      activeElementRef.current = null;
      setBars([]);
    };

    const hideSoon = () => {
      if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
      hideTimerRef.current = window.setTimeout(hide, 700);
    };

    const scrollableAxis = (element) => {
      if (!(element instanceof HTMLElement)) return { x: false, y: false };
      if (element.matches(overlayScrollbarIgnoreSelector)) return { x: false, y: false };
      const styles = window.getComputedStyle(element);
      const isRootScroller = element === document.scrollingElement || element === document.documentElement || element === document.body;
      const rootAllowsX = isRootScroller && !/^(hidden|clip)$/.test(styles.overflowX);
      const rootAllowsY = isRootScroller && !/^(hidden|clip)$/.test(styles.overflowY);
      const x = (scrollbarOverflowPattern.test(styles.overflowX) || rootAllowsX) && element.scrollWidth > element.clientWidth + 2;
      const y = (scrollbarOverflowPattern.test(styles.overflowY) || rootAllowsY) && element.scrollHeight > element.clientHeight + 2;
      return { x, y };
    };

    const findScrollableElement = (target) => {
      let element = target instanceof Element ? target : target?.parentElement;
      if (element?.closest?.(".custom-scrollbar-layer")) return activeElementRef.current;
      const allowRootFallback = Boolean(element?.closest?.(".modal-backdrop"));
      while (element && element !== document.documentElement) {
        const axis = scrollableAxis(element);
        if (axis.x || axis.y) return element;
        element = element.parentElement;
      }
      if (!allowRootFallback) return null;
      const root = document.scrollingElement;
      const rootAxis = scrollableAxis(root);
      if (rootAxis.x || rootAxis.y) return root;
      return null;
    };

    const measure = (element) => {
      if (!(element instanceof HTMLElement) || !document.contains(element)) return [];
      const rect = element.getBoundingClientRect();
      const visible = rect.width > 8 && rect.height > 8 && rect.bottom > 0 && rect.right > 0 && rect.top < window.innerHeight && rect.left < window.innerWidth;
      if (!visible) return [];

      const axis = scrollableAxis(element);
      const next = [];
      if (axis.y) {
        const trackSize = Math.max(1, rect.height - 8);
        const thumbSize = Math.max(28, trackSize * (element.clientHeight / element.scrollHeight));
        const maxThumbOffset = Math.max(1, trackSize - thumbSize);
        const scrollMax = Math.max(1, element.scrollHeight - element.clientHeight);
        next.push({
          axis: "y",
          left: Math.min(window.innerWidth - 5, Math.max(2, rect.right - 5)),
          top: Math.max(2, rect.top + 4),
          width: 3,
          height: trackSize,
          thumbSize,
          thumbOffset: (element.scrollTop / scrollMax) * maxThumbOffset,
        });
      }
      if (axis.x) {
        const trackSize = Math.max(1, rect.width - 8);
        const thumbSize = Math.max(28, trackSize * (element.clientWidth / element.scrollWidth));
        const maxThumbOffset = Math.max(1, trackSize - thumbSize);
        const scrollMax = Math.max(1, element.scrollWidth - element.clientWidth);
        next.push({
          axis: "x",
          left: Math.max(2, rect.left + 4),
          top: Math.min(window.innerHeight - 5, Math.max(2, rect.bottom - 5)),
          width: trackSize,
          height: 3,
          thumbSize,
          thumbOffset: (element.scrollLeft / scrollMax) * maxThumbOffset,
        });
      }
      return next;
    };

    const scheduleUpdate = () => {
      if (frameRef.current) window.cancelAnimationFrame(frameRef.current);
      frameRef.current = window.requestAnimationFrame(() => {
        frameRef.current = 0;
        const element = activeElementRef.current;
        setBars(element ? measure(element) : []);
      });
    };

    const showFor = (element) => {
      if (!element) {
        hide();
        return;
      }
      if (hideTimerRef.current) {
        window.clearTimeout(hideTimerRef.current);
        hideTimerRef.current = null;
      }
      activeElementRef.current = element;
      scheduleUpdate();
    };

    const handlePointerMove = (event) => {
      const drag = dragRef.current;
      if (drag) {
        const element = drag.element;
        const pointer = drag.axis === "x" ? event.clientX : event.clientY;
        const next = drag.startScroll + (pointer - drag.startPointer) * drag.ratio;
        if (drag.axis === "x") {
          element.scrollLeft = Math.max(0, Math.min(drag.maxScroll, next));
        } else {
          element.scrollTop = Math.max(0, Math.min(drag.maxScroll, next));
        }
        scheduleUpdate();
        event.preventDefault();
        return;
      }
      const element = findScrollableElement(event.target);
      if (element) {
        showFor(element);
      } else {
        hide();
      }
    };

    const handlePointerActivate = (event) => {
      if (dragRef.current) return;
      const element = findScrollableElement(event.target);
      if (element) showFor(element);
    };

    const handleScroll = (event) => {
      const target = event.target === document ? document.scrollingElement : event.target;
      const element = findScrollableElement(target);
      if (!element || element !== activeElementRef.current) return;
      scheduleUpdate();
      hideSoon();
    };

    const handlePointerUp = () => {
      if (!dragRef.current) return;
      dragRef.current = null;
      scheduleUpdate();
    };

    updateRef.current = scheduleUpdate;
    document.addEventListener("pointerover", handlePointerActivate, true);
    document.addEventListener("pointerdown", handlePointerActivate, true);
    document.addEventListener("pointermove", handlePointerMove, true);
    document.addEventListener("scroll", handleScroll, true);
    document.addEventListener("pointerup", handlePointerUp, true);
    document.addEventListener("pointercancel", handlePointerUp, true);
    window.addEventListener("resize", scheduleUpdate);
    window.addEventListener("blur", hide);

    return () => {
      document.removeEventListener("pointerover", handlePointerActivate, true);
      document.removeEventListener("pointerdown", handlePointerActivate, true);
      document.removeEventListener("pointermove", handlePointerMove, true);
      document.removeEventListener("scroll", handleScroll, true);
      document.removeEventListener("pointerup", handlePointerUp, true);
      document.removeEventListener("pointercancel", handlePointerUp, true);
      window.removeEventListener("resize", scheduleUpdate);
      window.removeEventListener("blur", hide);
      if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
      if (frameRef.current) window.cancelAnimationFrame(frameRef.current);
    };
  }, []);

  function handlePointerDown(event) {
    const element = activeElementRef.current;
    const axis = event.currentTarget.dataset.axis;
    if (!(element instanceof HTMLElement) || !axis) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const thumb = event.currentTarget.querySelector(".custom-scrollbar-thumb")?.getBoundingClientRect();
    const trackSize = axis === "x" ? rect.width : rect.height;
    const thumbSize = axis === "x" ? thumb?.width || 28 : thumb?.height || 28;
    const maxScroll = axis === "x" ? element.scrollWidth - element.clientWidth : element.scrollHeight - element.clientHeight;
    const maxTrack = Math.max(1, trackSize - thumbSize);
    dragRef.current = {
      axis,
      element,
      maxScroll,
      ratio: maxScroll / maxTrack,
      startPointer: axis === "x" ? event.clientX : event.clientY,
      startScroll: axis === "x" ? element.scrollLeft : element.scrollTop,
    };
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    updateRef.current();
  }

  if (bars.length === 0) return null;

  return (
    <div className="custom-scrollbar-layer" aria-hidden="true">
      {bars.map((bar) => (
        <div
          key={bar.axis}
          className={classNames("custom-scrollbar", `custom-scrollbar-${bar.axis}`)}
          data-axis={bar.axis}
          onPointerDown={handlePointerDown}
          style={{
            left: `${bar.left}px`,
            top: `${bar.top}px`,
            width: `${bar.width}px`,
            height: `${bar.height}px`,
          }}
        >
          <span
            className="custom-scrollbar-thumb"
            style={bar.axis === "x"
              ? { width: `${bar.thumbSize}px`, transform: `translateX(${bar.thumbOffset}px)` }
              : { height: `${bar.thumbSize}px`, transform: `translateY(${bar.thumbOffset}px)` }}
          />
        </div>
      ))}
    </div>
  );
}

export function App() {
  const [locale, setLocale] = useState(() => {
    if (typeof window === "undefined") return "zh-CN";
    return resolveLocale(readStoredLocale() || window.navigator?.language);
  });
  const [theme, setTheme] = useState(() => readStoredTheme());
  const t = useMemo(() => makeTranslator(locale), [locale]);
  const [toast, setToast] = useState(null);
  const toastTimerRef = useRef(null);
  const skipNextBacktestAutoRunRef = useRef(false);
  const labModeInitializedRef = useRef(false);
  const [labState, setLabState] = useState(fallbackLabState);
  const [sourceStatus, setSourceStatus] = useState("loading");
  const [selectedRun, setSelectedRun] = useState(0);
  const [mode, setMode] = useState("Shadow");
  const [dataSource, setDataSource] = useState("Binance");
  const [marketDataMode, setMarketDataMode] = useState("live");
  const [workspaceTab, setWorkspaceTab] = useState("Real-time Sim");
  const [bottomTab, setBottomTab] = useState("Performance");
  const [eventFilter, setEventFilter] = useState("All");
  const [timeframe, setTimeframe] = useState("15m");
  const [showArchivedRuns, setShowArchivedRuns] = useState(false);
  const [isPaused, setIsPaused] = useState(false);
  const [isStopped, setIsStopped] = useState(false);
  const [isRunStopped, setIsRunStopped] = useState(false);
  const [killSwitch, setKillSwitch] = useState({ active: false, message: "kill switch clear" });
  const [isUpdatingKillSwitch, setIsUpdatingKillSwitch] = useState(false);
  const [replaySpeed, setReplaySpeed] = useState(1);
  const [replayCursor, setReplayCursor] = useState("");
  const [isSimulating, setIsSimulating] = useState(false);
  const [autopilot, setAutopilot] = useState(defaultAutopilotState);
  const [autopilotRuns, setAutopilotRuns] = useState([]);
  const [autopilotSteps, setAutopilotSteps] = useState([]);
  const [paperAccount, setPaperAccount] = useState(defaultPaperAccount);
  const [paperExecutions, setPaperExecutions] = useState([]);
  const [paperResetStatus, setPaperResetStatus] = useState({ tone: "warn", message: "Ready" });
  const [isResettingPaper, setIsResettingPaper] = useState(false);
  const [isPaperResetConfirmOpen, setIsPaperResetConfirmOpen] = useState(false);
  const [paperResetPhrase, setPaperResetPhrase] = useState("");
  const [isUpdatingAutopilot, setIsUpdatingAutopilot] = useState(false);
  const autopilotRunAtRef = useRef("");
  const [credentials, setCredentials] = useState([]);
  const [credentialStatus, setCredentialStatus] = useState({ tone: "loading", message: "Loading" });
  const [isCredentialPanelOpen, setIsCredentialPanelOpen] = useState(false);
  const [credentialForm, setCredentialForm] = useState(defaultCredentialForm);
  const [isSavingCredential, setIsSavingCredential] = useState(false);
  const [credentialDeleteCandidateId, setCredentialDeleteCandidateId] = useState(null);
  const [deletingCredentialId, setDeletingCredentialId] = useState(null);
  const [vaultTestForm, setVaultTestForm] = useState(defaultVaultTestForm);
  const [vaultTestStatus, setVaultTestStatus] = useState({ tone: "warn", message: "Not tested" });
  const [vaultTestResult, setVaultTestResult] = useState(null);
  const [vaultTestError, setVaultTestError] = useState("");
  const [isTestingCredential, setIsTestingCredential] = useState(false);
  const [liveGuard, setLiveGuard] = useState({ unlocked: false, message: "loading" });
  const [auditLog, setAuditLog] = useState({ entries: [], verification: { valid: true, checked: 0 } });
  const [isLiveGuardOpen, setIsLiveGuardOpen] = useState(false);
  const [liveGuardForm, setLiveGuardForm] = useState(defaultLiveGuardForm);
  const [liveGuardStatus, setLiveGuardStatus] = useState({ tone: "loading", message: "Loading" });
  const [isUpdatingLiveGuard, setIsUpdatingLiveGuard] = useState(false);
  const [liveExecutionForm, setLiveExecutionForm] = useState(defaultLiveExecutionForm);
  const [liveExecutionStatus, setLiveExecutionStatus] = useState({ tone: "warn", message: "Guarded" });
  const [liveExecutionResult, setLiveExecutionResult] = useState(null);
  const [liveExecutions, setLiveExecutions] = useState([]);
  const [liveReconciliations, setLiveReconciliations] = useState([]);
  const [riskProfile, setRiskProfile] = useState(defaultRiskProfile);
  const [riskProfileStatus, setRiskProfileStatus] = useState({ tone: "loading", message: "Loading" });
  const [isSavingRiskProfile, setIsSavingRiskProfile] = useState(false);
  const [strategyProfile, setStrategyProfile] = useState(defaultStrategyProfile);
  const [strategyProfileStatus, setStrategyProfileStatus] = useState({ tone: "loading", message: "Loading" });
  const [isStrategyPanelOpen, setIsStrategyPanelOpen] = useState(false);
  const [isAIConfigOpen, setIsAIConfigOpen] = useState(false);
  const [isSavingStrategyProfile, setIsSavingStrategyProfile] = useState(false);
  const [reconciliationStatus, setReconciliationStatus] = useState({ tone: "warn", message: "No checks" });
  const [reconcilingId, setReconcilingId] = useState(null);
  const [isExecutingLive, setIsExecutingLive] = useState(false);
  const [accountSyncStatus, setAccountSyncStatus] = useState({ tone: "warn", message: "Not synced" });
  const [accountSnapshot, setAccountSnapshot] = useState(null);
  const [accountSnapshotMeta, setAccountSnapshotMeta] = useState(null);
  const [isSyncingAccount, setIsSyncingAccount] = useState(false);
  const [workspaceExportStatus, setWorkspaceExportStatus] = useState({ tone: "warn", message: "Ready" });
  const [isExportingWorkspace, setIsExportingWorkspace] = useState(false);
  const [localData, setLocalData] = useState(defaultLocalData);
  const [localDataStatus, setLocalDataStatus] = useState({ tone: "warn", message: "Ready" });
  const [localDataPhrase, setLocalDataPhrase] = useState("");
  const [isPruningLocalData, setIsPruningLocalData] = useState(false);
  const [backtestResult, setBacktestResult] = useState(null);
  const [backtestRuns, setBacktestRuns] = useState([]);
  const [backtestStatus, setBacktestStatus] = useState({ tone: "warn", message: "Ready" });
  const [isRunningBacktest, setIsRunningBacktest] = useState(false);
  const [appInfo, setAppInfo] = useState(defaultAppInfo);
  const [preflight, setPreflight] = useState(defaultPreflight);
  const [aiProviders, setAIProviders] = useState(defaultAIProviders);
  const [aiProvidersStatus, setAIProvidersStatus] = useState({ tone: "warn", message: "Ready" });

  function notify(message, tone = "info") {
    if (toastTimerRef.current) {
      window.clearTimeout(toastTimerRef.current);
    }
    setToast({ id: Date.now(), tone, message });
    toastTimerRef.current = window.setTimeout(() => {
      setToast(null);
      toastTimerRef.current = null;
    }, 3200);
  }

  useEffect(() => () => {
    if (toastTimerRef.current) {
      window.clearTimeout(toastTimerRef.current);
    }
  }, []);

  useEffect(() => {
    if (typeof document !== "undefined") {
      document.documentElement.lang = locale;
    }
    if (typeof window !== "undefined") {
      writeStoredLocale(locale);
    }
  }, [locale]);

  useEffect(() => {
    const resolvedTheme = resolveTheme(theme);
    if (typeof document !== "undefined") {
      document.documentElement.dataset.theme = resolvedTheme;
      document.documentElement.style.colorScheme = resolvedTheme;
    }
    writeStoredTheme(resolvedTheme);
  }, [theme]);

  useEffect(() => {
    let active = true;
    if (marketDataMode === "seed") {
      setSourceStatus("fallback");
      setLabState({
        ...fallbackLabState,
        meta: {
          ...fallbackLabState.meta,
          dataSource: "Local seed",
          selectedMarket: labState?.meta?.selectedMarket || fallbackLabState.meta.selectedMarket,
        },
      });
      return () => {
        active = false;
      };
    }
    if (marketDataMode === "replay") {
      setSourceStatus("fallback");
      setIsPaused(true);
      setLabState((current) => ({
        ...current,
        meta: {
          ...current.meta,
          dataSource: "Replay cache",
        },
      }));
      return () => {
        active = false;
      };
    }
    setSourceStatus("loading");
    loadLabState({ exchange: dataSource, symbol: labState?.meta?.selectedMarket || "BTCUSDT" }).then(({ data, source }) => {
      if (!active) return;
      setLabState(data);
      if (!labModeInitializedRef.current) {
        setMode(data.meta.mode);
        labModeInitializedRef.current = true;
      }
      setSourceStatus(source);
    });
    return () => {
      active = false;
    };
  }, [dataSource, marketDataMode]);

  useEffect(() => {
    refreshCredentials();
    refreshLiveGuard();
    refreshKillSwitch();
    refreshAutopilot();
    refreshAutopilotRuns();
    refreshAutopilotSteps();
    refreshPaperAccount();
    refreshPaperExecutions();
    refreshAuditLog();
    refreshLiveExecutions();
    refreshLiveReconciliations();
    refreshRiskProfile();
    refreshStrategyProfile();
    refreshBacktestRuns();
    refreshLocalData();
    refreshAppInfo();
    refreshPreflight();
    refreshAIProviders({ silent: true });
  }, []);

  useEffect(() => {
    if (!isAIConfigOpen) return;
    refreshAIProviders({ silent: true });
  }, [isAIConfigOpen]);

  useEffect(() => {
    if (workspaceTab === "Backtest" && !backtestResult && !isRunningBacktest) {
      if (skipNextBacktestAutoRunRef.current) {
        skipNextBacktestAutoRunRef.current = false;
        return;
      }
      handleRunBacktest();
    }
  }, [workspaceTab]);

  useEffect(() => {
    if (!autopilot?.running) return;
    const timer = window.setInterval(() => {
      refreshAutopilot({ silent: true });
      refreshAutopilotSteps();
      refreshPaperAccount();
      refreshPaperExecutions({ silent: true });
    }, 5000);
    return () => window.clearInterval(timer);
  }, [autopilot?.running]);

  useEffect(() => {
    if (credentials.length === 0) {
      setLiveExecutionForm((current) => ({ ...current, credentialId: "" }));
      setVaultTestForm(defaultVaultTestForm);
      setVaultTestResult(null);
      setVaultTestStatus({ tone: "warn", message: "Not tested" });
      setAccountSnapshot(null);
      setAccountSnapshotMeta(null);
      setAccountSyncStatus({ tone: "warn", message: "Not synced" });
      return;
    }
    setLiveExecutionForm((current) => {
      if (current.credentialId && credentials.some((credential) => String(credential.id) === String(current.credentialId))) {
        return current;
      }
      return {
        ...current,
        credentialId: String(credentials[0].id),
        exchange: credentials[0].exchange,
      };
    });
    setVaultTestForm((current) => {
      if (current.credentialId && credentials.some((credential) => String(credential.id) === String(current.credentialId))) {
        return current;
      }
      return {
        ...current,
        credentialId: String(credentials[0].id),
        environment: credentials[0].exchange === "OKX" ? "demo" : current.environment || "testnet",
      };
    });
  }, [credentials]);

  useEffect(() => {
    if (!isLiveGuardOpen) return;
    refreshAccountSnapshot({ silent: true });
    refreshLiveExecutions();
    refreshLiveReconciliations();
  }, [
    isLiveGuardOpen,
    liveExecutionForm.credentialId,
    liveExecutionForm.exchange,
    liveExecutionForm.symbol,
    liveGuard?.environment,
    liveGuard?.unlocked,
    liveGuardForm.environment,
  ]);

  const visibleEvents = useMemo(() => {
    if (!labState) return [];
    if (eventFilter === "All") return labState.events;
    if (eventFilter === "Risk") {
      return labState.events.filter((event) => event.type.includes("Risk") || event.level === "danger");
    }
    return labState.events.filter((event) => event.type.includes(eventFilter));
  }, [eventFilter, labState]);

  const replayOptions = useMemo(() => buildReplayOptions(t, labState, backtestResult), [t, labState, backtestResult]);
  const replayPoint = replayOptions.find((option) => option.value === replayCursor) || replayOptions[0] || null;

  function handleLocaleChange(nextLocale) {
    const resolved = resolveLocale(nextLocale);
    const nextT = makeTranslator(resolved);
    setLocale(resolved);
    notify(
      resolved === "zh-CN"
        ? nextT("toast.switchedChinese", "Switched to Chinese")
        : nextT("toast.switchedEnglish", "Switched to English"),
      "success",
    );
  }

  function handleThemeToggle() {
    const next = theme === "light" ? "dark" : "light";
    setTheme(next);
    notify(
      t("toast.themeChanged", "Theme switched to {value}", {
        value: next === "light" ? t("top.themeLight", "Light") : t("top.themeDark", "Dark"),
      }),
      "success",
    );
  }

  function notifyBlocked(message) {
    notify(message || t("toast.actionUnavailable", "Action is not ready yet"), "warn");
  }

  function handleDataSourceChange(value) {
    if (value === dataSource) {
      notify(t("toast.exchangeAlreadyActive", "{value} is already active", { value }), "info");
      return;
    }
    setDataSource(value);
    notify(t("toast.exchangeChanged", "Exchange switched to {value}", { value }), "success");
  }

  function handleMarketDataModeChange(value) {
    const nextMode = marketDataModeMeta(value).value;
    const label = marketDataModeLabel(t, nextMode);
    if (nextMode === marketDataMode) {
      notify(t("toast.dataModeAlreadyActive", "{value} data mode is already active", { value: label }), "info");
      return;
    }
    setMarketDataMode(nextMode);
    if (nextMode === "replay") {
      setIsPaused(true);
    }
    notify(t("toast.dataModeChanged", "Data mode switched to {value}", { value: label }), nextMode === "live" ? "success" : "warn");
  }

  function handleModeChange(nextMode) {
    labModeInitializedRef.current = true;
    if (nextMode === mode) {
      notify(t("toast.modeAlreadyActive", "{value} mode is already active", { value: choiceLabel(t, nextMode) }), "info");
      if (nextMode === "Live") {
        setIsLiveGuardOpen(true);
      }
      return;
    }
    setMode(nextMode);
    notify(t("toast.modeChanged", "Mode switched to {value}", { value: choiceLabel(t, nextMode) }), nextMode === "Live" ? "warn" : "success");
    if (nextMode === "Live") {
      setIsLiveGuardOpen(true);
      refreshLiveGuard();
      refreshAuditLog();
      refreshLiveExecutions();
    }
  }

  function handleWorkspaceTabChange(tab) {
    if (tab === workspaceTab) {
      return;
    }
    setWorkspaceTab(tab);
  }

  function handleBottomTabChange(tab) {
    if (tab === bottomTab) {
      return;
    }
    setBottomTab(tab);
  }

  function handleEventFilterChange(filter) {
    if (filter === eventFilter) {
      notify(t("toast.filterAlreadyActive", "{value} filter is already active", { value: choiceLabel(t, filter) }), "info");
      return;
    }
    setEventFilter(filter);
    notify(t("toast.filterChanged", "Event filter: {value}", { value: choiceLabel(t, filter) }), "info");
  }

  function handleTimeframeChange(nextTimeframe) {
    if (nextTimeframe === timeframe) {
      notify(t("toast.timeframeAlreadyActive", "{value} timeframe is already active", { value: nextTimeframe }), "info");
      return;
    }
    setTimeframe(nextTimeframe);
    notify(t("toast.timeframeChanged", "Timeframe switched to {value}", { value: nextTimeframe }), "success");
  }

  function handleFeatureNotice(label, message) {
    notify(message || t("toast.openStrategyForConfig", "{label}: use Strategy Profile for configuration", { label }), "info");
  }

  function handleRestartRun() {
    setIsPaused(false);
    setIsRunStopped(false);
    setReplaySpeed(1);
    notify(t("toast.runRestarted", "Simulation controls reset"), "success");
  }

  function handlePauseToggle() {
    setIsPaused((value) => {
      const next = !value;
      notify(next ? t("toast.runPaused", "Simulation paused") : t("toast.runResumed", "Simulation resumed"), next ? "warn" : "success");
      return next;
    });
  }

  function handleReplayJump(option) {
    if (!option) {
      notify(t("toast.jumpHint", "Jump-to-time uses replay history; choose a backtest or run AI step first"), "warn");
      return;
    }
    setReplayCursor(option.value);
    setIsPaused(true);
    notify(t("toast.replayJumped", "Replay cursor moved to {value}", { value: option.label }), "success");
  }

  function handleRunStopToggle() {
    setIsRunStopped((value) => {
      const next = !value;
      notify(next ? t("toast.runStopped", "Run stopped") : t("toast.runResumed", "Simulation resumed"), next ? "warn" : "success");
      return next;
    });
  }

  function buildAIContextSnapshot() {
    const verdict = labState.verdict || {};
    const featureLines = (labState.features || [])
      .map((feature) => `- ${feature.name}: ${feature.value > 0 ? "+" : ""}${feature.value.toFixed(2)} (${feature.impact})`)
      .join("\n");
    return [
      "CCVar Quant Lab AI context",
      `Time: ${new Date().toISOString()}`,
      `Mode: ${mode}`,
      `Exchange: ${strategyProfile.exchange || dataSource}`,
      `Market data: ${marketDataModeLabel(t, marketDataMode)}`,
      `Symbol: ${strategyProfile.symbol || labState.meta.selectedMarket || "BTCUSDT"}`,
      `Strategy: ${strategyProfile.name || labState.meta.strategy}`,
      `Model route: ${modelRouteLabel(t, strategyProfile.modelProfile)} (fallback ${modelRouteLabel(t, strategyProfile.modelFallback)})`,
      `Strategy concurrency: ${Number(strategyProfile.concurrency || defaultStrategyProfile.concurrency)} workers, routed through one local risk guard`,
      `Intent: ${String(strategyProfile.side || "buy").toUpperCase()} ${formatMoney(Number(strategyProfile.orderSizeUsdt || 0))} USDT`,
      `Risk guard: max order ${formatMoney(Number(riskProfile.maxOrderUsdt || 0))} USDT, min confidence ${Number(riskProfile.minConfidence || 0).toFixed(2)}`,
      `Current signal: ${verdict.signal || "-"} / confidence ${verdict.confidence || "-"}% / regime ${verdict.regime || "-"}`,
      `Reasoning: ${verdict.reasoning || "-"}`,
      "Feature impacts:",
      featureLines || "- no feature data",
      "",
      "Please produce a cautious trading analysis only. Do not assume production/mainnet execution is allowed. Respect the local risk guardrails and identify missing evidence before suggesting an action.",
    ].join("\n");
  }

  async function handleCopyAIContext() {
    const text = buildAIContextSnapshot();
    const copied = await copyTextToClipboard(text);
    notify(
      copied ? t("toast.aiContextCopied", "AI context copied") : t("toast.copyFailed", "Copy failed. Try again from the desktop menu."),
      copied ? "success" : "danger"
    );
  }

  async function handleCopyAICommand(command) {
    if (!command) {
      notify(t("toast.actionUnavailable", "Action is not ready yet"), "warn");
      return;
    }
    const copied = await copyTextToClipboard(command);
    notify(
      copied ? t("toast.commandCopied", "Command copied") : t("toast.copyFailed", "Copy failed. Try again from the desktop menu."),
      copied ? "success" : "danger"
    );
  }

  function handleRunSelect(index) {
    setSelectedRun(index);
    const run = labState.runs[index];
    if (run) {
      notify(t("toast.runSelected", "Selected {name}", { name: run.name }), "success");
    }
  }

  function handleToggleArchivedRuns() {
    setShowArchivedRuns((value) => {
      const next = !value;
      notify(next ? t("toast.archivedShown", "Archived run drawer opened") : t("toast.archivedHidden", "Archived run drawer closed"), "info");
      return next;
    });
  }

  function handleCreateNewRun() {
    skipNextBacktestAutoRunRef.current = workspaceTab !== "Backtest";
    setWorkspaceTab("Backtest");
    setBottomTab("Performance");
    setBacktestResult(null);
    setBacktestStatus({ tone: "warn", message: "Draft" });
    setIsStrategyPanelOpen(true);
    notify(t("toast.newRunStarted", "New run draft opened"), "success");
  }

  async function handleSimStep() {
    if (isStopped || isRunStopped) {
      notifyBlocked(t("toast.resumeBeforeAiStep", "Resume the run before starting an AI step"));
      return;
    }
    setIsSimulating(true);
    try {
      const result = await simulateStep({
        exchange: strategyProfile.exchange || dataSource,
        symbol: strategyProfile.symbol || labState.meta.selectedMarket || "BTCUSDT",
        mode: mode === "Paper" ? "paper" : "shadow",
      });
      setLabState((current) => ({
        ...current,
        events: [...result.events, ...current.events].slice(0, 12),
      }));
      await refreshPaperAccount();
      await refreshPaperExecutions();
      setSourceStatus("api");
      notify(t("toast.aiStepComplete", "AI step completed"), "success");
    } catch (error) {
      const now = new Date();
      const message = localizedErrorDetail(t, error);
      setLabState((current) => ({
        ...current,
        events: [
          {
            time: now.toLocaleTimeString("en-GB", { hour12: false }),
            type: "Sim Step",
            symbol: current.meta.selectedMarket,
            action: "-",
            price: 0,
            result: "Failed",
            note: message,
            level: "danger",
          },
          ...current.events,
        ].slice(0, 12),
      }));
      setSourceStatus("fallback");
      notifyError(notify, t, error, "simStep");
    } finally {
      setIsSimulating(false);
    }
  }

  async function handlePaperReset() {
    if (isResettingPaper) return;
    if (isSimulating || autopilot?.running) {
      setPaperResetStatus({ tone: "warn", message: "Stop AI first" });
      notify(t("toast.stopAiFirst", "Stop AI before resetting paper ledger"), "warn");
      return;
    }
    setPaperResetPhrase("");
    setPaperResetStatus({ tone: "warn", message: "Confirm reset" });
    setIsPaperResetConfirmOpen(true);
  }

  function handleClosePaperResetConfirm() {
    if (isResettingPaper) return;
    setIsPaperResetConfirmOpen(false);
    setPaperResetPhrase("");
    setPaperResetStatus({ tone: "warn", message: "Ready" });
  }

  async function handleConfirmPaperReset() {
    if (isResettingPaper) return;
    const phrase = paperResetPhrase.trim();
    if (phrase !== "RESET PAPER") {
      setPaperResetStatus({ tone: "warn", message: "Phrase mismatch" });
      notify(t("toast.resetPaperPhraseMismatch", "Type RESET PAPER to confirm the reset"), "warn");
      return;
    }
    setIsResettingPaper(true);
    setPaperResetStatus({ tone: "loading", message: "Resetting" });
    try {
      const payload = await resetPaperExecutions({
        operator: liveGuardForm.operator || "local",
        reason: "manual paper reset from UI",
        phrase,
      });
      await refreshPaperAccount();
      await refreshPaperExecutions();
      await refreshAuditLog();
      setPaperResetStatus({ tone: "success", message: `Reset ${payload.deletedRecords || 0}` });
      setIsPaperResetConfirmOpen(false);
      setPaperResetPhrase("");
      notify(t("toast.paperReset", "Paper ledger reset"), "success");
    } catch (error) {
      setPaperResetStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "paperReset");
      await refreshAuditLog();
    } finally {
      setIsResettingPaper(false);
    }
  }

  async function refreshCredentials() {
    setCredentialStatus({ tone: "loading", message: "Loading" });
    try {
      const payload = await loadCredentials();
      setCredentials(payload.credentials || []);
      setCredentialStatus({ tone: "success", message: "Local vault" });
    } catch (error) {
      setCredentials([]);
      setCredentialStatus({ tone: "warn", message: localizedErrorMessage(t, error, "credentialLoad") });
    }
  }

  function openCredentialPanel() {
    setCredentialForm((current) => ({
      ...defaultCredentialForm,
      exchange: dataSource,
      label: current.label || `${dataSource} main key`,
      permissions: { ...defaultCredentialForm.permissions },
    }));
    setIsCredentialPanelOpen(true);
  }

  function closeCredentialPanel() {
    setCredentialDeleteCandidateId(null);
    setIsCredentialPanelOpen(false);
  }

  function updateVaultTestForm(updater) {
    setVaultTestForm((current) => (typeof updater === "function" ? updater(current) : updater));
    if (!isTestingCredential) {
      setVaultTestResult(null);
      setVaultTestError("");
      setVaultTestStatus({ tone: "warn", message: "Not tested" });
    }
  }

  async function handleCredentialSave(event) {
    event.preventDefault();
    setIsSavingCredential(true);
    setCredentialStatus({ tone: "loading", message: "Encrypting" });
    try {
      const saved = await saveCredential(credentialForm);
      setCredentials((current) => [saved, ...current]);
      setCredentialForm((current) => ({
        ...defaultCredentialForm,
        exchange: current.exchange,
        label: current.label,
      }));
      setCredentialStatus({ tone: "success", message: "Saved encrypted" });
      notify(t("toast.credentialSaved", "Credential saved encrypted"), "success");
      await refreshPreflight();
    } catch (error) {
      setCredentialStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "credentialSave");
    } finally {
      setIsSavingCredential(false);
    }
  }

  function handleCredentialDeleteRequest(id) {
    if (deletingCredentialId) return;
    setCredentialDeleteCandidateId(id);
  }

  function handleCredentialDeleteCancel() {
    if (deletingCredentialId) return;
    setCredentialDeleteCandidateId(null);
  }

  async function handleCredentialDelete(id) {
    if (deletingCredentialId) return;
    setDeletingCredentialId(id);
    setCredentialStatus({ tone: "loading", message: "Deleting" });
    try {
      await deleteCredential(id);
      setCredentials((current) => current.filter((credential) => credential.id !== id));
      setVaultTestForm((current) => (String(current.credentialId) === String(id) ? { ...current, credentialId: "" } : current));
      setCredentialDeleteCandidateId(null);
      setCredentialStatus({ tone: "success", message: "Deleted" });
      notify(t("toast.credentialDeleted", "Credential deleted"), "success");
      await refreshPreflight();
    } catch (error) {
      setCredentialStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "credentialDelete");
    } finally {
      setDeletingCredentialId(null);
    }
  }

  async function handleVaultConnectionTest() {
    if (isTestingCredential) return;
    setVaultTestResult(null);
    setVaultTestError("");
    const credential = credentials.find((item) => String(item.id) === String(vaultTestForm.credentialId));
    if (!credential) {
      setVaultTestStatus({ tone: "warn", message: "Select key" });
      notify(t("toast.selectKey", "Select a saved key first"), "warn");
      return;
    }
    if (!vaultTestForm.passphrase) {
      setVaultTestStatus({ tone: "warn", message: "Passphrase required" });
      notify(t("toast.passphraseRequired", "Passphrase required"), "warn");
      return;
    }
    const environment = credential.exchange === "OKX" ? "demo" : vaultTestForm.environment || "testnet";
    setIsTestingCredential(true);
    setVaultTestStatus({ tone: "loading", message: `Testing ${credential.exchange}` });
    try {
      const result = await syncAccount({
        credentialId: Number(credential.id),
        passphrase: vaultTestForm.passphrase,
        exchange: credential.exchange,
        environment,
        symbol: vaultTestForm.symbol || "BTCUSDT",
        operator: liveGuardForm.operator || "local",
      });
      setVaultTestResult(result);
      setVaultTestError("");
      setVaultTestStatus({
        tone: result.snapshot?.canTrade ? "success" : "warn",
        message: `${result.snapshot?.canTrade ? "Can trade" : "Read only"} · ${result.snapshot?.balances?.length || 0}/${result.snapshot?.openOrders?.length || 0}`,
      });
      if (String(liveExecutionForm.credentialId) === String(credential.id)) {
        setAccountSnapshot(result.snapshot);
        setAccountSnapshotMeta({
          snapshotId: result.snapshotId,
          credentialId: result.credential?.id,
          persistedAt: result.persistedAt,
        });
        setAccountSyncStatus({
          tone: "success",
          message: `Saved #${result.snapshotId || "-"} · ${result.snapshot.balances?.length || 0}/${result.snapshot.openOrders?.length || 0}`,
        });
      }
      await refreshAuditLog();
      await refreshPreflight();
      notify(t("toast.connectionTested", "Connection test finished"), result.snapshot?.canTrade ? "success" : "warn");
    } catch (error) {
      setVaultTestResult(null);
      setVaultTestError(localizedErrorDetail(t, error));
      setVaultTestStatus({ tone: "danger", message: "Connection failed" });
      notifyError(notify, t, error, "vaultTest");
      await refreshAuditLog();
      await refreshPreflight();
    } finally {
      setIsTestingCredential(false);
    }
  }

  async function refreshLiveGuard() {
    try {
      const state = await loadLiveGuard();
      setLiveGuard(state);
      setLiveGuardStatus({
        tone: state.unlocked ? "success" : "warn",
        message: state.unlocked ? "Unlocked" : state.message || "Locked",
      });
    } catch (error) {
      const message = localizedErrorDetail(t, error);
      setLiveGuard({ unlocked: false, message });
      setLiveGuardStatus({ tone: "danger", message: localizedErrorMessage(t, error, "liveGuardLoad") });
    }
  }

  async function refreshKillSwitch() {
    try {
      const state = await loadKillSwitch();
      setKillSwitch(state);
      setIsStopped(Boolean(state.active));
    } catch {
      setKillSwitch({ active: false, message: "kill switch unavailable" });
    }
  }

  function absorbAutopilotState(state) {
    setAutopilot(state || defaultAutopilotState);
    if (state?.lastRunAt && state.lastRunAt !== autopilotRunAtRef.current && (state.lastEvents || []).length > 0) {
      autopilotRunAtRef.current = state.lastRunAt;
      setLabState((current) => ({
        ...current,
        events: [...state.lastEvents, ...current.events].slice(0, 12),
      }));
      setSourceStatus("api");
      refreshAutopilotRuns();
      refreshAutopilotSteps();
    }
  }

  async function refreshAutopilot({ silent = false } = {}) {
    try {
      const state = await loadAutopilot();
      absorbAutopilotState(state);
    } catch (error) {
      if (!silent) {
        setAutopilot((current) => ({
          ...current,
          running: false,
          lastStatus: "unavailable",
          message: localizedErrorDetail(t, error),
        }));
      }
    }
  }

  async function refreshAutopilotRuns() {
    try {
      const payload = await loadAutopilotRuns({ limit: 4 });
      setAutopilotRuns(payload.records || []);
    } catch {
      setAutopilotRuns([]);
    }
  }

  async function refreshAutopilotSteps() {
    try {
      const payload = await loadAutopilotSteps({ limit: 6 });
      setAutopilotSteps(payload.records || []);
    } catch {
      setAutopilotSteps([]);
    }
  }

  async function refreshBacktestRuns() {
    try {
      const payload = await loadBacktestRuns({ limit: 6 });
      setBacktestRuns(payload.records || []);
    } catch {
      setBacktestRuns([]);
    }
  }

  async function refreshPaperExecutions() {
    try {
      const payload = await loadPaperExecutions({ limit: 8 });
      setPaperExecutions(payload.records || []);
    } catch {
      setPaperExecutions([]);
    }
  }

  async function refreshPaperAccount() {
    try {
      const payload = await loadPaperAccount();
      setPaperAccount(payload || defaultPaperAccount);
    } catch {
      setPaperAccount(defaultPaperAccount);
    }
  }

  async function refreshAuditLog() {
    try {
      const payload = await loadAuditLog({ limit: 12 });
      setAuditLog(payload);
    } catch (error) {
      setAuditLog({
        entries: [],
        verification: { valid: false, checked: 0, error: localizedErrorDetail(t, error) },
      });
    }
  }

  async function refreshLocalData() {
    try {
      const payload = await loadLocalData();
      setLocalData({
        summary: payload.summary || defaultLocalData.summary,
        keep: payload.keep || defaultLocalData.keep,
        protected: payload.protected || [],
      });
      setLocalDataStatus({ tone: "success", message: "Loaded" });
    } catch (error) {
      setLocalData(defaultLocalData);
      setLocalDataStatus({ tone: "danger", message: localizedErrorMessage(t, error, "localDataLoad") });
    }
  }

  async function refreshAppInfo() {
    try {
      const payload = await loadAppInfo();
      setAppInfo(payload || defaultAppInfo);
    } catch {
      setAppInfo(defaultAppInfo);
    }
  }

  async function refreshPreflight() {
    try {
      const payload = await loadPreflight();
      setPreflight(payload || defaultPreflight);
    } catch (error) {
      setPreflight({
        ...defaultPreflight,
        overall: "block",
        block: 1,
        checks: [{
          id: "preflight",
          label: "Preflight",
          status: "block",
          summary: localizedErrorDetail(t, error),
        }],
      });
    }
  }

  async function refreshAIProviders({ silent = false } = {}) {
    if (!silent) {
      setAIProvidersStatus({ tone: "loading", message: "Loading" });
    }
    try {
      const payload = await loadAIProviders();
      setAIProviders(payload || defaultAIProviders);
      setAIProvidersStatus({ tone: "success", message: "Loaded" });
      if (!silent) {
        notify(t("toast.aiProvidersRefreshed", "AI providers refreshed"), "success");
      }
    } catch (error) {
      setAIProviders(defaultAIProviders);
      setAIProvidersStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      if (!silent) {
        notifyError(notify, t, error, "aiProviders");
      }
    }
  }

  async function refreshLiveExecutions() {
    try {
      const payload = await loadLiveExecutions({ limit: 6 });
      setLiveExecutions(payload.records || []);
    } catch {
      setLiveExecutions([]);
    }
  }

  async function refreshLiveReconciliations() {
    try {
      const payload = await loadLiveReconciliations({ limit: 6 });
      setLiveReconciliations(payload.records || []);
      if ((payload.records || []).length > 0) {
        const latest = payload.records[0];
        setReconciliationStatus({ tone: "success", message: `${latest.status} #${latest.liveExecutionId}` });
      }
    } catch {
      setLiveReconciliations([]);
    }
  }

  async function refreshRiskProfile() {
    try {
      const profile = await loadRiskProfile();
      setRiskProfile(profile || defaultRiskProfile);
      setRiskProfileStatus({ tone: "success", message: "Saved" });
    } catch (error) {
      setRiskProfileStatus({ tone: "danger", message: localizedErrorMessage(t, error, "riskLoad") });
    }
  }

  function setRiskProfileField(field, value) {
    setRiskProfile((current) => ({ ...current, [field]: value }));
    setRiskProfileStatus({ tone: "warn", message: "Unsaved" });
  }

  async function handleSaveRiskProfile() {
    setIsSavingRiskProfile(true);
    setRiskProfileStatus({ tone: "loading", message: "Saving" });
    try {
      const profile = await saveRiskProfile(riskProfile);
      setRiskProfile(profile);
      setRiskProfileStatus({ tone: "success", message: "Saved" });
      setLiveGuardForm((current) => ({
        ...current,
        maxOrderUsdt: Math.min(Number(current.maxOrderUsdt || profile.maxOrderUsdt), profile.maxOrderUsdt),
      }));
      await refreshAuditLog();
    } catch (error) {
      setRiskProfileStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "riskSave");
    } finally {
      setIsSavingRiskProfile(false);
    }
  }

  async function refreshStrategyProfile() {
    try {
      const profile = await loadStrategyProfile();
      setStrategyProfile(normalizeStrategyProfile(profile));
      setStrategyProfileStatus({ tone: "success", message: "Saved" });
    } catch (error) {
      setStrategyProfileStatus({ tone: "danger", message: localizedErrorMessage(t, error, "strategyLoad") });
    }
  }

  function setStrategyProfileField(field, value) {
    setStrategyProfile((current) => ({ ...current, [field]: value }));
    setStrategyProfileStatus({ tone: "warn", message: "Unsaved" });
    setBacktestStatus({ tone: "warn", message: "Run needed" });
  }

  async function handleSaveStrategyProfile() {
    setIsSavingStrategyProfile(true);
    setStrategyProfileStatus({ tone: "loading", message: "Saving" });
    try {
      const profile = await saveStrategyProfile(strategyProfile);
      const normalized = normalizeStrategyProfile(profile);
      setStrategyProfile(normalized);
      setStrategyProfileStatus({ tone: "success", message: "Saved" });
      setDataSource(normalized.exchange);
      setLiveExecutionForm((current) => ({
        ...current,
        exchange: normalized.exchange,
        symbol: normalized.symbol,
        side: normalized.side,
        sizeUsdt: normalized.orderSizeUsdt,
      }));
      const { data, source } = await loadLabState({ exchange: normalized.exchange, symbol: normalized.symbol });
      setLabState(data);
      if (!labModeInitializedRef.current) {
        setMode(data.meta.mode);
        labModeInitializedRef.current = true;
      }
      setSourceStatus(source);
      setBacktestResult(null);
      setBacktestStatus({ tone: "warn", message: "Run needed" });
      await refreshAuditLog();
      notify(t("toast.strategySaved", "Strategy saved"), "success");
    } catch (error) {
      setStrategyProfileStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "strategySave");
    } finally {
      setIsSavingStrategyProfile(false);
    }
  }

  async function handleRunBacktest() {
    if (isRunningBacktest) return;
    setIsRunningBacktest(true);
    setBacktestStatus({ tone: "loading", message: "Running" });
    try {
      const result = await runBacktest({
        exchange: strategyProfile.exchange || dataSource,
        symbol: strategyProfile.symbol || labState.meta.selectedMarket || "BTCUSDT",
        side: strategyProfile.side,
        orderSizeUsdt: Number(strategyProfile.orderSizeUsdt || 500),
        interval: timeframe,
        limit: 200,
        fastWindow: 6,
        slowWindow: 18,
      });
      setBacktestResult(result);
      setBacktestStatus({ tone: "success", message: `${result.summary.tradeCount} trades` });
      setSourceStatus("api");
      await refreshBacktestRuns();
      notify(t("toast.backtestComplete", "Backtest completed"), "success");
    } catch (error) {
      setBacktestStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notify(localizedErrorMessage(t, error, "backtest"), "danger");
    } finally {
      setIsRunningBacktest(false);
    }
  }

  async function handleExportWorkspace() {
    setIsExportingWorkspace(true);
    setWorkspaceExportStatus({ tone: "loading", message: "Exporting" });
    try {
      const { blob, filename } = await exportWorkspace();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      setWorkspaceExportStatus({ tone: "success", message: "Saved" });
      notify(t("toast.workspaceExported", "Workspace export downloaded"), "success");
    } catch (error) {
      setWorkspaceExportStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "workspaceExport");
    } finally {
      setIsExportingWorkspace(false);
    }
  }

  async function handlePruneLocalData() {
    if (isPruningLocalData) return;
    if (localDataPhrase !== "PRUNE LOCAL DATA") {
      setLocalDataStatus({ tone: "warn", message: "Phrase required" });
      notifyBlocked(t("toast.prunePhraseRequired", "Type PRUNE LOCAL DATA before pruning local research data"));
      return;
    }
    setIsPruningLocalData(true);
    setLocalDataStatus({ tone: "loading", message: "Pruning" });
    try {
      const report = await pruneLocalData({
        operator: liveGuardForm.operator || "local",
        reason: "manual local retention prune from UI",
        phrase: localDataPhrase,
        ...(localData.keep || defaultLocalData.keep),
      });
      setLocalDataPhrase("");
      setLocalData({
        summary: report.after || defaultLocalData.summary,
        keep: report.keep || localData.keep || defaultLocalData.keep,
        protected: report.protected || localData.protected || [],
      });
      const deleted = report.deleted || {};
      const deletedTotal = Object.values(deleted).reduce((sum, value) => sum + Number(value || 0), 0);
      setLocalDataStatus({ tone: "success", message: `Pruned ${deletedTotal}` });
      await refreshBacktestRuns();
      await refreshAutopilotRuns();
      await refreshPaperExecutions();
      await refreshPaperAccount();
      await refreshAuditLog();
      notify(t("toast.localDataPruned", "Local research data pruned"), "success");
    } catch (error) {
      setLocalDataStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "localDataPrune");
      await refreshAuditLog();
      await refreshLocalData();
    } finally {
      setIsPruningLocalData(false);
    }
  }

  async function handleKillSwitchToggle() {
    if (isUpdatingKillSwitch) return;
    const nextAction = killSwitch?.active ? "resume" : "activate";
    const reason = nextAction === "activate" ? "manual stop all from UI" : "manual resume from UI";
    setIsUpdatingKillSwitch(true);
    setIsStopped(nextAction === "activate");
    try {
      const state = await updateKillSwitch({
        action: nextAction,
        operator: liveGuardForm.operator || "local",
        reason,
      });
      setKillSwitch(state);
      setIsStopped(Boolean(state.active));
      await refreshLiveGuard();
      await refreshAutopilot();
      await refreshAutopilotRuns();
      await refreshAuditLog();
      await refreshPreflight();
      notify(nextAction === "activate" ? t("toast.killActivated", "Kill switch activated") : t("toast.killResumed", "Kill switch resumed"), nextAction === "activate" ? "warn" : "success");
    } catch {
      setIsStopped((value) => !value);
      await refreshKillSwitch();
      await refreshPreflight();
      notify(t("toast.actionFailed", "Action failed: {message}", { message: statusText(t, "kill switch unavailable") }), "danger");
    } finally {
      setIsUpdatingKillSwitch(false);
    }
  }

  async function handleAutopilotToggle() {
    const shouldStop = Boolean(autopilot?.running);
    if (!shouldStop && isStopped) {
      notifyBlocked(t("toast.killSwitchBlocksAuto", "Resume the Kill Switch before starting Autopilot"));
      return;
    }
    if (!shouldStop && mode === "Live" && !(liveGuard?.unlocked && liveExecutionForm.credentialId && liveExecutionForm.passphrase)) {
      setIsLiveGuardOpen(true);
      notifyBlocked(t("toast.liveSetupRequired", "Complete Live Setup before starting Live Autopilot"));
      return;
    }
    setIsUpdatingAutopilot(true);
    try {
      if (shouldStop) {
        const state = await updateAutopilot({
          action: "stop",
          operator: liveGuardForm.operator || "local",
          reason: "manual stop from UI",
        });
        absorbAutopilotState(state);
        await refreshAutopilotRuns();
        await refreshAutopilotSteps();
        await refreshPaperAccount();
        await refreshPaperExecutions();
        await refreshAuditLog();
        await refreshPreflight();
        notify(t("toast.autopilotStopped", "Autopilot stopped"), "success");
        return;
      }
      const autopilotMode = mode === "Live" ? "live" : mode === "Paper" ? "paper" : "shadow";
      const liveEnvironment = liveGuard?.unlocked ? liveGuard.environment : liveGuardForm.environment;
      const strategyInterval = Number(strategyProfile.intervalSeconds || 15);
      const state = await updateAutopilot({
        action: "start",
        operator: liveGuardForm.operator || "local",
        mode: autopilotMode,
        exchange: autopilotMode === "live" ? liveExecutionForm.exchange : strategyProfile.exchange || dataSource,
        environment: autopilotMode === "live" ? liveEnvironment : "",
        symbol: autopilotMode === "live" ? liveExecutionForm.symbol : strategyProfile.symbol || labState.meta.selectedMarket || "BTCUSDT",
        intervalSeconds: strategyInterval,
        maxSteps: autopilotMode === "live" ? 0 : Number(strategyProfile.maxSteps || 0),
        credentialId: Number(liveExecutionForm.credentialId || 0),
        passphrase: liveExecutionForm.passphrase,
        side: autopilotMode === "live" ? liveExecutionForm.side : strategyProfile.side,
        sizeUsdt: autopilotMode === "live" ? Number(liveExecutionForm.sizeUsdt) : Number(strategyProfile.orderSizeUsdt),
        validationOnly: liveExecutionForm.validationOnly,
        reason: "manual start from UI",
      });
      setIsRunStopped(false);
      absorbAutopilotState(state);
      await refreshAutopilotRuns();
      await refreshAutopilotSteps();
      await refreshPaperAccount();
      await refreshPaperExecutions();
      await refreshAuditLog();
      await refreshLiveGuard();
      await refreshLiveExecutions();
      await refreshLiveReconciliations();
      await refreshPreflight();
      notify(t("toast.autopilotStarted", "Autopilot started"), "success");
    } catch (error) {
      const message = localizedErrorDetail(t, error);
      setAutopilot((current) => ({
        ...current,
        running: false,
        lastStatus: "failed",
        lastError: message,
        message,
      }));
      await refreshAuditLog();
      await refreshPreflight();
      notifyError(notify, t, error, "autopilot");
    } finally {
      setIsUpdatingAutopilot(false);
    }
  }

  async function handleLiveGuardAction(action) {
    setIsUpdatingLiveGuard(true);
    setLiveGuardStatus({ tone: "loading", message: action === "unlock" ? "Unlocking" : "Locking" });
    try {
      const state = await updateLiveGuard({ action, ...liveGuardForm });
      setLiveGuard(state);
      setLiveGuardStatus({ tone: state.unlocked ? "success" : "warn", message: state.message || "Updated" });
      await refreshAuditLog();
      await refreshPreflight();
      notify(action === "unlock" ? t("toast.guardUnlocked", "Live Guard updated") : t("toast.guardLocked", "Live Guard locked"), state.unlocked ? "success" : "warn");
    } catch (error) {
      setLiveGuardStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "liveGuard");
      await refreshLiveGuard();
      await refreshAuditLog();
      await refreshPreflight();
    } finally {
      setIsUpdatingLiveGuard(false);
    }
  }

  async function handleLiveExecute() {
    setIsExecutingLive(true);
    setLiveExecutionStatus({ tone: "loading", message: "Executing" });
    try {
      const result = await executeLive({
        ...liveExecutionForm,
        credentialId: Number(liveExecutionForm.credentialId),
        sizeUsdt: Number(liveExecutionForm.sizeUsdt),
        operator: liveGuardForm.operator || "local",
      });
      setLiveExecutionResult(result);
      const approved = result.decision?.approved;
      const statusText = result.execution?.status || (approved ? "approved" : "risk rejected");
      setLiveExecutionStatus({
        tone: approved ? "success" : "danger",
        message: statusText,
      });
      setLabState((current) => ({
        ...current,
        events: [...(result.events || []), ...current.events].slice(0, 12),
      }));
      await refreshAuditLog();
      await refreshLiveGuard();
      await refreshLiveExecutions();
      await refreshLiveReconciliations();
      await refreshPreflight();
      notify(t("toast.executionComplete", "AI execution attempt recorded"), approved ? "success" : "warn");
    } catch (error) {
      setLiveExecutionStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      setLiveExecutionResult(null);
      notifyError(notify, t, error, "liveExecute");
      await refreshAuditLog();
      await refreshLiveGuard();
      await refreshLiveExecutions();
      await refreshLiveReconciliations();
      await refreshPreflight();
    } finally {
      setIsExecutingLive(false);
    }
  }

  async function handleLiveReconcile(record) {
    setReconcilingId(record.id);
    setReconciliationStatus({ tone: "loading", message: `Checking #${record.id}` });
    try {
      const result = await reconcileLiveExecution({
        liveExecutionId: record.id,
        passphrase: liveExecutionForm.passphrase,
        operator: liveGuardForm.operator || "local",
      });
      setReconciliationStatus({
        tone: "success",
        message: `${result.report.status} · ${formatMoney(result.report.filledUsdt || 0)} USDT`,
      });
      await refreshLiveReconciliations();
      await refreshAuditLog();
      await refreshLiveExecutions();
      notify(t("toast.reconciled", "Reconciliation finished"), "success");
    } catch (error) {
      setReconciliationStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "liveReconcile");
      await refreshAuditLog();
      await refreshLiveReconciliations();
    } finally {
      setReconcilingId(null);
    }
  }

  async function handleAccountSync() {
    if (!liveExecutionForm.credentialId) {
      setAccountSyncStatus({ tone: "warn", message: "Select key" });
      notifyBlocked(t("toast.selectKey", "Select a saved key first"));
      return;
    }
    if (!liveExecutionForm.passphrase) {
      setAccountSyncStatus({ tone: "warn", message: "Passphrase required" });
      notifyBlocked(t("toast.passphraseRequired", "Passphrase required"));
      return;
    }
    setIsSyncingAccount(true);
    setAccountSyncStatus({ tone: "loading", message: "Syncing" });
    try {
      const result = await syncAccount({
        credentialId: Number(liveExecutionForm.credentialId),
        passphrase: liveExecutionForm.passphrase,
        exchange: liveExecutionForm.exchange,
        environment: liveGuard?.unlocked ? liveGuard.environment : liveGuardForm.environment,
        symbol: liveExecutionForm.symbol,
        operator: liveGuardForm.operator || "local",
      });
      setAccountSnapshot(result.snapshot);
      setAccountSnapshotMeta({
        snapshotId: result.snapshotId,
        credentialId: result.credential?.id,
        persistedAt: result.persistedAt,
      });
      setAccountSyncStatus({
        tone: "success",
        message: `Saved #${result.snapshotId || "-"} · ${result.snapshot.balances?.length || 0}/${result.snapshot.openOrders?.length || 0}`,
      });
      await refreshAuditLog();
      await refreshPreflight();
      notify(t("toast.accountSynced", "Account snapshot synced"), "success");
    } catch (error) {
      setAccountSnapshot(null);
      setAccountSnapshotMeta(null);
      setAccountSyncStatus({ tone: "danger", message: localizedErrorDetail(t, error) });
      notifyError(notify, t, error, "accountSync");
      await refreshAuditLog();
      await refreshPreflight();
    } finally {
      setIsSyncingAccount(false);
    }
  }

  async function refreshAccountSnapshot({ silent = false } = {}) {
    if (!liveExecutionForm.credentialId) {
      setAccountSnapshot(null);
      setAccountSnapshotMeta(null);
      setAccountSyncStatus({ tone: "warn", message: "Not synced" });
      return;
    }
    if (!silent) {
      setAccountSyncStatus({ tone: "loading", message: "Loading" });
    }
    try {
      const payload = await loadAccountSnapshot({
        credentialId: Number(liveExecutionForm.credentialId),
        exchange: liveExecutionForm.exchange,
        environment: liveGuard?.unlocked ? liveGuard.environment : liveGuardForm.environment,
        symbol: liveExecutionForm.symbol,
      });
      if (!payload.snapshot) {
        setAccountSnapshot(null);
        setAccountSnapshotMeta(null);
        setAccountSyncStatus({ tone: "warn", message: "Not synced" });
        return;
      }
      setAccountSnapshot(payload.snapshot);
      setAccountSnapshotMeta({
        snapshotId: payload.snapshotId,
        credentialId: payload.credentialId,
        persistedAt: payload.persistedAt,
      });
      setAccountSyncStatus({
        tone: "success",
        message: `Loaded #${payload.snapshotId || "-"} · ${payload.snapshot.balances?.length || 0}/${payload.snapshot.openOrders?.length || 0}`,
      });
    } catch (error) {
      setAccountSnapshot(null);
      setAccountSnapshotMeta(null);
      setAccountSyncStatus({ tone: "danger", message: localizedErrorMessage(t, error, "accountSnapshot") });
    }
  }

  const activeRun = labState.runs[selectedRun] ?? labState.runs[0];
  const modeTone = mode === "Live" ? "danger" : mode === "Paper" ? "paper" : "shadow";

  return (
    <>
      <GlobalScrollbars />
      <main className={classNames("app-shell", (isStopped || isRunStopped) && "is-stopped")}>
        <TopBar
          t={t}
          meta={labState.meta}
          paperAccount={paperAccount}
          mode={mode}
          modeTone={modeTone}
          setMode={handleModeChange}
          dataSource={dataSource}
          setDataSource={handleDataSourceChange}
          isStopped={isStopped}
          isUpdatingKillSwitch={isUpdatingKillSwitch}
          onToggleKillSwitch={handleKillSwitchToggle}
          sourceStatus={sourceStatus}
          credentialCount={credentials.length}
          onOpenCredentials={openCredentialPanel}
          strategyName={strategyProfile.name}
          modelProfile={strategyProfile.modelProfile}
          onOpenStrategy={() => setIsStrategyPanelOpen(true)}
          onOpenAIConfig={() => setIsAIConfigOpen(true)}
          liveGuard={liveGuard}
          killSwitch={killSwitch}
          onOpenLiveGuard={() => setIsLiveGuardOpen(true)}
        />

        <section className="lab-grid">
        <aside className="left-rail">
          <BrandBlock
            appInfo={appInfo}
            t={t}
            locale={locale}
            theme={theme}
            onLocaleChange={handleLocaleChange}
            onThemeToggle={handleThemeToggle}
          />
          <ExperimentRuns
            t={t}
            runs={labState.runs}
            defaultModelProfile={strategyProfile.modelProfile}
            selectedRun={selectedRun}
            onSelect={handleRunSelect}
            showArchived={showArchivedRuns}
            onToggleArchived={handleToggleArchivedRuns}
            onNewRun={handleCreateNewRun}
            onConfigure={() => setIsStrategyPanelOpen(true)}
          />
          <SimulationControls
            t={t}
            meta={labState.meta}
            dataSource={dataSource}
            setDataSource={handleDataSourceChange}
            marketDataMode={marketDataMode}
            onMarketDataModeChange={handleMarketDataModeChange}
            timeframe={timeframe}
            onTimeframeChange={handleTimeframeChange}
            isPaused={isPaused}
            isStopped={isStopped || isRunStopped}
            stopLocked={isStopped}
            replaySpeed={replaySpeed}
            replayPoint={replayPoint}
            replayOptions={replayOptions}
            onTogglePause={handlePauseToggle}
            onToggleRunStopped={handleRunStopToggle}
            setReplaySpeed={setReplaySpeed}
            onReplayJump={handleReplayJump}
            onSimStep={handleSimStep}
            onRestart={handleRestartRun}
            onOpenStrategy={() => setIsStrategyPanelOpen(true)}
            onActionNotice={handleFeatureNotice}
            isSimulating={isSimulating}
            autopilot={autopilot}
            autopilotRuns={autopilotRuns}
            mode={mode}
            liveReady={Boolean(liveGuard?.unlocked && liveExecutionForm.credentialId && liveExecutionForm.passphrase)}
            onToggleAutopilot={handleAutopilotToggle}
            isUpdatingAutopilot={isUpdatingAutopilot}
          />
          <footer className="latency-strip">
            <span>{t("panels.dataLatency", "Data Latency")}</span>
            <strong className={latencyToneClass(labState.meta.dataLatencyMs)}>
              {labState.meta.dataLatencyMs}<span className="connection-unit">ms</span>
            </strong>
          </footer>
        </aside>

        <section className="workspace">
          <WorkspaceTabs t={t} active={workspaceTab} onChange={handleWorkspaceTabChange} />
          <ChartWorkspace
            t={t}
            meta={labState.meta}
            activeRun={activeRun}
            candles={labState.candles}
            equity={labState.equity}
            benchmark={labState.benchmark}
            tab={workspaceTab}
            mode={mode}
            backtest={backtestResult}
            backtestRuns={backtestRuns}
            backtestStatus={backtestStatus}
            isRunningBacktest={isRunningBacktest}
            onRunBacktest={handleRunBacktest}
            timeframe={timeframe}
            marketDataMode={marketDataMode}
            theme={theme}
            onTimeframeChange={handleTimeframeChange}
            onOpenStrategy={() => setIsStrategyPanelOpen(true)}
            onActionNotice={handleFeatureNotice}
          />
          <BottomPanel
            active={bottomTab}
            setActive={handleBottomTabChange}
            performance={labState.performance}
            positions={labState.positions}
            orders={labState.orders}
            paperAccount={paperAccount}
            paperExecutions={paperExecutions}
            autopilotSteps={autopilotSteps}
            paperResetStatus={paperResetStatus}
            isResettingPaper={isResettingPaper}
            isPaperResetDisabled={isSimulating || Boolean(autopilot?.running)}
            onPaperReset={handlePaperReset}
            events={visibleEvents}
            eventFilter={eventFilter}
            setEventFilter={handleEventFilterChange}
            onNotify={notify}
            meta={labState.meta}
            features={labState.features}
            verdict={labState.verdict}
            riskProfile={riskProfile}
            strategyProfile={strategyProfile}
            allEvents={labState.events}
            mode={mode}
            t={t}
          />
        </section>

        <aside className="right-rail">
          <VerdictPanel t={t} verdict={labState.verdict} features={labState.features} mode={mode} onOpenAIConfig={() => setIsAIConfigOpen(true)} />
        </aside>
      </section>
      <CredentialPanel
        t={t}
        open={isCredentialPanelOpen}
        onClose={closeCredentialPanel}
        credentials={credentials}
        status={credentialStatus}
        form={credentialForm}
        setForm={setCredentialForm}
        testForm={vaultTestForm}
        setTestForm={updateVaultTestForm}
        testStatus={vaultTestStatus}
        testResult={vaultTestResult}
        testError={vaultTestError}
        onTest={handleVaultConnectionTest}
        onSave={handleCredentialSave}
        onRequestDelete={handleCredentialDeleteRequest}
        onCancelDelete={handleCredentialDeleteCancel}
        onConfirmDelete={handleCredentialDelete}
        deleteCandidateId={credentialDeleteCandidateId}
        deletingCredentialId={deletingCredentialId}
        isSaving={isSavingCredential}
        isTesting={isTestingCredential}
      />
      <StrategyPanel
        t={t}
        open={isStrategyPanelOpen}
        onClose={() => setIsStrategyPanelOpen(false)}
        profile={strategyProfile}
        status={strategyProfileStatus}
        setField={setStrategyProfileField}
        onSave={handleSaveStrategyProfile}
        isSaving={isSavingStrategyProfile}
      />
      <AIConfigPanel
        t={t}
        open={isAIConfigOpen}
        onClose={() => setIsAIConfigOpen(false)}
        meta={labState.meta}
        verdict={labState.verdict}
        strategyProfile={strategyProfile}
        riskProfile={riskProfile}
        providers={aiProviders}
        providerStatus={aiProvidersStatus}
        onRefreshProviders={() => refreshAIProviders()}
        onCopyContext={handleCopyAIContext}
        onCopyCommand={handleCopyAICommand}
      />
      <LiveGuardPanel
        t={t}
        open={isLiveGuardOpen}
        onClose={() => setIsLiveGuardOpen(false)}
        state={liveGuard}
        status={liveGuardStatus}
        form={liveGuardForm}
        setForm={setLiveGuardForm}
        auditLog={auditLog}
        credentials={credentials}
        executionForm={liveExecutionForm}
        setExecutionForm={setLiveExecutionForm}
        executionStatus={liveExecutionStatus}
        executionResult={liveExecutionResult}
        liveExecutions={liveExecutions}
        liveReconciliations={liveReconciliations}
        riskProfile={riskProfile}
        riskProfileStatus={riskProfileStatus}
        setRiskProfileField={setRiskProfileField}
        onSaveRiskProfile={handleSaveRiskProfile}
        isSavingRiskProfile={isSavingRiskProfile}
        reconciliationStatus={reconciliationStatus}
        reconcilingId={reconcilingId}
        accountSyncStatus={accountSyncStatus}
        accountSnapshot={accountSnapshot}
        accountSnapshotMeta={accountSnapshotMeta}
        exportStatus={workspaceExportStatus}
        isExportingWorkspace={isExportingWorkspace}
        localData={localData}
        appInfo={appInfo}
        preflight={preflight}
        localDataStatus={localDataStatus}
        localDataPhrase={localDataPhrase}
        setLocalDataPhrase={setLocalDataPhrase}
        isPruningLocalData={isPruningLocalData}
        onPruneLocalData={handlePruneLocalData}
        onUnlock={() => handleLiveGuardAction("unlock")}
        onLock={() => handleLiveGuardAction("lock")}
        onExecute={handleLiveExecute}
        onSyncAccount={handleAccountSync}
        onReconcile={handleLiveReconcile}
        onExportWorkspace={handleExportWorkspace}
        onRefresh={() => {
          refreshLiveGuard();
          refreshAuditLog();
          refreshLiveExecutions();
          refreshLiveReconciliations();
          refreshAccountSnapshot({ silent: true });
          refreshRiskProfile();
          refreshLocalData();
          refreshAppInfo();
          refreshPreflight();
          notify(t("toast.refreshed", "Refreshed"), "success");
        }}
        isUpdating={isUpdatingLiveGuard}
        isExecuting={isExecutingLive}
        isSyncingAccount={isSyncingAccount}
        onNotify={notifyBlocked}
      />
      <PaperResetDialog
        t={t}
        open={isPaperResetConfirmOpen}
        onClose={handleClosePaperResetConfirm}
        phrase={paperResetPhrase}
        setPhrase={setPaperResetPhrase}
        status={paperResetStatus}
        isResetting={isResettingPaper}
        onConfirm={handleConfirmPaperReset}
      />
        <ToastMessage t={t} toast={toast} onClose={() => setToast(null)} />
      </main>
    </>
  );
}

function ToastMessage({ t, toast, onClose }) {
  if (!toast) return null;
  return (
    <div className={classNames("toast-message", toast.tone)} role="status" aria-live="polite">
      <span>{toast.message}</span>
      <button type="button" onClick={onClose} aria-label={t("toast.closeNotification", "Close notification")}>
        <X size={14} />
      </button>
    </div>
  );
}

function LanguageSwitcher({ t, locale, onChange }) {
  const [isOpen, setIsOpen] = useState(false);
  const switcherRef = useRef(null);
  const languageOptions = [
    { value: "zh-CN", label: t("top.languageChinese", "中文") },
    { value: "en-US", label: "English" },
  ];
  const currentLanguage = languageOptions.find((item) => item.value === locale) || languageOptions[0];

  useEffect(() => {
    if (!isOpen) return undefined;

    function handlePointerDown(event) {
      if (!switcherRef.current?.contains(event.target)) {
        setIsOpen(false);
      }
    }

    function handleKeyDown(event) {
      if (event.key === "Escape") {
        setIsOpen(false);
      }
    }

    window.addEventListener("mousedown", handlePointerDown);
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("mousedown", handlePointerDown);
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen]);

  function selectLanguage(value) {
    setIsOpen(false);
    focusMenuTrigger(switcherRef.current);
    if (value !== locale) {
      onChange(resolveLocale(value));
    }
  }

  function openLanguageMenu() {
    setIsOpen(true);
    focusFirstMenuItem(switcherRef.current);
  }

  return (
    <div className="brand-language-switcher" ref={switcherRef}>
      <button
        className={classNames("language-trigger", isOpen && "active")}
        type="button"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={t("top.languageSwitch", "Language switch")}
        onClick={() => setIsOpen((value) => !value)}
        onKeyDown={(event) => handleMenuTriggerKeyDown(event, openLanguageMenu)}
      >
        <Languages size={14} />
        <span>{currentLanguage.label}</span>
        <ChevronDown size={13} />
      </button>
      {isOpen ? (
        <div
          className="language-menu"
          role="listbox"
          aria-label={t("top.languageSwitch", "Language switch")}
          onKeyDown={(event) => handleMenuListKeyDown(event, { container: switcherRef.current, closeMenu: () => setIsOpen(false) })}
        >
          {languageOptions.map((item) => (
            <button
              className={classNames(item.value === locale && "active")}
              type="button"
              role="option"
              aria-selected={item.value === locale}
              key={item.value}
              onClick={() => selectLanguage(item.value)}
            >
              <span>{item.label}</span>
              {item.value === locale ? <Check size={13} /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function CredentialPanel({
  t,
  open,
  onClose,
  credentials,
  status,
  form,
  setForm,
  testForm,
  setTestForm,
  testStatus,
  testResult,
  testError,
  onTest,
  onSave,
  onRequestDelete,
  onCancelDelete,
  onConfirmDelete,
  deleteCandidateId,
  deletingCredentialId,
  isSaving,
  isTesting,
}) {
  useEscapeToClose(open, onClose);
  const dialogRef = useDialogFocus(open);
  if (!open) return null;

  const setField = (field, value) => {
    setForm((current) => ({ ...current, [field]: value }));
  };
  const setPermission = (field, value) => {
    setForm((current) => ({
      ...current,
      permissions: { ...current.permissions, [field]: value },
    }));
  };
  const setTestField = (field, value) => {
    setTestForm((current) => ({ ...current, [field]: value }));
  };
  const selectedTestCredential = credentials.find((credential) => String(credential.id) === String(testForm.credentialId));
  const testEnvironment = selectedTestCredential?.exchange === "OKX" ? "demo" : testForm.environment;

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="credential-modal" role="dialog" aria-modal="true" aria-labelledby="credential-title" ref={dialogRef}>
        <header className="credential-modal-header">
          <div>
            <h2 id="credential-title">{t("vault.title", "Exchange Vault")}</h2>
            <span><LockKeyhole size={13} /> {t("vault.subtitle", "Local encrypted credentials")}</span>
          </div>
          <button className="icon-close" type="button" onClick={onClose} aria-label={t("vault.close", "Close credentials")}>
            <X size={16} />
          </button>
        </header>

        <div className="credential-modal-grid">
          <form className="credential-form" onSubmit={onSave}>
            <div className="form-title">
              <KeyRound size={15} />
              <strong>{t("vault.addCredential", "Add Credential")}</strong>
              <span className={classNames("vault-status", status.tone)}>{statusText(t, status.message)}</span>
            </div>

            <label className="field">
              <span>{t("common.exchange", "Exchange")}</span>
              <Segmented
                label={t("common.exchange", "Exchange")}
                value={form.exchange}
                values={["Binance", "OKX"]}
                onChange={(value) => setField("exchange", value)}
              />
            </label>

            <div className="field-grid">
              <label className="field">
                <span>{t("common.label", "Label")}</span>
                <input
                  aria-label={t("common.label", "Label")}
                  value={form.label}
                  onChange={(event) => setField("label", event.target.value)}
                  placeholder={t("vault.mainKeyPlaceholder", "Main trading key")}
                />
              </label>
              <label className="field">
                <span>{t("vault.vaultPassphrase", "Vault Passphrase")}</span>
                <input
                  aria-label={t("vault.vaultPassphrase", "Vault Passphrase")}
                  type="password"
                  value={form.passphrase}
                  onChange={(event) => setField("passphrase", event.target.value)}
                  placeholder={t("vault.passphrasePlaceholder", "12+ chars")}
                  autoComplete="new-password"
                />
              </label>
            </div>

            <label className="field">
              <span>{t("common.apiKey", "API Key")}</span>
              <input
                aria-label={t("common.apiKey", "API Key")}
                value={form.apiKey}
                onChange={(event) => setField("apiKey", event.target.value)}
                placeholder={t("vault.apiKeyPlaceholder", "Exchange API key")}
                autoComplete="off"
              />
            </label>

            <label className="field">
              <span>{t("common.apiSecret", "API Secret")}</span>
              <input
                aria-label={t("common.apiSecret", "API Secret")}
                type="password"
                value={form.secret}
                onChange={(event) => setField("secret", event.target.value)}
                placeholder={t("vault.secretPlaceholder", "Exchange secret")}
                autoComplete="new-password"
              />
            </label>

            <label className="field">
              <span>{form.exchange === "OKX" ? t("vault.okxPassphrase", "OKX API Passphrase") : t("vault.optionalPassphrase", "API Passphrase (optional)")}</span>
              <input
                aria-label={form.exchange === "OKX" ? t("vault.okxPassphrase", "OKX API Passphrase") : t("vault.optionalPassphrase", "API Passphrase (optional)")}
                type="password"
                value={form.apiPassphrase}
                onChange={(event) => setField("apiPassphrase", event.target.value)}
                placeholder={form.exchange === "OKX" ? t("vault.okxPassphrasePlaceholder", "Required for OKX private API") : t("vault.optionalPassphrasePlaceholder", "Only if exchange requires it")}
                autoComplete="new-password"
              />
            </label>

            <div className="permission-stack">
              <label className="permission-row">
                <input type="checkbox" aria-label={t("common.read", "Read")} checked readOnly />
                <span>{t("common.read", "Read")}</span>
                <strong>{t("common.required", "Required")}</strong>
              </label>
              <label className="permission-row">
                <input
                  type="checkbox"
                  aria-label={t("common.trade", "Trade")}
                  checked={form.permissions.trade}
                  onChange={(event) => setPermission("trade", event.target.checked)}
                />
                <span>{t("common.trade", "Trade")}</span>
                <strong>{t("common.allowed", "Allowed")}</strong>
              </label>
              <label className="permission-row blocked">
                <input type="checkbox" aria-label={t("common.withdraw", "Withdraw")} checked={false} disabled readOnly />
                <span>{t("common.withdraw", "Withdraw")}</span>
                <strong>{t("common.blockedUpper", "Blocked")}</strong>
              </label>
            </div>

            <button className="save-credential" type="submit" disabled={isSaving}>
              <LockKeyhole size={14} />
              {isSaving ? t("vault.encrypting", "ENCRYPTING") : t("vault.saveEncrypted", "SAVE ENCRYPTED")}
            </button>
          </form>

          <section className="credential-list">
            <div className="form-title">
              <ShieldCheck size={15} />
              <strong>{t("vault.savedKeys", "Saved Keys")}</strong>
              <span>{credentials.length}</span>
            </div>
            <div className="credential-rows">
              {credentials.length === 0 ? (
                <div className="empty-vault">
                  <LockKeyhole size={22} />
                  <strong>{t("vault.noCredentials", "No credentials")}</strong>
                  <span>{t("vault.ready", "Vault ready")}</span>
                </div>
              ) : (
                credentials.map((credential) => {
                  const confirmingDelete = String(deleteCandidateId) === String(credential.id);
                  const deleting = String(deletingCredentialId) === String(credential.id);
                  const deleteLabel = t("vault.deleteKey", "Delete {label}", { label: credential.label });
                  return (
                    <article className={classNames("credential-row", confirmingDelete && "confirming-delete")} key={credential.id}>
                      <div>
                        <strong>{credential.exchange}</strong>
                        <span>{credential.label}</span>
                      </div>
                      <code>{credential.apiKeyMask}</code>
                      <div className="permission-pills">
                        <span>READ</span>
                        {credential.permissions.trade ? <span>TRADE</span> : null}
                      </div>
                      <button
                        className="credential-delete-button"
                        type="button"
                        onClick={() => onRequestDelete(credential.id)}
                        aria-label={deleteLabel}
                        title={deleteLabel}
                        disabled={Boolean(deletingCredentialId)}
                      >
                        <Trash2 size={13} />
                      </button>
                      {confirmingDelete ? (
                        <div className="credential-delete-confirm" role="group" aria-label={t("vault.deleteConfirmTitle", "Delete saved key?")}>
                          <div className="credential-delete-copy">
                            <strong>{t("vault.deleteConfirmTitle", "Delete saved key?")}</strong>
                            <span>{t("vault.deleteConfirmBody", "This removes only the local encrypted credential for {exchange} / {label}.", { exchange: credential.exchange, label: credential.label })}</span>
                          </div>
                          <div className="credential-delete-actions">
                            <button className="credential-delete-cancel" type="button" onClick={onCancelDelete} disabled={deleting}>
                              {t("common.cancel", "Cancel")}
                            </button>
                            <button className="credential-delete-confirm-button" type="button" onClick={() => onConfirmDelete(credential.id)} disabled={deleting}>
                              {deleting ? <LoaderCircle size={12} /> : <Trash2 size={12} />}
                              {deleting ? t("vault.deleting", "Deleting") : t("common.delete", "Delete")}
                            </button>
                          </div>
                        </div>
                      ) : null}
                    </article>
                  );
                })
              )}
            </div>

            <div className="vault-test-card">
              <div className="vault-test-head">
                <Activity size={14} />
                <strong>{t("vault.connectionTest", "Connection Test")}</strong>
                <span className={classNames("vault-status", testStatus.tone)}>{statusText(t, testStatus.message)}</span>
              </div>
              {testError ? (
                <div className="vault-test-alert danger" role="alert">
                  <AlertTriangle size={14} />
                  <div>
                    <strong>{t("vault.connectionIssue", "Connection issue")}</strong>
                    <span>{testError}</span>
                  </div>
                </div>
              ) : null}
              <label className="field">
                <span>{t("vault.credential", "Credential")}</span>
                <select
                  aria-label={t("vault.credential", "Credential")}
                  value={testForm.credentialId}
                  onChange={(event) => {
                    const credential = credentials.find((item) => String(item.id) === event.target.value);
                    setTestForm((current) => ({
                      ...current,
                      credentialId: event.target.value,
                      environment: credential?.exchange === "OKX" ? "demo" : current.environment || "testnet",
                    }));
                  }}
                >
                  {credentials.length === 0 ? (
                    <option value="">{t("vault.noSavedKey", "No saved key")}</option>
                  ) : (
                    credentials.map((credential) => (
                      <option value={credential.id} key={credential.id}>
                        {credential.exchange} / {credential.label}
                      </option>
                    ))
                  )}
                </select>
              </label>
              <div className="field-grid">
                <label className="field">
                  <span>{t("common.environment", "Environment")}</span>
                  {selectedTestCredential?.exchange === "OKX" ? (
                    <input aria-label={t("common.environment", "Environment")} value="demo" readOnly />
                  ) : (
                    <Segmented
                      label={t("common.environment", "Environment")}
                      value={testEnvironment}
                      values={["testnet", "demo"]}
                      onChange={(value) => setTestField("environment", value)}
                    />
                  )}
                </label>
                <label className="field">
                  <span>{t("common.symbol", "Symbol")}</span>
                  <input
                    aria-label={t("common.symbol", "Symbol")}
                    value={testForm.symbol}
                    onChange={(event) => setTestField("symbol", event.target.value.toUpperCase())}
                    placeholder="BTCUSDT"
                  />
                </label>
              </div>
              <label className="field">
                <span>{t("vault.vaultPassphrase", "Vault Passphrase")}</span>
                <input
                  aria-label={t("vault.vaultPassphrase", "Vault Passphrase")}
                  type="password"
                  value={testForm.passphrase}
                  onChange={(event) => setTestField("passphrase", event.target.value)}
                  placeholder={t("vault.testPassphrasePlaceholder", "Decrypt locally for read-only sync")}
                  autoComplete="new-password"
                />
              </label>
              <button
                className={classNames("vault-test-button", isTesting && "is-testing")}
                type="button"
                onClick={onTest}
                disabled={isTesting}
                title={!testForm.credentialId ? t("toast.selectKey", "Select a saved key first") : !testForm.passphrase ? t("toast.passphraseRequired", "Passphrase required") : t("vault.testReadOnly", "TEST READ-ONLY CONNECTION")}
              >
                <RotateCcw size={14} />
                {isTesting ? t("vault.testing", "TESTING") : t("vault.testReadOnly", "TEST READ-ONLY CONNECTION")}
              </button>
              <div className="vault-test-summary">
                <span>{t("vault.canTrade", "Can Trade")}</span>
                <strong className={testResult?.snapshot?.canTrade ? "success-text" : testResult ? "warn-text" : ""}>
                  {testResult ? (testResult.snapshot?.canTrade ? t("common.yes", "yes") : t("common.no", "no")) : "-"}
                </strong>
                <span>{t("vault.balances", "Balances")}</span>
                <strong>{testResult?.snapshot?.balances?.length ?? "-"}</strong>
                <span>{t("vault.openOrders", "Open Orders")}</span>
                <strong>{testResult?.snapshot?.openOrders?.length ?? "-"}</strong>
                <span>{t("vault.snapshot", "Snapshot")}</span>
                <strong>{testResult?.snapshotId ? `#${testResult.snapshotId}` : "-"}</strong>
                <span>{t("vault.synced", "Synced")}</span>
                <code>{testResult?.snapshot?.syncedAt ? formatDateTime(testResult.snapshot.syncedAt) : "-"}</code>
              </div>
            </div>
          </section>
        </div>
      </section>
    </div>
  );
}

function StrategyPanel({ t, open, onClose, profile, status, setField, onSave, isSaving }) {
  useEscapeToClose(open, onClose);
  const dialogRef = useDialogFocus(open);
  if (!open) return null;
  const activeProfile = normalizeStrategyProfile(profile);

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="credential-modal strategy-modal" role="dialog" aria-modal="true" aria-labelledby="strategy-title" ref={dialogRef}>
        <header className="credential-modal-header">
          <div>
            <h2 id="strategy-title">{t("strategy.title", "Strategy Profile")}</h2>
            <span><Brain size={13} /> {t("strategy.subtitle", "AI intent defaults for simulation and Autopilot")}</span>
          </div>
          <button className="icon-close" type="button" onClick={onClose} aria-label={t("strategy.close", "Close strategy profile")}>
            <X size={16} />
          </button>
        </header>

        <section className="strategy-profile-body">
          <div className="form-title">
            <SlidersHorizontal size={15} />
            <strong>{t("strategy.defaults", "Execution Defaults")}</strong>
            <span className={classNames("vault-status", status.tone)}>{statusText(t, status.message)}</span>
          </div>

          <div className="field-grid">
            <label className="field">
              <span>{t("common.name", "Name")}</span>
              <input
                value={profile.name}
                onChange={(event) => setField("name", event.target.value)}
                placeholder="AI Momentum Pro"
              />
            </label>
            <label className="field">
              <span>{t("common.exchange", "Exchange")}</span>
              <Segmented
                label={t("common.exchange", "Exchange")}
                value={profile.exchange}
                values={["Binance", "OKX"]}
                onChange={(value) => setField("exchange", value)}
              />
            </label>
          </div>

          <div className="field-grid">
            <label className="field">
              <span>{t("common.symbol", "Symbol")}</span>
              <input
                value={profile.symbol}
                onChange={(event) => setField("symbol", event.target.value.toUpperCase())}
                placeholder="BTCUSDT"
              />
            </label>
            <label className="field">
              <span>{t("common.side", "Side")}</span>
              <Segmented
                label={t("common.side", "Side")}
                value={profile.side}
                values={["buy", "sell"]}
                onChange={(value) => setField("side", value)}
                labelFor={(item) => choiceLabel(t, item)}
              />
            </label>
          </div>

          <div className="form-title compact">
            <Brain size={15} />
            <strong>{t("strategy.routing", "Model Routing")}</strong>
          </div>

          <div className="field-grid">
            <label className="field">
              <span>{t("strategy.modelRoute", "Model Route")}</span>
              <select
                value={activeProfile.modelProfile}
                onChange={(event) => setField("modelProfile", event.target.value)}
              >
                {strategyModelRoutes.map((route) => (
                  <option key={route.value} value={route.value}>{modelRouteLabel(t, route.value)}</option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>{t("strategy.fallbackRoute", "Fallback Route")}</span>
              <select
                value={activeProfile.modelFallback}
                onChange={(event) => setField("modelFallback", event.target.value)}
              >
                {strategyModelRoutes.map((route) => (
                  <option key={route.value} value={route.value}>{modelRouteLabel(t, route.value)}</option>
                ))}
              </select>
            </label>
          </div>

          <div className="strategy-number-grid">
            <label className="field">
              <span>{t("strategy.orderUsdt", "Order USDT")}</span>
              <input
                type="number"
                min="1"
                value={profile.orderSizeUsdt}
                onChange={(event) => setField("orderSizeUsdt", Number(event.target.value))}
              />
            </label>
            <label className="field">
              <span>{t("strategy.autoInterval", "Auto Interval")}</span>
              <input
                type="number"
                min="5"
                max="3600"
                value={profile.intervalSeconds}
                onChange={(event) => setField("intervalSeconds", Number(event.target.value))}
              />
            </label>
            <label className="field">
              <span>{t("strategy.maxSteps", "Max Steps")}</span>
              <input
                type="number"
                min="0"
                value={profile.maxSteps}
                onChange={(event) => setField("maxSteps", Number(event.target.value))}
              />
            </label>
            <label className="field">
              <span>{t("strategy.concurrentWorkers", "Concurrent Workers")}</span>
              <input
                type="number"
                min="1"
                max="8"
                value={activeProfile.concurrency}
                onChange={(event) => setField("concurrency", Number(event.target.value))}
              />
            </label>
          </div>

          <div className="strategy-topology" aria-label={t("strategy.topology", "Strategy execution topology")}>
            <span>{t("strategy.topologyStrategy", "Strategy Instance")}</span>
            <i>&gt;</i>
            <span>{t("strategy.topologyModel", "Model Route")}</span>
            <i>&gt;</i>
            <span>{t("strategy.topologyRisk", "Risk Guard")}</span>
            <i>&gt;</i>
            <span>{t("strategy.topologyExecution", "Order Router")}</span>
          </div>

          <div className="strategy-summary">
            <span>{t("strategy.simulation", "Simulation")}</span>
            <strong>{profile.exchange} / {profile.symbol}</strong>
            <span>{t("strategy.modelRoute", "Model Route")}</span>
            <strong>{modelRouteLabel(t, activeProfile.modelProfile)}</strong>
            <span>{t("common.intent", "Intent")}</span>
            <strong>{choiceLabel(t, profile.side)} {Number(profile.orderSizeUsdt || 0).toFixed(0)} USDT</strong>
            <span>{t("strategy.concurrency", "Concurrency")}</span>
            <strong>{t("strategy.concurrentRuns", "{count} workers", { count: activeProfile.concurrency })}</strong>
            <span>{t("strategy.autopilot", "Autopilot")}</span>
            <strong>{profile.intervalSeconds}s / {profile.maxSteps > 0 ? t("strategy.steps", "{count} steps", { count: profile.maxSteps }) : t("strategy.unlimited", "unlimited")}</strong>
          </div>

          <button className="save-credential" type="button" onClick={onSave} disabled={isSaving}>
            <SlidersHorizontal size={14} />
            {isSaving ? t("strategy.saving", "SAVING") : t("strategy.save", "SAVE STRATEGY")}
          </button>
        </section>
      </section>
    </div>
  );
}

function providerStateTone(state) {
  if (state === "ok") return "success";
  if (state === "noauth") return "warn";
  if (state === "missing") return "danger";
  return "neutral";
}

function providerGuidanceText(t, provider) {
  const stateGuidance = t(`aiConfig.providerGuidance.${provider.id}.${provider.state}`, "");
  if (stateGuidance) return stateGuidance;
  return t(`aiConfig.providerGuidance.${provider.id}.default`, provider.guidance || "");
}

function AIConfigPanel({
  t,
  open,
  onClose,
  meta,
  verdict,
  strategyProfile,
  riskProfile,
  providers,
  providerStatus,
  onRefreshProviders,
  onCopyContext,
  onCopyCommand,
}) {
  useEscapeToClose(open, onClose);
  const dialogRef = useDialogFocus(open);
  if (!open) return null;

  const providerCards = (providers?.providers?.length ? providers.providers : defaultAIProviders.providers)
    .map((provider) => ({
      ...provider,
      title: t(`aiConfig.providerTitles.${provider.id}`, provider.label),
      body: t(`aiConfig.providerBodies.${provider.id}`, provider.detail),
      guidance: providerGuidanceText(t, provider),
      stateLabel: t(`aiConfig.providerStates.${provider.state}`, provider.state || "-"),
      tone: providerStateTone(provider.state),
    }));
  const activeProfile = normalizeStrategyProfile(strategyProfile);
  const activeRouteLabel = modelRouteLabel(t, activeProfile.modelProfile);
  const fallbackRouteLabel = modelRouteLabel(t, activeProfile.modelFallback);

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section
        className="credential-modal ai-config-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="ai-config-title"
        ref={dialogRef}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="credential-modal-header">
          <div>
            <h2 id="ai-config-title">{t("aiConfig.title", "AI Configuration")}</h2>
            <span><Brain size={13} /> {t("aiConfig.subtitle", "Model routing, subscriptions, and safety boundary")}</span>
          </div>
          <div className="modal-header-actions">
            <button className="header-ghost-button" type="button" onClick={onRefreshProviders} disabled={providerStatus?.tone === "loading"}>
              <RefreshCw size={13} />
              {providerStatus?.tone === "loading" ? t("common.loading", "Loading") : t("aiConfig.refreshProviders", "Refresh")}
            </button>
            <button className="icon-close" type="button" onClick={onClose} aria-label={t("aiConfig.close", "Close AI configuration")}>
              <X size={16} />
            </button>
          </div>
        </header>

        <section className="ai-config-body">
          <div className="ai-active-card">
            <div>
              <span>{t("aiConfig.activeRoute", "Active Route")}</span>
              <strong>{activeRouteLabel}</strong>
              <small>{t("aiConfig.activeRouteBody", "This is the default model route for the current strategy profile. Failed or unavailable assisted routes fall back before orders reach the risk guard.")}</small>
            </div>
            <code>{t("aiConfig.active", "ACTIVE")}</code>
          </div>

          <div className="ai-route-map" aria-label={t("aiConfig.routeMap", "Strategy model route map")}>
            <div className="route-step">
              <span>{t("aiConfig.routeStrategy", "Strategy")}</span>
              <strong>{activeProfile.name || meta?.strategy || "-"}</strong>
            </div>
            <i>&gt;</i>
            <div className="route-step">
              <span>{t("aiConfig.routeModel", "Model")}</span>
              <strong>{activeRouteLabel}</strong>
            </div>
            <i>&gt;</i>
            <div className="route-step">
              <span>{t("aiConfig.routeFallback", "Fallback")}</span>
              <strong>{fallbackRouteLabel}</strong>
            </div>
            <i>&gt;</i>
            <div className="route-step">
              <span>{t("aiConfig.routeExecution", "Execution")}</span>
              <strong>{t("aiConfig.guardedExecution", "Guarded order router")}</strong>
            </div>
          </div>

          <div className="ai-config-summary">
            <span>{t("aiConfig.strategy", "Strategy")}</span>
            <strong>{activeProfile.name || meta?.strategy || "-"}</strong>
            <span>{t("common.intent", "Intent")}</span>
            <strong>{String(activeProfile.side || "buy").toUpperCase()} {formatMoney(Number(activeProfile.orderSizeUsdt || 0))} USDT</strong>
            <span>{t("common.risk", "Risk")}</span>
            <strong>{formatMoney(Number(riskProfile?.maxOrderUsdt || 0))} USDT / {Number(riskProfile?.minConfidence || 0).toFixed(2)}</strong>
            <span>{t("strategy.concurrency", "Concurrency")}</span>
            <strong>{t("strategy.concurrentRuns", "{count} workers", { count: activeProfile.concurrency })}</strong>
          </div>

          <div className="ai-provider-grid">
            {providerCards.map((card) => (
              <article className={classNames("ai-provider-card", `state-${card.tone}`)} key={card.id}>
                <div>
                  <strong>{card.title}</strong>
                  <span>{card.stateLabel}</span>
                </div>
                <p>{card.body}</p>
                {card.guidance ? <small>{card.guidance}</small> : null}
                <footer>
                  <span>{card.source || card.kind || "-"}</span>
                  <code>{card.model || "-"}</code>
                </footer>
                {card.command && card.command !== "configure in AI Vault" && card.state !== "ok" ? (
                  <button className="provider-command-button" type="button" onClick={() => onCopyCommand(card.command)}>
                    <Copy size={13} />
                    {t("aiConfig.copyCommand", "Copy command")}
                  </button>
                ) : null}
              </article>
            ))}
          </div>

          <div className="subscription-assist-card">
            <div>
              <strong>{t("aiConfig.subscriptionAssist", "Subscription Assisted Mode")}</strong>
              <span>{t("aiConfig.subscriptionAssistStatus", "Manual review")}</span>
            </div>
            <p>{t("aiConfig.subscriptionAssistBody", "For Codex, ChatGPT, Claude, or Claude Code subscriptions, copy a sanitized market/strategy/risk context and paste it into your subscribed tool. The app will not read browser cookies, local login sessions, or CLI OAuth tokens.")}</p>
            <button className="save-credential" type="button" onClick={onCopyContext}>
              <Download size={14} />
              {t("aiConfig.copyContext", "COPY AI CONTEXT")}
            </button>
          </div>
        </section>
      </section>
    </div>
  );
}

function LiveGuardPanel({
  t,
  open,
  onClose,
  state,
  status,
  form,
  setForm,
  auditLog,
  credentials,
  executionForm,
  setExecutionForm,
  executionStatus,
  executionResult,
  liveExecutions,
  liveReconciliations,
  riskProfile,
  riskProfileStatus,
  setRiskProfileField,
  onSaveRiskProfile,
  isSavingRiskProfile,
  reconciliationStatus,
  reconcilingId,
  accountSyncStatus,
  accountSnapshot,
  accountSnapshotMeta,
  exportStatus,
  isExportingWorkspace,
  localData,
  appInfo,
  preflight,
  localDataStatus,
  localDataPhrase,
  setLocalDataPhrase,
  isPruningLocalData,
  onPruneLocalData,
  onUnlock,
  onLock,
  onExecute,
  onSyncAccount,
  onReconcile,
  onExportWorkspace,
  onRefresh,
  isUpdating,
  isExecuting,
  isSyncingAccount,
  onNotify,
}) {
  useEscapeToClose(open, onClose);
  const dialogRef = useDialogFocus(open);
  if (!open) return null;

  const setField = (field, value) => {
    setForm((current) => ({ ...current, [field]: value }));
  };
  const setExecutionField = (field, value) => {
    setExecutionForm((current) => ({ ...current, [field]: value }));
  };
  const snapshotTime = accountSnapshot?.syncedAt || accountSnapshotMeta?.persistedAt;
  const snapshotAge = snapshotTime ? Date.now() - new Date(snapshotTime).getTime() : Number.POSITIVE_INFINITY;
  const hasRecentSnapshot =
    Boolean(accountSnapshot && accountSnapshotMeta?.snapshotId) &&
    Number.isFinite(snapshotAge) &&
    snapshotAge <= 5 * 60 * 1000;
  const latestReconciliationByExecution = new Map();
  (liveReconciliations || []).forEach((record) => {
    if (!latestReconciliationByExecution.has(record.liveExecutionId)) {
      latestReconciliationByExecution.set(record.liveExecutionId, record);
    }
  });
  const canReconcile = (record) => {
    const status = String(record.executionStatus || "").toLowerCase();
    return (
      Boolean(executionForm.passphrase) &&
      !record.validationOnly &&
      !["", "not_submitted", "failed", "validated", "signed-preflight", "rejected"].includes(status)
    );
  };
  const localSummary = localData?.summary || defaultLocalData.summary;
  const localKeep = localData?.keep || defaultLocalData.keep;
  const researchRecords =
    Number(localSummary.backtestRuns || 0) +
    Number(localSummary.autopilotRuns || 0) +
    Number(localSummary.autopilotSteps || 0) +
    Number(localSummary.paperExecutions || 0) +
    Number(localSummary.accountSnapshots || 0);
  const protectedRecords =
    Number(localSummary.liveExecutions || 0) +
    Number(localSummary.liveReconciliations || 0) +
    Number(localSummary.auditEntries || 0) +
    Number(localSummary.credentials || 0);
  const clientRuntime = [appInfo?.runtime?.goos, appInfo?.runtime?.goarch].filter(Boolean).join("/") || "-";
  const originMode = appInfo?.security?.localOriginOnly ? t("common.localOnly", "local-only") : t("common.open", "open");
  const docsReady = Boolean(appInfo?.docs?.runbook?.exists && appInfo?.docs?.safety?.exists);
  const docsTitle = [
    appInfo?.docs?.runbook?.path ? `Runbook: ${appInfo.docs.runbook.path}` : "",
    appInfo?.docs?.safety?.path ? `Safety: ${appInfo.docs.safety.path}` : "",
  ].filter(Boolean).join("\n");
  const preflightChecks = preflight?.checks || [];
  const findPreflight = (id) => preflightChecks.find((check) => check.id === id);
  const marketChecks = preflightChecks.filter((check) => String(check.id || "").startsWith("market_"));
  const marketBlocked = marketChecks.some((check) => check.status === "block");
  const marketWarn = marketChecks.some((check) => check.status === "warn");
  const marketStatus = marketBlocked ? "block" : marketWarn ? "warn" : marketChecks.length > 0 ? "ready" : "warn";
  const preflightItems = [
    { label: t("guard.preflightItems.audit", "Audit"), check: findPreflight("audit") },
    { label: t("guard.preflightItems.guard", "Guard"), check: findPreflight("live_guard") },
    { label: t("guard.preflightItems.vault", "Vault"), check: findPreflight("vault") },
    { label: t("guard.preflightItems.market", "Market"), check: { status: marketStatus, summary: `${marketChecks.filter((check) => check.status === "ready").length}/${marketChecks.length || 2}` } },
    { label: t("guard.preflightItems.live", "Live"), check: findPreflight("live_autopilot") },
  ];
  const livePreflight = findPreflight("live_autopilot");
  const selectedExecutionCredential = credentials.find((credential) => String(credential.id) === String(executionForm.credentialId));
  const hasTradeCredential = Boolean(selectedExecutionCredential?.permissions?.trade);
  const hasVaultPassphrase = Boolean(executionForm.passphrase);
  const hasBlockingPreflight = Number(preflight?.block || 0) > 0 || livePreflight?.status === "block";
  const livePreflightReady = livePreflight?.status === "ready" && !hasBlockingPreflight;
  const canExecuteLive =
    Boolean(state?.unlocked) &&
    hasTradeCredential &&
    hasVaultPassphrase &&
    hasRecentSnapshot &&
    !hasBlockingPreflight;
  const liveSetupItems = [
    {
      label: t("guard.setup.vault", "Vault"),
      status: hasTradeCredential ? "ready" : selectedExecutionCredential ? "block" : "todo",
      value: hasTradeCredential ? t("status.trade", "trade") : selectedExecutionCredential ? t("status.read_only", "read-only") : t("status.missing", "missing"),
    },
    {
      label: t("guard.setup.passphrase", "Passphrase"),
      status: hasVaultPassphrase ? "ready" : "todo",
      value: hasVaultPassphrase ? t("status.loaded_lower", "loaded") : t("status.required", "required"),
    },
    {
      label: t("guard.setup.guard", "Guard"),
      status: state?.unlocked ? "ready" : "todo",
      value: state?.unlocked ? state.environment : t("status.locked", "locked"),
    },
    {
      label: t("guard.setup.account", "Account"),
      status: hasRecentSnapshot ? "ready" : "todo",
      value: hasRecentSnapshot ? `#${accountSnapshotMeta.snapshotId}` : t("status.sync", "sync"),
    },
    {
      label: t("guard.setup.preflight", "Preflight"),
      status: hasBlockingPreflight ? "block" : livePreflightReady ? "ready" : "warn",
      value: livePreflight?.summary || preflight?.overall || "warn",
    },
    {
      label: t("guard.setup.execute", "Execute"),
      status: canExecuteLive ? "ready" : "todo",
      value: canExecuteLive ? (executionForm.validationOnly ? t("status.validate", "validate") : t("status.demo", "demo")) : t("status.locked", "locked"),
    },
  ];
  const executeDisabledReason = !state?.unlocked
    ? t("status.guard_locked", "Guard locked")
    : !hasTradeCredential
      ? t("guard.noSavedTradeKey", "No saved trade key")
      : !hasVaultPassphrase
        ? t("status.passphrase_required", "Passphrase required")
        : !hasRecentSnapshot
          ? t("status.not_synced", "Not synced")
          : hasBlockingPreflight
            ? t("common.blocked", "blocked")
            : "";
  const accountSyncDisabledReason = !executionForm.credentialId
    ? t("toast.selectKey", "Select a saved key first")
    : !executionForm.passphrase
      ? t("toast.passphraseRequired", "Passphrase required")
      : "";
  const pruneDisabledReason = localDataPhrase === "PRUNE LOCAL DATA"
    ? ""
    : t("toast.prunePhraseRequired", "Type PRUNE LOCAL DATA before pruning local research data");
  const handleExecuteClick = () => {
    if (isExecuting) return;
    if (!canExecuteLive) {
      onNotify(executeDisabledReason || t("toast.liveSetupRequired", "Complete Live Setup before starting Live Autopilot"));
      return;
    }
    onExecute();
  };
  const handleAccountSyncClick = () => {
    if (isSyncingAccount) return;
    if (accountSyncDisabledReason) {
      onNotify(accountSyncDisabledReason);
      return;
    }
    onSyncAccount();
  };
  const handlePruneClick = () => {
    if (isPruningLocalData) return;
    if (pruneDisabledReason) {
      onNotify(pruneDisabledReason);
      return;
    }
    onPruneLocalData();
  };

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="credential-modal live-guard-modal" role="dialog" aria-modal="true" aria-labelledby="live-guard-title" ref={dialogRef}>
        <header className="credential-modal-header">
          <div>
            <h2 id="live-guard-title">{t("guard.title", "Live Guard")}</h2>
            <span><ShieldCheck size={13} /> {t("guard.subtitle", "Testnet unlock and audit trail")}</span>
          </div>
          <button className="icon-close" type="button" onClick={onClose} aria-label={t("guard.close", "Close live guard")}>
            <X size={16} />
          </button>
        </header>

        <div className="live-guard-grid">
          <section className="credential-form">
            <div className="form-title">
              <LockKeyhole size={15} />
              <strong>{t("guard.unlockGate", "Unlock Gate")}</strong>
              <span className={classNames("vault-status", status.tone)}>{statusText(t, status.message)}</span>
            </div>

            <div className={classNames("guard-state-card", state?.unlocked && "unlocked")}>
              <span>{state?.unlocked ? t("guard.unlocked", "UNLOCKED") : t("guard.locked", "LOCKED")}</span>
              <strong>{state?.environment || t("status.no_live_session", "No live session")}</strong>
              <small>{state?.unlocked ? t("guard.expires", "Expires {time}", { time: formatDateTime(state.expiresAt) }) : state?.message || t("status.live_trading_locked", "live trading locked")}</small>
            </div>

            <div className="risk-profile-card">
              <div className="risk-profile-head">
                <ShieldCheck size={13} />
                <strong>{t("guard.riskProfile", "Risk Profile")}</strong>
                <span className={classNames("vault-status", riskProfileStatus.tone)}>{statusText(t, riskProfileStatus.message)}</span>
              </div>
              <div className="risk-profile-grid">
                <label>
                  <span>{t("guard.maxOrder", "Max Order")}</span>
                  <input
                    aria-label={t("guard.maxOrder", "Max Order")}
                    type="number"
                    min="1"
                    value={riskProfile.maxOrderUsdt}
                    onChange={(event) => setRiskProfileField("maxOrderUsdt", Number(event.target.value))}
                  />
                </label>
                <label>
                  <span>{t("guard.totalExposure", "Total Exp.")}</span>
                  <input
                    aria-label={t("guard.totalExposure", "Total Exp.")}
                    type="number"
                    min="1"
                    value={riskProfile.maxTotalExposureUsdt}
                    onChange={(event) => setRiskProfileField("maxTotalExposureUsdt", Number(event.target.value))}
                  />
                </label>
                <label>
                  <span>{t("guard.dailyDd", "Daily DD %")}</span>
                  <input
                    aria-label={t("guard.dailyDd", "Daily DD %")}
                    type="number"
                    min="0.1"
                    step="0.1"
                    value={riskProfile.maxDailyDrawdownPct}
                    onChange={(event) => setRiskProfileField("maxDailyDrawdownPct", Number(event.target.value))}
                  />
                </label>
                <label>
                  <span>{t("guard.minConfidence", "Min Conf.")}</span>
                  <input
                    aria-label={t("guard.minConfidence", "Min Conf.")}
                    type="number"
                    min="0.1"
                    max="1"
                    step="0.01"
                    value={riskProfile.minConfidence}
                    onChange={(event) => setRiskProfileField("minConfidence", Number(event.target.value))}
                  />
                </label>
                <label>
                  <span>{t("guard.spread", "Spread %")}</span>
                  <input
                    aria-label={t("guard.spread", "Spread %")}
                    type="number"
                    min="0.001"
                    step="0.001"
                    value={riskProfile.maxSpreadPct}
                    onChange={(event) => setRiskProfileField("maxSpreadPct", Number(event.target.value))}
                  />
                </label>
                <label>
                  <span>{t("guard.losses", "Losses")}</span>
                  <input
                    aria-label={t("guard.losses", "Losses")}
                    type="number"
                    min="1"
                    value={riskProfile.maxConsecutiveLosses}
                    onChange={(event) => setRiskProfileField("maxConsecutiveLosses", Number(event.target.value))}
                  />
                </label>
              </div>
              <button className="risk-profile-save" type="button" onClick={onSaveRiskProfile} disabled={isSavingRiskProfile}>
                <ShieldCheck size={13} />
                {isSavingRiskProfile ? t("common.saving", "SAVING") : t("guard.saveRisk", "SAVE RISK")}
              </button>
            </div>

            <div className="field-grid">
              <label className="field">
                <span>{t("common.operator", "Operator")}</span>
                <input
                  aria-label={t("common.operator", "Operator")}
                  value={form.operator}
                  onChange={(event) => setField("operator", event.target.value)}
                  placeholder="local"
                />
              </label>
              <label className="field">
                <span>{t("common.environment", "Environment")}</span>
                <Segmented
                  label={t("common.environment", "Environment")}
                  value={form.environment}
                  values={["testnet", "demo"]}
                  onChange={(value) => setField("environment", value)}
                />
              </label>
            </div>

            <div className="field-grid">
              <label className="field">
                <span>{t("guard.ttlSeconds", "TTL Seconds")}</span>
                <input
                  aria-label={t("guard.ttlSeconds", "TTL Seconds")}
                  type="number"
                  min="60"
                  max="900"
                  value={form.ttlSeconds}
                  onChange={(event) => setField("ttlSeconds", Number(event.target.value))}
                />
              </label>
              <label className="field">
                <span>{t("guard.maxOrderUsdt", "Max Order USDT")}</span>
                <input
                  aria-label={t("guard.maxOrderUsdt", "Max Order USDT")}
                  type="number"
                  min="1"
                  value={form.maxOrderUsdt}
                  onChange={(event) => setField("maxOrderUsdt", Number(event.target.value))}
                />
              </label>
            </div>

            <label className="field">
              <span>{t("guard.unlockPhrase", "Unlock Phrase")}</span>
              <input
                aria-label={t("guard.unlockPhrase", "Unlock Phrase")}
                value={form.phrase}
                onChange={(event) => setField("phrase", event.target.value)}
                placeholder={t("guard.unlockPhrasePlaceholder", "ENABLE TESTNET LIVE")}
                autoComplete="off"
              />
            </label>

            <label className="field">
              <span>{t("common.reason", "Reason")}</span>
              <input
                aria-label={t("common.reason", "Reason")}
                value={form.reason}
                onChange={(event) => setField("reason", event.target.value)}
                placeholder={t("guard.reasonPlaceholder", "testnet validation only")}
              />
            </label>

            <div className="guard-actions">
              <button className="save-credential" type="button" onClick={onUnlock} disabled={isUpdating}>
                <ShieldCheck size={14} />
                {isUpdating ? t("panels.updating", "UPDATING") : t("guard.unlockTestnet", "UNLOCK TESTNET")}
              </button>
              <button className="lock-live" type="button" onClick={onLock} disabled={isUpdating}>
                <Square size={13} />
                {t("guard.lock", "LOCK")}
              </button>
            </div>
          </section>

          <section className="credential-form live-execute-panel">
            <div className="form-title">
              <Zap size={15} />
              <strong>{t("guard.aiExecute", "AI Execute")}</strong>
              <span className={classNames("vault-status", executionStatus.tone)}>{statusText(t, executionStatus.message)}</span>
            </div>

            <div className="execute-state">
              <span>{executionForm.validationOnly ? t("common.validation", "VALIDATION") : t("common.demoSubmit", "DEMO SUBMIT")}</span>
              <strong>{executionResult?.execution?.status || (state?.unlocked ? t("guard.ready", "Ready") : t("status.guard_locked", "Guard locked"))}</strong>
              <small>{executionResult?.execution?.message || executeDisabledReason || t("guard.executeRequirement", "Vault passphrase and recent account sync required for every attempt")}</small>
            </div>

            <div className="live-setup-card">
              <div className="live-setup-head">
                <KeyRound size={13} />
                <strong>{t("guard.liveSetup", "Live Setup")}</strong>
                <span className={canExecuteLive ? "success-text" : hasBlockingPreflight ? "danger-text" : "warn-text"}>
                  {canExecuteLive ? t("common.ready", "ready") : hasBlockingPreflight ? t("common.blocked", "blocked") : t("common.pending", "pending")}
                </span>
              </div>
              <div className="live-setup-grid">
                {liveSetupItems.map((item) => (
                  <span className={classNames("live-setup-item", item.status)} key={item.label}>
                    <small>{item.label}</small>
                    <strong>{item.value}</strong>
                  </span>
                ))}
              </div>
            </div>

            <div className={classNames("preflight-card", preflight?.overall)}>
              <div className="preflight-head">
                <strong>{t("guard.preflight", "Preflight")}</strong>
                <span>{preflight?.overall || "warn"}</span>
              </div>
              <div className="preflight-counts">
                <span>{t("guard.readyCount", "Ready")} <strong>{preflight?.ready || 0}</strong></span>
                <span>{t("guard.warnCount", "Warn")} <strong>{preflight?.warn || 0}</strong></span>
                <span>{t("guard.blockCount", "Block")} <strong>{preflight?.block || 0}</strong></span>
              </div>
              <div className="preflight-grid">
                {preflightItems.map((item) => (
                  <span key={item.label}>
                    <small>{item.label}</small>
                    <strong className={classNames(item.check?.status === "ready" && "success-text", item.check?.status === "warn" && "warn-text", item.check?.status === "block" && "danger-text")}>
                      {item.check?.status || "-"}
                    </strong>
                  </span>
                ))}
              </div>
            </div>

            <label className="field">
              <span>{t("guard.credential", "Credential")}</span>
              <select
                aria-label={t("guard.credential", "Credential")}
                value={executionForm.credentialId}
                onChange={(event) => {
                  const credential = credentials.find((item) => String(item.id) === event.target.value);
                  setExecutionForm((current) => ({
                    ...current,
                    credentialId: event.target.value,
                    exchange: credential?.exchange || current.exchange,
                  }));
                }}
              >
                {credentials.length === 0 ? (
                  <option value="">{t("guard.noSavedTradeKey", "No saved trade key")}</option>
                ) : (
                  credentials.map((credential) => (
                    <option value={credential.id} key={credential.id}>
                      {credential.exchange} / {credential.label}
                    </option>
                  ))
                )}
              </select>
            </label>

            <div className="field-grid">
              <label className="field">
                <span>{t("common.exchange", "Exchange")}</span>
                <input aria-label={t("common.exchange", "Exchange")} value={executionForm.exchange} readOnly />
              </label>
              <label className="field">
                <span>{t("common.symbol", "Symbol")}</span>
                <input
                  aria-label={t("common.symbol", "Symbol")}
                  value={executionForm.symbol}
                  onChange={(event) => setExecutionField("symbol", event.target.value.toUpperCase())}
                  placeholder="BTCUSDT"
                />
              </label>
            </div>

            <div className="field-grid">
              <label className="field">
                <span>{t("common.side", "Side")}</span>
                <Segmented
                  label={t("common.side", "Side")}
                  value={executionForm.side}
                  values={["buy", "sell"]}
                  onChange={(value) => setExecutionField("side", value)}
                  labelFor={(item) => choiceLabel(t, item)}
                />
              </label>
              <label className="field">
                <span>{t("guard.sizeUsdt", "Size USDT")}</span>
                <input
                  aria-label={t("guard.sizeUsdt", "Size USDT")}
                  type="number"
                  min="1"
                  value={executionForm.sizeUsdt}
                  onChange={(event) => setExecutionField("sizeUsdt", Number(event.target.value))}
                />
              </label>
            </div>

            <label className="field">
              <span>{t("vault.vaultPassphrase", "Vault Passphrase")}</span>
              <input
                aria-label={t("vault.vaultPassphrase", "Vault Passphrase")}
                type="password"
                value={executionForm.passphrase}
                onChange={(event) => setExecutionField("passphrase", event.target.value)}
                placeholder={t("guard.requiredForExecution", "Required for execution")}
                autoComplete="new-password"
              />
            </label>

            <label className="permission-row execute-toggle">
              <input
                type="checkbox"
                aria-label={t("guard.validationOnly", "Validation Only")}
                checked={executionForm.validationOnly}
                onChange={(event) => setExecutionField("validationOnly", event.target.checked)}
              />
              <span>{t("guard.validationOnly", "Validation Only")}</span>
              <strong>{executionForm.validationOnly ? t("common.safe", "SAFE") : t("common.demo", "DEMO")}</strong>
            </label>

            <button
              className={classNames("save-credential execute-live", !canExecuteLive && "blocked-action")}
              type="button"
              disabled={isExecuting}
              onClick={handleExecuteClick}
              title={!canExecuteLive ? executeDisabledReason : t("guard.runAiExecute", "RUN AI EXECUTE")}
            >
              <Zap size={14} />
              {isExecuting ? t("guard.executing", "EXECUTING") : t("guard.runAiExecute", "RUN AI EXECUTE")}
            </button>

            <div className="execute-result">
              <span>{t("guard.clientOrder", "Client Order")}</span>
              <code>{executionResult?.execution?.clientOrderId || executionResult?.intent?.id || "-"}</code>
              <span>{t("common.risk", "Risk")}</span>
              <strong className={executionResult?.decision?.approved ? "success-text" : executionResult ? "danger-text" : ""}>
                {executionResult ? (executionResult.decision?.approved ? t("status.approved", "approved") : t("status.rejected", "rejected")) : "-"}
              </strong>
            </div>

            <div className="sync-title">
              <strong>{t("guard.accountSync", "Account Sync")}</strong>
              <span className={classNames("vault-status", accountSyncStatus.tone)}>{statusText(t, accountSyncStatus.message)}</span>
            </div>
            <button
              className={classNames("sync-account", accountSyncDisabledReason && "blocked-action")}
              type="button"
              disabled={isSyncingAccount}
              onClick={handleAccountSyncClick}
              title={accountSyncDisabledReason || t("guard.syncBalanceOrders", "SYNC BALANCE / ORDERS")}
            >
              <RotateCcw size={14} />
              {isSyncingAccount ? t("guard.syncing", "SYNCING") : t("guard.syncBalanceOrders", "SYNC BALANCE / ORDERS")}
            </button>
            <div className="account-sync-summary">
              <span>{t("common.environment", "Environment")}</span>
              <strong>{accountSnapshot?.environment || (state?.unlocked ? state.environment : form.environment)}</strong>
              <span>{t("vault.balances", "Balances")}</span>
              <strong>{accountSnapshot?.balances?.length ?? "-"}</strong>
              <span>{t("vault.openOrders", "Open Orders")}</span>
              <strong>{accountSnapshot?.openOrders?.length ?? "-"}</strong>
              <span>{t("vault.snapshot", "Snapshot")}</span>
              <strong>{accountSnapshotMeta?.snapshotId ? `#${accountSnapshotMeta.snapshotId}` : "-"}</strong>
              <span>{t("vault.synced", "Synced")}</span>
              <code>{accountSnapshot?.syncedAt ? formatDateTime(accountSnapshot.syncedAt) : "-"}</code>
              <span>{t("guard.persisted", "Persisted")}</span>
              <code>{accountSnapshotMeta?.persistedAt ? formatDateTime(accountSnapshotMeta.persistedAt) : "-"}</code>
            </div>
          </section>

          <section className="credential-list audit-list">
            <div className="form-title">
              <Activity size={15} />
              <strong>{t("guard.auditTrail", "Audit Trail")}</strong>
              <span className={auditLog?.verification?.valid ? "success-text" : "danger-text"}>
                {auditLog?.verification?.valid ? t("status.hash_ok", "hash ok") : t("status.hash_fail", "hash fail")}
              </span>
            </div>
            <div className="audit-summary">
              <span>{t("guard.checked", "Checked")}</span>
              <strong>{auditLog?.verification?.checked ?? 0}</strong>
              <span className={classNames("audit-export-status", exportStatus.tone)}>{statusText(t, exportStatus.message)}</span>
              <button className="audit-export" type="button" onClick={onExportWorkspace} disabled={isExportingWorkspace}>
                <Download size={10} />
                {isExportingWorkspace ? t("common.exporting", "Exporting") : t("common.export", "Export")}
              </button>
              <button type="button" onClick={onRefresh}>{t("common.refresh", "Refresh")}</button>
            </div>
            <div className="client-info-card">
              <div className="ledger-title">
                <strong>{t("guard.client", "Client")}</strong>
                <span>{appInfo?.version || "0.1.0"}</span>
              </div>
              <div className="client-info-grid">
                <span>{t("guard.bind", "Bind")}</span>
                <code>{appInfo?.address || "127.0.0.1:8787"}</code>
                <span>{t("guard.runtime", "Runtime")}</span>
                <code>{clientRuntime}</code>
                <span>{t("guard.db", "DB")}</span>
                <code title={appInfo?.database?.path || ""}>{fileName(appInfo?.database?.path)}</code>
                <span>{t("guard.size", "Size")}</span>
                <code>{formatBytes(appInfo?.database?.sizeBytes)}</code>
                <span>{t("guard.origin", "Origin")}</span>
                <strong className={appInfo?.security?.localOriginOnly ? "success-text" : "danger-text"}>{originMode}</strong>
                <span>{t("guard.docs", "Docs")}</span>
                <strong className={docsReady ? "success-text" : "warn-text"} title={docsTitle}>{docsReady ? t("common.ready", "ready") : t("status.missing", "missing")}</strong>
              </div>
            </div>
            <div className="local-data-card">
              <div className="ledger-title">
                <strong>{t("guard.localData", "Local Data")}</strong>
                <span className={classNames("vault-status", localDataStatus.tone)}>{statusText(t, localDataStatus.message)}</span>
              </div>
              <div className="local-data-grid">
                <span><small>{t("guard.backtests", "Backtests")}</small><strong>{localSummary.backtestRuns}</strong></span>
                <span><small>{t("strategy.autopilot", "Autopilot")}</small><strong>{localSummary.autopilotRuns}/{localSummary.autopilotSteps}</strong></span>
                <span><small>{t("guard.paper", "Paper")}</small><strong>{localSummary.paperExecutions}</strong></span>
                <span><small>{t("guard.snapshots", "Snapshots")}</small><strong>{localSummary.accountSnapshots}</strong></span>
                <span><small>{t("guard.protected", "Protected")}</small><strong>{protectedRecords}</strong></span>
                <span><small>{t("guard.research", "Research")}</small><strong>{researchRecords}</strong></span>
              </div>
              <div className="local-data-retention">
                <span>{t("guard.keep", "Keep")}</span>
                <code>{localKeep.keepBacktestRuns}/{localKeep.keepAutopilotRuns}/{localKeep.keepPaperExecutions}/{localKeep.keepAccountSnapshots}</code>
                <span>{t("guard.safe", "Safe")}</span>
                <code>keys/audit/live</code>
              </div>
              <div className="local-data-actions">
                <input
                  aria-label={t("guard.prunePlaceholder", "PRUNE LOCAL DATA")}
                  value={localDataPhrase}
                  onChange={(event) => setLocalDataPhrase(event.target.value)}
                  placeholder={t("guard.prunePlaceholder", "PRUNE LOCAL DATA")}
                  autoComplete="off"
                />
                <button
                  className={classNames(pruneDisabledReason && "blocked-action")}
                  type="button"
                  onClick={handlePruneClick}
                  disabled={isPruningLocalData}
                  title={pruneDisabledReason || t("guard.prune", "Prune")}
                >
                  <AlertTriangle size={11} />
                  {isPruningLocalData ? t("guard.pruning", "Pruning") : t("guard.prune", "Prune")}
                </button>
              </div>
            </div>
            <div className="execution-ledger">
              <div className="ledger-title">
                <strong>{t("guard.executionLedger", "Execution Ledger")}</strong>
                <span>{liveExecutions?.length || 0} / {liveReconciliations?.length || 0}</span>
              </div>
              <div className={classNames("ledger-feedback", reconciliationStatus.tone)}>{reconciliationStatus.message}</div>
              {(liveExecutions || []).length === 0 ? (
                <div className="ledger-empty">{t("guard.noLiveRecords", "No live execution records")}</div>
              ) : (
                liveExecutions.slice(0, 3).map((record) => {
                  const reconciliation = latestReconciliationByExecution.get(record.id);
                  const isReconciling = reconcilingId === record.id;
                  return (
                    <article className="ledger-row" key={record.id}>
                      <div className="ledger-row-main">
                        <strong>{record.symbol}</strong>
                        <span>{record.side} / {record.environment}</span>
                      </div>
                      <div className="ledger-row-actions">
                        <code>#{record.id}</code>
                        <button
                          className={classNames("ledger-reconcile", !canReconcile(record) && "blocked-action")}
                          type="button"
                          disabled={Boolean(reconcilingId)}
                          onClick={() => {
                            if (!canReconcile(record)) {
                              onNotify(t("toast.reconcileNeedsPassphrase", "Load the vault passphrase and use a submitted demo order before reconciliation"));
                              return;
                            }
                            onReconcile(record);
                          }}
                          title={t("guard.reconcileOrder", "Reconcile order")}
                        >
                          <RotateCcw size={10} />
                          {isReconciling ? "..." : t("guard.check", "CHECK")}
                        </button>
                      </div>
                      <small className={record.riskStatus === "approved" ? "success-text" : "danger-text"}>{record.riskStatus}</small>
                      <small>{record.executionStatus}</small>
                      <small className={reconciliation ? "success-text" : "warn-text"}>
                        {reconciliation ? reconciliation.status : t("guard.noCheck", "no check")}
                      </small>
                    </article>
                  );
                })
              )}
            </div>
            <div className="audit-rows">
              {(auditLog?.entries || []).length === 0 ? (
                <div className="empty-vault compact-empty">
                  <LockKeyhole size={20} />
                  <strong>{t("guard.noAuditEntries", "No audit entries")}</strong>
                  <span>{t("guard.awaitingGuard", "Awaiting guard events")}</span>
                </div>
              ) : (
                auditLog.entries.slice(0, 3).map((entry) => (
                  <article className="audit-row" key={entry.id}>
                    <div>
                      <strong>{entry.action}</strong>
                      <span>{formatDateTime(entry.createdAt)} / {entry.actor}</span>
                    </div>
                    <code>{entry.hash.slice(0, 12)}</code>
                    <small className={entry.status === "approved" ? "success-text" : "danger-text"}>{entry.status}</small>
                  </article>
                ))
              )}
            </div>
          </section>
        </div>
      </section>
    </div>
  );
}

function BrandBlock({ appInfo, t, locale, theme, onLocaleChange, onThemeToggle }) {
  return (
    <header className="brand-block">
      <div className="brand-icon">
        <img src="/favicon.svg" alt="" aria-hidden="true" />
      </div>
      <div className="brand-copy">
        <div>
          <h1>CCVar Quant Lab</h1>
          <div className="brand-meta-row">
            <span>v{appInfo?.version || "0.1.0"}</span>
            <ThemeToggle t={t} theme={theme} onToggle={onThemeToggle} />
            <LanguageSwitcher t={t} locale={locale} onChange={onLocaleChange} />
          </div>
        </div>
      </div>
    </header>
  );
}

function ThemeToggle({ t, theme, onToggle }) {
  const isLight = theme === "light";
  const label = isLight ? t("top.themeLight", "Light") : t("top.themeDark", "Dark");
  return (
    <button
      className="theme-trigger"
      type="button"
      aria-label={t("top.themeSwitch", "Theme switch")}
      aria-pressed={isLight}
      onClick={onToggle}
      title={t("top.themeSwitch", "Theme switch")}
    >
      {isLight ? <Sun size={14} /> : <Moon size={14} />}
      <span>{label}</span>
    </button>
  );
}

function TopBar({
  t,
  meta,
  paperAccount,
  mode,
  modeTone,
  setMode,
  dataSource,
  setDataSource,
  isStopped,
  isUpdatingKillSwitch,
  onToggleKillSwitch,
  sourceStatus,
  credentialCount,
  onOpenCredentials,
  strategyName,
  modelProfile,
  onOpenStrategy,
  onOpenAIConfig,
  liveGuard,
  killSwitch,
  onOpenLiveGuard,
}) {
  const simCapital = Number(paperAccount?.startingCapitalUsdt ?? meta.simCapital ?? 0);
  const simPnl = normalizeSignedValue(Number(paperAccount?.totalPnlUsdt ?? meta.dailyPnl ?? 0), 2);
  const simPnlPct = normalizeSignedValue(Number(paperAccount?.returnPct ?? meta.dailyPnlPct ?? 0), 2);
  const simDrawdown = simPnlPct < 0 ? simPnlPct : 0;
  return (
    <header className="top-bar">
      <div className="top-section source-section">
        <span className="label">{t("top.dataSource", "Data Source")}</span>
        <Segmented
          label={t("top.dataSource", "Data Source")}
          value={dataSource}
          values={["Binance", "OKX"]}
          onChange={setDataSource}
          icon={<Database size={15} />}
        />
      </div>

      <div className="top-section mode-section">
        <span className="label">{t("top.mode", "Mode")}</span>
        <Segmented
          label={t("top.mode", "Mode")}
          value={mode}
          values={["Shadow", "Paper", "Live"]}
          onChange={setMode}
          tone={modeTone}
          labelFor={(item) => choiceLabel(t, item)}
        />
        <button className="guard-link" type="button" onClick={onOpenLiveGuard} aria-label={t("guard.title", "Live Guard")}>
          <LockKeyhole size={11} />
          {t("top.guard", "Guard")}
          <strong className={liveGuard?.unlocked ? "success-text" : "warn-text"}>
            {liveGuard?.unlocked ? t("top.on", "ON") : t("top.lock", "LOCK")}
          </strong>
        </button>
      </div>

      <div className="top-section strategy-section">
        <span className="label">{t("top.strategy", "Strategy")}</span>
        <button className="select-button" type="button" onClick={onOpenStrategy}>
          <span>{strategyName || meta.strategy}</span>
          <ChevronDown size={14} />
        </button>
      </div>

      <div className="top-section model-section">
        <span className="label">{t("top.defaultRoute", "Default Route")}</span>
        <button className="model-pill model-config-button" type="button" onClick={onOpenAIConfig} title={t("aiConfig.title", "AI Configuration")}>
          <span>{modelRouteLabel(t, modelProfile) || meta.model}</span>
          <i />
        </button>
      </div>

      <MetricTile label={t("top.simCapital", "Sim Capital")} value={formatMoney(simCapital)} unit="USDT" density="wide" />
      <MetricTile label={t("top.dailyPnl", "Sim PnL")} value={formatSignedMoney(simPnl)} unit="USDT" sub={formatSignedPct(simPnlPct)} tone={simPnl >= 0 ? "positive" : "negative"} density="wide" />
      <MetricTile label={t("top.dailyDrawdown", "Sim Drawdown")} value={`${simDrawdown.toFixed(2)}%`} tone={simDrawdown < 0 ? "negative" : ""} />

      <button
        className={classNames("stop-all", isStopped && "active")}
        type="button"
        onClick={onToggleKillSwitch}
        disabled={isUpdatingKillSwitch}
        aria-busy={isUpdatingKillSwitch}
        title={statusText(t, killSwitch?.message)}
        aria-pressed={isStopped}
        aria-label={
          isUpdatingKillSwitch
            ? t("top.updatingKillSwitch", "Updating Kill Switch")
            : isStopped
              ? t("top.resumeKillSwitch", "Resume Kill Switch")
              : t("top.activateKillSwitch", "Activate Kill Switch")
        }
      >
        {isStopped ? <Play size={16} /> : <Square size={16} />}
        <span>{isStopped ? t("top.resume", "RESUME") : t("top.stopAll", "STOP ALL")}</span>
        <small>{t("top.killSwitch", "Kill Switch")}</small>
      </button>

      <div className="connection">
        <span className="label">{t("top.connection", "Connection")}</span>
        <div>
          <i className="dot success" />
          <span className="connection-name">{dataSource}</span>
          <strong className={latencyToneClass(meta.dataLatencyMs)}>{meta.dataLatencyMs}<span className="connection-unit">ms</span></strong>
        </div>
        <div>
          <i className={classNames("dot", sourceStatus === "api" ? "success" : "warn")} />
          <span className="connection-name">{t("top.localApi", "Local API")}</span>
          <strong className={sourceStatus === "api" ? "success-text" : "warn-text"}>{sourceStatusText(t, sourceStatus)}</strong>
        </div>
        <button className="connection-link connection-vault-link" type="button" onClick={onOpenCredentials} aria-label={t("vault.title", "Exchange Vault")}>
          <ShieldCheck size={12} />
          <span className="connection-name">{t("top.vault", "Vault")}</span>
          <strong>{credentialCount}</strong>
        </button>
      </div>
    </header>
  );
}

function Segmented({ value, values, onChange, icon, tone, labelFor, label }) {
  return (
    <div className={classNames("segmented", tone && `tone-${tone}`)} role="group" aria-label={label}>
      {values.map((item) => {
        const displayLabel = labelFor ? labelFor(item) : item;
        return (
          <button
            key={item}
            type="button"
            className={value === item ? "active" : ""}
            aria-label={label ? `${label}: ${displayLabel}` : displayLabel}
            aria-pressed={value === item}
            onClick={() => onChange(item)}
          >
            {icon && item === value ? icon : null}
            {displayLabel}
          </button>
        );
      })}
    </div>
  );
}

function MetricTile({ label, value, unit, sub, tone, density }) {
  return (
    <div className={classNames("metric-tile", tone, density && `metric-${density}`)}>
      <span className="label">{label}</span>
      <strong className="metric-value">
        <span className="metric-number">{value}</span>
        {unit ? <span className="metric-unit">{unit}</span> : null}
      </strong>
      {sub ? <small>{sub}</small> : null}
    </div>
  );
}

function ExperimentRuns({ t, runs, defaultModelProfile, selectedRun, onSelect, showArchived, onToggleArchived, onNewRun, onConfigure }) {
  const archivedCount = 12;

  return (
    <section className={classNames("panel runs-panel", showArchived && "archive-open")}>
      <div className="panel-header">
        <h2>{t("panels.experimentRuns", "Experiment Runs")}</h2>
        <div className="icon-row">
          <button type="button" onClick={onNewRun} title={t("panels.newRun", "New run")} aria-label={t("panels.newRun", "New run")}>
            <Plus size={14} />
          </button>
          <button className="strategy-config-trigger" type="button" onClick={onConfigure} title={t("panels.configureStrategy", "Configure strategy")} aria-label={t("panels.configureStrategy", "Configure strategy")}>
            <SlidersHorizontal size={14} />
          </button>
        </div>
      </div>
      <div className="runs-head">
        <span>{t("panels.strategyRunColumn", "Strategy / Run")}</span>
        <span>{t("panels.modelRoute", "Model")}</span>
        <span>{t("common.status", "Status")}</span>
        <span>{t("panels.return7d", "Ret. 7D")}</span>
        <span>{t("panels.maxDd", "Max DD")}</span>
        <span>{t("panels.last", "Last")}</span>
      </div>
      <div className="run-list">
        {runs.map((run, index) => {
          const route = runModelProfile(run, index, defaultModelProfile || defaultStrategyProfile.modelProfile);
          return (
            <button
              className={classNames("run-row", index === selectedRun && "selected", run.status?.toLowerCase() === "running" && "running")}
              key={`${run.name}-${run.run}`}
              onClick={() => onSelect(index)}
              title={t("panels.runTitle", "{name} {version} - {run} / {route} / {status} / Last {last}", {
                name: run.name,
                version: run.version,
                run: run.run,
                route: modelRouteLabel(t, route),
                status: choiceLabel(t, run.status),
                last: run.lastRun,
              })}
              aria-label={t("panels.runAria", "{name} {version} {run}, {route}, {status}, return {return7d} percent, max drawdown {maxDd} percent, last run {last}", {
                name: run.name,
                version: run.version,
                run: run.run,
                route: modelRouteLabel(t, route),
                status: choiceLabel(t, run.status),
                return7d: run.return7d.toFixed(2),
                maxDd: run.maxDd.toFixed(2),
                last: run.lastRun,
              })}
            >
              <span className="run-name">
                <strong>{run.name}</strong>
                <small>{run.version} - {run.run}</small>
              </span>
              <ModelBadge t={t} value={route} />
              <StatusPill t={t} status={run.status} />
              <ValueCell value={run.return7d} suffix="%" />
              <ValueCell value={run.maxDd} suffix="%" />
              <span>{run.lastRun}</span>
            </button>
          );
        })}
      </div>
      <button
        className={classNames("archive-row", showArchived && "open")}
        type="button"
        onClick={onToggleArchived}
        aria-expanded={showArchived}
        aria-controls="archived-runs-summary"
        aria-label={t("panels.archivedAria", "{title}, {count} saved runs", {
          title: t("panels.archivedTitle", "Archived runs"),
          count: archivedCount,
        })}
        title={t("panels.archivedAria", "{title}, {count} saved runs", {
          title: t("panels.archivedTitle", "Archived runs"),
          count: archivedCount,
        })}
      >
        <span className="archive-row-copy">
          <span>{t("panels.archivedTitle", "Archived runs")}</span>
        </span>
        <span className="archive-row-meta">
          <strong className="archive-count">{archivedCount}</strong>
          <ChevronDown size={13} />
        </span>
      </button>
      {showArchived ? (
        <div className="archive-drawer" id="archived-runs-summary" role="region" aria-label={t("panels.archivedTitle", "Archived runs")}>
          <span>{t("panels.archivedReady", "Archived runs are retained in local history")}</span>
        </div>
      ) : null}
    </section>
  );
}

function ModelBadge({ t, value }) {
  return (
    <span className={classNames("model-badge", `route-${modelRouteMeta(value).value}`)} title={modelRouteLabel(t, value)}>
      {modelRouteBadge(t, value)}
    </span>
  );
}

function StatusPill({ t, status }) {
  return <span className={classNames("status-pill", status.toLowerCase())}>{choiceLabel(t, status)}</span>;
}

function ValueCell({ value, suffix = "" }) {
  return <span className={value >= 0 ? "positive-text" : "negative-text"}>{value > 0 ? "+" : ""}{value.toFixed(2)}{suffix}</span>;
}

function SimulationControls({
  t,
  meta,
  dataSource,
  setDataSource,
  marketDataMode,
  onMarketDataModeChange,
  timeframe,
  onTimeframeChange,
  mode,
  isPaused,
  isStopped,
  stopLocked = false,
  replaySpeed,
  replayPoint,
  replayOptions = [],
  onTogglePause,
  onToggleRunStopped,
  setReplaySpeed,
  onReplayJump,
  onSimStep,
  onRestart,
  onOpenStrategy,
  onActionNotice,
  isSimulating,
  autopilot,
  autopilotRuns,
  liveReady,
  onToggleAutopilot,
  isUpdatingAutopilot,
}) {
  const autoMode = autopilot?.running ? autopilot.mode : mode === "Live" ? "live" : mode === "Paper" ? "paper" : "shadow";
  const autoTone = autopilot?.running ? "success" : autopilot?.lastStatus === "failed" ? "danger" : "warn";
  const autoBlocked = !autopilot?.running && (stopLocked || (mode === "Live" && !liveReady));
  const autoDisabledReason = stopLocked
    ? t("toast.killSwitchBlocksAuto", "Resume the Kill Switch before starting Autopilot")
    : mode === "Live" && !liveReady
      ? t("toast.liveSetupRequired", "Complete Live Setup before starting Live Autopilot")
      : "";
  const latestAutoRun = autopilotRuns?.[0];
  const currentRunId = autopilot?.runId || latestAutoRun?.id;
  const livePlan = normalizeAutopilotResult(autopilot?.lastResult)?.aiPlan;
  const liveIntent = livePlan?.intent;
  const liveTrace = livePlan?.ai;
  const planConfidence = liveIntent?.confidence ?? liveTrace?.confidence;
  const planTTL = Number(liveIntent?.ttlSeconds || 0);
  const isReplayMode = marketDataMode === "replay";
  const guideCopy = mode === "Live"
    ? t("panels.runGuideLive", "Live mode stays behind the guard. Finish vault, account sync, and validation before any exchange action.")
    : mode === "Paper"
      ? t("panels.runGuidePaper", "Paper mode uses a virtual account. It tests balances, positions, orders, and PnL without touching an exchange account.")
      : t("panels.runGuideShadow", "Shadow mode reads market data and records AI decisions, risk checks, and simulated fills. No real order is sent.");
  const cycleExchange = () => setDataSource(dataSource === "Binance" ? "OKX" : "Binance");
  const cycleTimeframe = () => {
    const index = chartTimeframes.indexOf(timeframe);
    onTimeframeChange(chartTimeframes[(index + 1) % chartTimeframes.length]);
  };
  const [isDataOpen, setIsDataOpen] = useState(false);
  const [isJumpOpen, setIsJumpOpen] = useState(false);
  const dataSelectRef = useRef(null);
  const jumpSelectRef = useRef(null);
  const dataLabel = marketDataModeShortLabel(t, marketDataMode);
  const jumpLabel = replayPoint?.label || t("panels.noReplayPoints", "No replay points");
  const jumpDisabledReason = replayOptions.length === 0
    ? t("toast.jumpHint", "Jump-to-time uses replay history; choose a backtest or run AI step first")
    : "";
  const openDataMenu = () => {
    setIsDataOpen(true);
    focusFirstMenuItem(dataSelectRef.current);
  };
  const openJumpMenu = () => {
    if (jumpDisabledReason) {
      onActionNotice(t("panels.jumpTo", "Jump To"), jumpDisabledReason);
      return;
    }
    setIsJumpOpen(true);
    focusFirstMenuItem(jumpSelectRef.current);
  };
  useDismissOnOutside(isDataOpen, dataSelectRef, () => setIsDataOpen(false));
  useDismissOnOutside(isJumpOpen, jumpSelectRef, () => setIsJumpOpen(false));
  useEffect(() => {
    if (!isDataOpen && !isJumpOpen) return undefined;
    function handleKeyDown(event) {
      if (event.key === "Escape") {
        setIsDataOpen(false);
        setIsJumpOpen(false);
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isDataOpen, isJumpOpen]);
  return (
    <section className="panel controls-panel">
      <div className="panel-header">
        <h2>{t("panels.strategyRun", "Strategy Run")}</h2>
      </div>
      <div className="run-guide">
        <div className="run-guide-copy">
          <Activity size={14} />
          <div>
            <strong>{t("panels.runGuideTitle", "Run the strategy safely")}</strong>
            <p>{guideCopy}</p>
          </div>
        </div>
        <div className="run-flow" aria-label={t("panels.runFlow", "Run flow")}>
          <span><i>1</i>{t("panels.runFlowMarket", "Read market")}</span>
          <span><i>2</i>{t("panels.runFlowDecision", "AI decision")}</span>
          <span><i>3</i>{t("panels.runFlowRecord", "Record result")}</span>
        </div>
      </div>
      <div className="control-grid">
        <label>
          <span>{t("panels.marketSource", "Market source")}</span>
          <button className="select-button compact" type="button" onClick={cycleExchange} title={t("panels.switchExchange", "Switch exchange")} aria-label={t("panels.switchExchange", "Switch exchange")}>
            {dataSource}<ChevronDown size={13} />
          </button>
        </label>
        <label>
          <span>{t("common.symbol", "Symbol")}</span>
          <button className="select-button compact" type="button" onClick={onOpenStrategy}>
            {meta.selectedMarket}<ChevronDown size={13} />
          </button>
        </label>
        <label>
          <span>{t("panels.klinePeriod", "K-line period")}</span>
          <button className="select-button compact" type="button" onClick={cycleTimeframe} title={t("panels.switchTimeframe", "Switch timeframe")} aria-label={t("panels.switchTimeframe", "Switch timeframe")}>
            {timeframe}<ChevronDown size={13} />
          </button>
        </label>
        <label className="control-select-label">
          <span>{t("panels.dataMode", "Data mode")}</span>
          <div
            className="data-select"
            ref={dataSelectRef}
            onBlur={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget)) {
                setIsDataOpen(false);
              }
            }}
            onKeyDown={(event) => {
              handleMenuListKeyDown(event, { container: dataSelectRef.current, closeMenu: () => setIsDataOpen(false) });
            }}
          >
            <button
              className="select-button compact"
              type="button"
              aria-haspopup="menu"
              aria-expanded={isDataOpen}
              title={marketDataModeDetail(t, marketDataMode)}
              onClick={() => setIsDataOpen((value) => !value)}
              onKeyDown={(event) => handleMenuTriggerKeyDown(event, openDataMenu)}
            >
              <Database size={13} />
              <strong>{dataLabel}</strong>
              <ChevronDown size={13} />
            </button>
            {isDataOpen ? (
              <div className="data-menu" role="menu">
                {marketDataModes.map((option) => (
                  <button
                    key={option.value}
                    className={option.value === marketDataMode ? "active" : ""}
                    type="button"
                    role="menuitem"
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => {
                      onMarketDataModeChange(option.value);
                      setIsDataOpen(false);
                      focusMenuTrigger(dataSelectRef.current);
                    }}
                  >
                    <span>{marketDataModeLabel(t, option.value)}</span>
                    <small>{marketDataModeDetail(t, option.value)}</small>
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </label>
      </div>
      <div className="control-actions">
        <button className="pause-btn" type="button" onClick={onTogglePause}>
          {isPaused ? <Play size={15} /> : <Pause size={15} />}
          {isPaused ? t("panels.resumeFeed", "Resume feed") : t("panels.pauseFeed", "Pause feed")}
        </button>
        <button
          className={classNames("danger-btn", stopLocked && "blocked-action")}
          type="button"
          onClick={() => {
            if (stopLocked) {
              onActionNotice(t("panels.killActive", "KILL ACTIVE"), t("toast.killSwitchControlsLocked", "Resume the Kill Switch before changing the run state"));
              return;
            }
            onToggleRunStopped();
          }}
          title={stopLocked ? t("toast.killSwitchControlsLocked", "Resume the Kill Switch before changing the run state") : ""}
        >
          <Square size={14} />
          {stopLocked ? t("panels.killActive", "KILL ACTIVE") : isStopped ? t("panels.resumeRun", "RESUME RUN") : t("panels.stopRun", "STOP RUN")}
        </button>
        <button type="button" onClick={onRestart}>
          <RotateCcw size={14} />
          {t("panels.restart", "RESTART")}
        </button>
        <button className={classNames(isStopped && "blocked-action", isSimulating && "is-running")} type="button" onClick={onSimStep} disabled={isSimulating} title={isStopped ? t("toast.resumeBeforeAiStep", "Resume the run before starting an AI step") : ""}>
          {isSimulating ? <LoaderCircle size={14} /> : <Plus size={14} />}
          {isSimulating ? t("panels.running", "RUNNING") : t("panels.aiStep", "AI STEP")}
        </button>
      </div>
      <div className={classNames("autopilot-card", autopilot?.running && "running")}>
        <div className="autopilot-head">
          <Brain size={14} />
          <strong>{t("panels.aiAutopilot", "AI Autopilot")}</strong>
          <span className={classNames("autopilot-status", autoTone)}>
            {autopilot?.running ? statusText(t, "Running") : statusText(t, autopilot?.lastStatus || "Idle")}
          </span>
        </div>
        <div className="autopilot-grid">
          <span>{t("panels.run", "Run")}</span>
          <strong>{currentRunId ? `#${currentRunId}` : "-"}</strong>
          <span>{t("panels.steps", "Steps")}</span>
          <strong>{autopilot?.completedSteps ?? 0}</strong>
          <span>{t("common.mode", "Mode")}</span>
          <strong>{choiceLabel(t, autoMode)}</strong>
          <span>{t("panels.next", "Next")}</span>
          <code>{autopilot?.nextRunAt ? formatClock(autopilot.nextRunAt) : latestAutoRun?.status || "-"}</code>
        </div>
        <button className={classNames("autopilot-toggle", autoBlocked && "blocked-action")} type="button" onClick={onToggleAutopilot} disabled={isUpdatingAutopilot} title={autoDisabledReason}>
          {autopilot?.running ? <Square size={13} /> : <Play size={13} />}
          {isUpdatingAutopilot ? t("panels.updating", "UPDATING") : autopilot?.running ? t("panels.stopAuto", "STOP AUTO") : t("panels.startAuto", "START AUTO")}
        </button>
      </div>
      {liveIntent ? (
        <div className="autopilot-plan">
          <div className="autopilot-plan-head">
            <Brain size={13} />
            <strong>{t("panels.aiLivePlan", "AI Live Plan")}</strong>
            <code>{liveTrace?.policyVersion || "-"}</code>
          </div>
          <div className="autopilot-plan-grid">
            <span>{t("common.side", "Side")}</span>
            <strong className={liveIntent.side === "sell" ? "negative-text" : "positive-text"}>{liveIntent.side ? choiceLabel(t, liveIntent.side) : "-"}</strong>
            <span>{t("common.size", "Size")}</span>
            <strong>{formatMoney(Number(liveIntent.sizeUsdt || 0))}</strong>
            <span>{t("panels.conf", "Conf")}</span>
            <strong>{formatConfidence(planConfidence)}</strong>
            <span>{t("panels.ttl", "TTL")}</span>
            <code>{planTTL > 0 ? `${planTTL}s` : "-"}</code>
          </div>
        </div>
      ) : (
        <>
          {isReplayMode ? (
            <>
              <label className="speed-control">
                <span>{t("panels.replaySpeed", "Replay Speed")}</span>
                <input
                  type="range"
                  min="0.25"
                  max="8"
                  step="0.25"
                  value={replaySpeed}
                  onChange={(event) => setReplaySpeed(Number(event.target.value))}
                />
                <div className="speed-labels">
                  <span>0.25x</span>
                  <span>0.5x</span>
                  <strong>{replaySpeed.toFixed(replaySpeed % 1 === 0 ? 0 : 2)}x</strong>
                  <span>4x</span>
                  <span>8x</span>
                </div>
              </label>
              <div className="jump-control">
                <span>{t("panels.jumpTo", "Jump To")}</span>
                <div
                  className="jump-select"
                  ref={jumpSelectRef}
                  onBlur={(event) => {
                    if (!event.currentTarget.contains(event.relatedTarget)) {
                      setIsJumpOpen(false);
                    }
                  }}
                  onKeyDown={(event) => {
                    handleMenuListKeyDown(event, { container: jumpSelectRef.current, closeMenu: () => setIsJumpOpen(false) });
                  }}
                >
                  <button
                    className={classNames("select-button compact", jumpDisabledReason && "blocked-action")}
                    type="button"
                    aria-haspopup="menu"
                    aria-expanded={isJumpOpen}
                    aria-disabled={Boolean(jumpDisabledReason)}
                    title={jumpDisabledReason || jumpLabel}
                    onClick={() => {
                      if (jumpDisabledReason) {
                        onActionNotice(t("panels.jumpTo", "Jump To"), jumpDisabledReason);
                        return;
                      }
                      setIsJumpOpen((value) => !value);
                    }}
                    onKeyDown={(event) => handleMenuTriggerKeyDown(event, openJumpMenu)}
                  >
                    <Clock3 size={13} />
                    <strong>{jumpLabel}</strong>
                    <ChevronDown size={13} />
                  </button>
                  <small>{replayPoint ? `${t("panels.replayCursor", "Replay cursor")} · ${replayPoint.tag}` : t("panels.replayPaused", "Replay waits for market data")}</small>
                  {isJumpOpen ? (
                    <div className="jump-menu" role="menu">
                      {replayOptions.map((option) => (
                        <button
                          key={option.value}
                          className={option.value === replayPoint?.value ? "active" : ""}
                          type="button"
                          role="menuitem"
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            onReplayJump(option);
                            setIsJumpOpen(false);
                            focusMenuTrigger(jumpSelectRef.current);
                          }}
                        >
                          <span>{option.label}</span>
                          <small>{option.tag}</small>
                        </button>
                      ))}
                    </div>
                  ) : null}
                </div>
              </div>
            </>
          ) : null}
        </>
      )}
    </section>
  );
}

function WorkspaceTabs({ t, active, onChange }) {
  return (
    <nav className="workspace-tabs" aria-label={t("panels.workspaceTabs", "Workspace tabs")}>
      {["Real-time Sim", "Backtest", "Shadow Trade"].map((tab) => {
        const label = choiceLabel(t, tab);
        return (
          <button
            key={tab}
            type="button"
            className={active === tab ? "active" : ""}
            aria-pressed={active === tab}
            aria-label={label}
            title={label}
            onClick={() => onChange(tab)}
          >
            {label}
          </button>
        );
      })}
    </nav>
  );
}

function ChartWorkspace({
  t,
  meta,
  activeRun,
  candles,
  equity,
  benchmark,
  tab,
  mode,
  backtest,
  backtestRuns,
  backtestStatus,
  isRunningBacktest,
  onRunBacktest,
  timeframe,
  marketDataMode,
  theme,
  onTimeframeChange,
  onOpenStrategy,
  onActionNotice,
}) {
  const lastCandle = candles[candles.length - 1];
  const candleChange = (lastCandle?.close ?? 0) - (lastCandle?.open ?? 0);
  const candleChangePct = lastCandle?.open ? (candleChange / lastCandle.open) * 100 : 0;
  const candleChangeTone = candleChange >= 0 ? "positive-text" : "negative-text";
  const [showIndicators, setShowIndicators] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  return (
    <section className={classNames("chart-workspace panel", isExpanded && "expanded")}>
      <div className="chart-header">
        <div>
          <h2>{meta.selectedSymbol}</h2>
          <span>{choiceLabel(t, tab)} - {activeRun.name} - {choiceLabel(t, mode)}</span>
        </div>
        <div className="chart-tools">
          {chartTimeframes.map((item) => (
            <button
              key={item}
              type="button"
              className={item === timeframe ? "active" : ""}
              aria-pressed={item === timeframe}
              aria-label={t("panels.timeframeOption", "Timeframe {value}", { value: item })}
              title={t("panels.timeframeOption", "Timeframe {value}", { value: item })}
              onClick={() => onTimeframeChange(item)}
            >
              {item}
            </button>
          ))}
          <button
            type="button"
            className={showIndicators ? "active" : ""}
            onClick={() => {
              setShowIndicators((value) => !value);
              onActionNotice(t("panels.indicators", "Indicators"), showIndicators ? t("toast.indicatorsHidden", "Indicators hidden") : t("toast.indicatorsShown", "Indicators shown"));
            }}
            aria-pressed={showIndicators}
            aria-label={t("panels.indicators", "Indicators")}
          >
            <BarChart3 size={14} /> {t("panels.indicators", "Indicators")}
          </button>
          {tab === "Backtest" && (
            <button
              className={classNames("run-action", isRunningBacktest && "is-running")}
              type="button"
              onClick={onRunBacktest}
              disabled={isRunningBacktest}
              aria-busy={isRunningBacktest}
            >
              {isRunningBacktest ? <LoaderCircle size={13} /> : <Play size={13} />}
              {isRunningBacktest ? t("panels.runningBacktest", "Running") : t("panels.runBacktest", "Run")}
            </button>
          )}
          <button className="strategy-config-trigger" type="button" onClick={onOpenStrategy} title={t("panels.configureStrategy", "Configure strategy")} aria-label={t("panels.configureStrategy", "Configure strategy")}>
            <Settings size={14} />
          </button>
          <button
            type="button"
            className={isExpanded ? "active" : ""}
            onClick={() => {
              setIsExpanded((value) => !value);
              onActionNotice(t("panels.expandChart", "Chart size"), isExpanded ? t("toast.chartCollapsed", "Chart returned to normal size") : t("toast.chartExpanded", "Chart expanded"));
            }}
            aria-pressed={isExpanded}
            aria-label={isExpanded ? t("panels.collapseChart", "Collapse chart") : t("panels.expandChart", "Chart size")}
            title={isExpanded ? t("panels.collapseChart", "Collapse chart") : t("panels.expandChart", "Chart size")}
          >
            {isExpanded ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
          </button>
        </div>
      </div>
      {showIndicators ? (
        <div className="indicator-strip">
          <span>EMA 6/18</span>
          <strong className="positive-text">{choiceLabel(t, "Momentum")} +0.48</strong>
          <span>{t("guard.spread", "Spread %").replace(" %", "")} 0.03%</span>
          <strong className="warn-text">{t("panels.funding", "Funding")} -0.23</strong>
        </div>
      ) : null}
      {tab === "Backtest" ? (
        <BacktestWorkspace t={t} result={backtest} runs={backtestRuns} status={backtestStatus} isRunning={isRunningBacktest} onRun={onRunBacktest} />
      ) : (
        <>
      <div className="equity-summary">
        <span>{t("panels.equityCurveSim", "Equity Curve (Sim)")} <strong>{formatMoney(equity[equity.length - 1]?.value ?? 0)}</strong></span>
        <span>{t("panels.benchmarkHold", "Benchmark (Buy & Hold)")} <strong>{formatMoney(benchmark[benchmark.length - 1]?.value ?? 0)}</strong></span>
        <span>{t("panels.lastUpdate", "Last Update")}: {meta.lastUpdated}</span>
      </div>
      <div className="chart-stack">
        <EquityChart equity={equity} benchmark={benchmark} theme={theme} height={isExpanded ? 190 : 120} />
        <MarketChart candles={candles} theme={theme} height={isExpanded ? 480 : 248} />
      </div>
      <div className="market-footer">
        <span>{meta.selectedMarket} - {timeframe} - {marketDataModeShortLabel(t, marketDataMode)}</span>
        <span>O {numberFormat.format(lastCandle.open)} H {numberFormat.format(lastCandle.high)} L {numberFormat.format(lastCandle.low)} C {numberFormat.format(lastCandle.close)}</span>
        <strong className={candleChangeTone}>{formatSignedNumber(candleChange)} ({formatSignedPct(candleChangePct)})</strong>
        <span className="chart-source-badge" title={marketDataModeDetail(t, marketDataMode)}>
          <Database size={11} />
          {marketDataModeLabel(t, marketDataMode)}
        </span>
      </div>
        </>
      )}
    </section>
  );
}

function BacktestWorkspace({ t, result, runs = [], status, isRunning, onRun }) {
  const summary = result?.summary;
  const points = result?.equity || [];
  const trades = result?.trades || [];
  const latestPoints = points.slice(-22);
  const sourceText = summary?.marketDataSource ? marketDataSourceLabel(t, summary.marketDataSource) : "";
  const statusDisplay = compactStatusText(t, status?.message || "Ready", 44);
  const statusTitle = compactStatusTitle(t, status?.message || "Ready");
  const statusLine = sourceText ? `${statusDisplay} / ${sourceText}` : statusDisplay;
  const fullStatusLine = sourceText ? `${statusText(t, status?.message || "Ready")} / ${sourceText}` : statusText(t, status?.message || "Ready");
  const tradeMeta = summary?.warning || (summary ? t("panels.maCandles", "{fast}/{slow} MA - {count} candles", { fast: summary.fastWindow, slow: summary.slowWindow, count: summary.candleCount }) : t("panels.publicCandles", "15m public candles"));
  const tradeMetaDisplay = compactStatusText(t, tradeMeta, 46);
  const tradeMetaTitle = compactStatusTitle(t, tradeMeta);
  return (
    <div className="backtest-workspace">
      <div className="backtest-summary">
        <BacktestMetric label={t("panels.endingEquity", "Ending Equity")} value={summary ? formatMoney(summary.endingEquityUsdt) : "-"} tone={summary?.totalPnlUsdt >= 0 ? "positive" : "negative"} />
        <BacktestMetric label={t("panels.return", "Return")} value={summary ? formatSignedPct(summary.returnPct) : "-"} tone={summary?.returnPct >= 0 ? "positive" : "negative"} />
        <BacktestMetric label={t("panels.benchmark", "Benchmark")} value={summary ? formatSignedPct(summary.benchmarkReturnPct) : "-"} tone={summary?.benchmarkReturnPct >= 0 ? "positive" : "negative"} />
        <BacktestMetric label={t("panels.maxDd", "Max DD")} value={summary ? `${summary.maxDrawdownPct.toFixed(2)}%` : "-"} tone="warn" />
        <BacktestMetric label={t("panels.trades", "Trades")} value={summary ? String(summary.tradeCount) : "-"} />
        <BacktestMetric label={t("panels.winRate", "Win Rate")} value={summary ? `${summary.winRatePct.toFixed(1)}%` : "-"} />
        <BacktestMetric label={t("panels.fees", "Fees")} value={summary ? formatMoney(summary.feesUsdt) : "-"} />
        <BacktestMetric label={t("panels.exposure", "Exposure")} value={summary ? `${summary.exposureTimePct.toFixed(1)}%` : "-"} />
      </div>
      <div className="backtest-body">
        <section className="backtest-curve">
          <div className="backtest-section-head">
            <strong>{t("panels.equityCurve", "Equity Curve")}</strong>
            <span className={status?.tone ? `${status.tone}-text` : ""} title={statusTitle || (fullStatusLine !== statusLine ? fullStatusLine : undefined)}>
              {statusLine}
            </span>
          </div>
          {latestPoints.length === 0 ? (
            <div className="backtest-empty">
              <span>{isRunning ? t("panels.runningBacktestState", "Running backtest") : t("panels.noBacktestResult", "No backtest result")}</span>
              <button type="button" onClick={onRun} disabled={isRunning}>
                {isRunning ? <LoaderCircle size={13} /> : <Play size={13} />}
                {isRunning ? t("panels.running", "RUNNING") : t("panels.runBacktestUpper", "RUN BACKTEST")}
              </button>
            </div>
          ) : (
            <div className="backtest-bars">
              {latestPoints.map((point) => {
                const pct = summary?.startingCapitalUsdt
                  ? Math.max(4, Math.min(100, (point.equity / summary.startingCapitalUsdt - 0.96) * 500))
                  : 0;
                return (
                  <div className="backtest-bar-row" key={point.time}>
                    <span>{formatBacktestTime(point.time)}</span>
                    <div><i style={{ width: `${pct}%` }} /></div>
                    <strong>{formatMoney(point.equity)}</strong>
                  </div>
                );
              })}
            </div>
          )}
        </section>
        <section className="backtest-trades">
          <div className="backtest-section-head">
            <strong>{t("panels.trades", "Trades")}</strong>
            <span title={tradeMetaTitle || (tradeMetaDisplay !== tradeMeta ? tradeMeta : undefined)}>{tradeMetaDisplay}</span>
          </div>
          <div className="backtest-table-wrap">
            <table className="data-table backtest-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>{t("common.side", "Side")}</th>
                  <th>{t("table.entry", "Entry")}</th>
                  <th>{t("table.exit", "Exit")}</th>
                  <th>PnL</th>
                  <th>{t("table.bars", "Bars")}</th>
                </tr>
              </thead>
              <tbody>
                {trades.length === 0 ? (
                  <tr>
                    <td colSpan={6}>
                      <strong>{summary ? t("panels.noTradesGenerated", "No trades generated") : t("panels.runBacktestPrompt", "Run backtest")}</strong>
                      <small>{summary ? t("panels.stayedFlat", "Strategy stayed flat over this sample") : t("panels.usesStrategy", "Uses current Strategy Profile")}</small>
                    </td>
                  </tr>
                ) : (
                  trades.map((trade) => (
                    <tr key={trade.id}>
                      <th>
                        <strong>{trade.id}</strong>
                        <small>{formatDateTime(trade.openedAt)}</small>
                      </th>
                      <td className={trade.side === "sell" ? "negative-text" : "positive-text"}>{choiceLabel(t, String(trade.side || "").toUpperCase())}</td>
                      <td>{formatMoney(trade.entryPrice)}</td>
                      <td>{formatMoney(trade.exitPrice)}</td>
                      <td className={trade.pnlUsdt >= 0 ? "positive-text" : "negative-text"}>{formatSignedMoney(trade.pnlUsdt)}</td>
                      <td>{trade.barsHeld}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          <BacktestHistory t={t} runs={runs} />
        </section>
      </div>
    </div>
  );
}

function BacktestHistory({ t, runs }) {
  return (
    <div className="backtest-history">
      <div className="backtest-section-head">
        <strong>{t("panels.history", "History")}</strong>
        <span>{runs.length ? t("panels.savedRuns", "{count} saved runs", { count: runs.length }) : t("panels.noSavedRuns", "No saved runs")}</span>
      </div>
      <div className="backtest-history-table">
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("table.created", "Created")}</th>
              <th>{t("common.symbol", "Symbol")}</th>
              <th>{t("table.ret", "Ret.")}</th>
              <th>{t("table.trd", "Trd.")}</th>
              <th>{t("common.source", "Source")}</th>
            </tr>
          </thead>
          <tbody>
            {runs.length === 0 ? (
              <tr>
                <td colSpan={5}>
                  <strong>{t("panels.noSavedHistory", "No saved history")}</strong>
                  <small>{t("panels.runBacktestFirst", "Run a backtest to create the first record")}</small>
                </td>
              </tr>
            ) : (
              runs.map((run) => (
                <tr key={run.id}>
                  <th>
                    <strong>#{run.id}</strong>
                    <small>{formatDateTime(run.createdAt)}</small>
                  </th>
                  <td>{run.symbol}</td>
                  <td className={run.returnPct >= 0 ? "positive-text" : "negative-text"}>{formatSignedPct(run.returnPct)}</td>
                  <td>{run.tradeCount}</td>
                  <td>{marketDataSourceLabel(t, run.marketDataSource)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function BacktestMetric({ label, value, tone }) {
  return (
    <div className="backtest-metric">
      <span>{label}</span>
      <strong className={tone ? `${tone}-text` : ""}>{value}</strong>
    </div>
  );
}

function formatBacktestTime(value) {
  if (!value) return "-";
  const date = new Date(value * 1000);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", hour12: false });
}

function EquityChart({ equity, benchmark, theme, height = 158 }) {
  const ref = useRef(null);

  useEffect(() => {
    if (!ref.current) return undefined;
    const chart = createChart(ref.current, chartOptions({ height, theme }));
    const palette = chartPalette(theme);
    const equitySeries = chart.addSeries(AreaSeries, {
      topColor: palette.equityTop,
      bottomColor: palette.equityBottom,
      lineColor: palette.teal,
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: true,
    });
    equitySeries.setData(equity);
    const benchmarkSeries = chart.addSeries(LineSeries, {
      color: palette.benchmark,
      lineWidth: 2,
      priceLineVisible: false,
    });
    benchmarkSeries.setData(benchmark);
    chart.timeScale().fitContent();
    const resize = () => chart.applyOptions({ width: ref.current.clientWidth });
    resize();
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("resize", resize);
      chart.remove();
    };
  }, [benchmark, equity, height, theme]);

  return <div className="equity-chart" ref={ref} style={{ height }} />;
}

function MarketChart({ candles, theme, height = 355 }) {
  const ref = useRef(null);

  useEffect(() => {
    if (!ref.current) return undefined;
    const chart = createChart(ref.current, chartOptions({ height, theme }));
    const palette = chartPalette(theme);
    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: palette.green,
      downColor: palette.red,
      borderVisible: false,
      wickUpColor: palette.green,
      wickDownColor: palette.red,
      priceLineColor: palette.teal,
    });
    candleSeries.setData(candles);
    const volumeSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: "volume" },
      priceScaleId: "",
      color: palette.volumeGreen,
    });
    volumeSeries.setData(candles.map((candle) => ({
      time: candle.time,
      value: candle.volume,
      color: candle.close >= candle.open ? palette.volumeGreen : palette.volumeRed,
    })));
    chart.priceScale("").applyOptions({
      scaleMargins: { top: 0.78, bottom: 0 },
    });
    createSeriesMarkers(candleSeries, [
      { time: candles[18].time, position: "belowBar", color: palette.teal, shape: "arrowUp", text: "BUY" },
      { time: candles[34].time, position: "aboveBar", color: palette.red, shape: "arrowDown", text: "SELL" },
      { time: candles[54].time, position: "belowBar", color: palette.teal, shape: "arrowUp", text: "BUY" },
      { time: candles[72].time, position: "aboveBar", color: palette.red, shape: "arrowDown", text: "SELL" },
      { time: candles[90].time, position: "belowBar", color: palette.teal, shape: "arrowUp", text: "BUY" },
    ]);
    chart.timeScale().fitContent();
    const resize = () => chart.applyOptions({ width: ref.current.clientWidth });
    resize();
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("resize", resize);
      chart.remove();
    };
  }, [candles, height, theme]);

  return <div className="market-chart" ref={ref} style={{ height }} />;
}

function chartPalette(theme) {
  if (theme === "light") {
    return {
      text: "rgba(39, 57, 66, 0.72)",
      grid: "rgba(79, 107, 119, 0.14)",
      border: "rgba(79, 107, 119, 0.24)",
      teal: "#089f95",
      green: "#0aa66d",
      red: "#dd3f3b",
      benchmark: "rgba(78, 98, 108, 0.58)",
      equityTop: "rgba(8, 159, 149, 0.2)",
      equityBottom: "rgba(8, 159, 149, 0.02)",
      volumeGreen: "rgba(10, 166, 109, 0.28)",
      volumeRed: "rgba(221, 63, 59, 0.26)",
      crosshair: "rgba(8, 159, 149, 0.5)",
    };
  }
  return {
    text: "rgba(212, 225, 232, 0.72)",
    grid: "rgba(114, 145, 160, 0.11)",
    border: "rgba(105, 137, 151, 0.22)",
    teal: "#23d4c6",
    green: "#30d889",
    red: "#ef5b57",
    benchmark: "rgba(184, 195, 203, 0.62)",
    equityTop: "rgba(30, 202, 185, 0.24)",
    equityBottom: "rgba(30, 202, 185, 0.02)",
    volumeGreen: "rgba(82, 203, 154, 0.35)",
    volumeRed: "rgba(239, 91, 87, 0.34)",
    crosshair: "rgba(35, 212, 198, 0.45)",
  };
}

function chartOptions({ height, theme }) {
  const palette = chartPalette(theme);
  return {
    height,
    layout: {
      background: { color: "transparent" },
      attributionLogo: false,
      textColor: palette.text,
      fontFamily: "IBM Plex Mono, ui-monospace, SFMono-Regular, Menlo, monospace",
      fontSize: 11,
    },
    grid: {
      vertLines: { color: palette.grid },
      horzLines: { color: palette.grid },
    },
    rightPriceScale: {
      borderColor: palette.border,
    },
    timeScale: {
      borderColor: palette.border,
      rightOffset: 2,
      timeVisible: true,
      secondsVisible: false,
    },
    crosshair: {
      vertLine: { color: palette.crosshair },
      horzLine: { color: palette.crosshair },
    },
  };
}

function VerdictPanel({ t, verdict, features, mode, onOpenAIConfig }) {
  return (
    <section className="panel verdict-panel">
      <div className="panel-header">
        <h2>{t("panels.aiModelVerdict", "AI Model Verdict")}</h2>
        <button className="model-dot model-config-link" type="button" onClick={onOpenAIConfig}>
          {t("panels.modelOnline", "Model online")}
        </button>
      </div>
      <div className="signal-block">
        <span>{t("panels.currentSignal", "Current Signal")}</span>
        <strong>{choiceLabel(t, verdict.signal)}</strong>
        <Zap size={28} />
      </div>
      <div className="confidence">
        <div>
          <span>{t("common.confidence", "Confidence")}</span>
          <strong>{verdict.confidence}%</strong>
        </div>
        <div className="progress"><i style={{ width: `${verdict.confidence}%` }} /></div>
      </div>
      <MiniConfidenceChart />
      <section className="feature-list">
        <h3>{t("panels.keyFeatures", "Key Features (Impact)")}</h3>
        {features.map((feature) => (
          <div className="feature-row" key={feature.name}>
            <span>{choiceLabel(t, feature.name)}</span>
            <div className="feature-track"><i style={{ width: `${Math.abs(feature.value) * 82}%` }} /></div>
            <strong className={feature.impact === "negative" ? "negative-text" : "positive-text"}>
              {feature.value > 0 ? "+" : ""}{feature.value.toFixed(2)}
            </strong>
          </div>
        ))}
      </section>
      <div className="verdict-facts">
        <Fact label={t("panels.uncertainty", "Uncertainty")} value={`${choiceLabel(t, verdict.uncertainty)} (${verdict.uncertaintyScore.toFixed(2)})`} tone="warn" />
        <Fact label={t("panels.regime", "Regime")} value={choiceLabel(t, verdict.regime)} />
        <Fact label={t("panels.riskOverride", "Risk Override")} value={choiceLabel(t, verdict.riskOverride)} tone="success" />
        <Fact label={t("common.mode", "Mode")} value={choiceLabel(t, mode)} />
      </div>
      <div className="reasoning">
        <h3>{t("panels.modelReasoning", "Model Reasoning (Summary)")}</h3>
        <p>{reasoningText(t, verdict.reasoning)}</p>
      </div>
      <div className="ttl-row">
        <Clock3 size={15} />
        <span>TTL</span>
        <strong>{verdict.ttl}</strong>
        <span>{t("panels.expires", "Expires")} {verdict.expiresAt}</span>
      </div>
    </section>
  );
}

function MiniConfidenceChart() {
  return (
    <div className="mini-chart" aria-hidden="true">
      {Array.from({ length: 30 }, (_, index) => (
        <i key={index} style={{ height: `${22 + Math.sin(index / 2) * 9 + index * 1.6}px` }} />
      ))}
    </div>
  );
}

function Fact({ label, value, tone }) {
  return (
    <div>
      <span>{label}</span>
      <strong className={tone ? `${tone}-text` : ""}>{value}</strong>
    </div>
  );
}

function BottomPanel({
  t,
  active,
  setActive,
  performance,
  positions,
  orders,
  paperAccount,
  paperExecutions,
  autopilotSteps,
  paperResetStatus,
  isResettingPaper,
  isPaperResetDisabled,
  onPaperReset,
  events,
  eventFilter,
  setEventFilter,
  onNotify,
  meta,
  features = [],
  verdict,
  riskProfile,
  strategyProfile,
  allEvents = [],
  mode,
}) {
  const [isLogSettingsOpen, setIsLogSettingsOpen] = useState(false);
  const [logAutoScroll, setLogAutoScroll] = useState(true);
  const [compactLog, setCompactLog] = useState(true);
  const logToolsRef = useRef(null);

  useDismissOnOutside(isLogSettingsOpen, logToolsRef, () => setIsLogSettingsOpen(false));

  useEffect(() => {
    if (!isLogSettingsOpen) return undefined;
    function handleKeyDown(event) {
      if (event.key === "Escape") {
        setIsLogSettingsOpen(false);
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isLogSettingsOpen]);

  function handleToggleLogAutoScroll() {
    setLogAutoScroll((value) => {
      const next = !value;
      onNotify(next ? t("toast.logAutoScrollEnabled", "Log auto-scroll enabled") : t("toast.logAutoScrollPaused", "Log auto-scroll paused"), next ? "success" : "warn");
      return next;
    });
  }

  function handleToggleCompactLog() {
    setCompactLog((value) => {
      const next = !value;
      onNotify(next ? t("toast.compactLogEnabled", "Compact event rows enabled") : t("toast.compactLogDisabled", "Expanded event rows enabled"), "info");
      return next;
    });
  }

  function openLogSettingsMenu() {
    setIsLogSettingsOpen(true);
    focusFirstMenuItem(logToolsRef.current);
  }

  function exportVisibleEvents() {
    const payload = {
      product: "CCVar Quant Lab",
      exportedAt: new Date().toISOString(),
      filter: eventFilter,
      events,
    };
    const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `ccvar-events-${eventFilter.toLowerCase().replace(/[^a-z0-9]+/g, "-")}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    onNotify(t("toast.eventsExported", "Visible events exported"), "success");
  }

  return (
    <section className="bottom-panel panel">
      <nav className="bottom-tabs" aria-label={t("panels.analysisTabs", "Analysis tabs")}>
        {["Performance", "AI Steps", "Trades", "Positions", "Orders", "Metrics", "Risk"].map((tab) => {
          const label = choiceLabel(t, tab);
          const countText = tab === "AI Steps"
            ? ` (${autopilotSteps.length})`
            : tab === "Trades"
              ? ` (${paperExecutions.length})`
              : tab === "Positions"
                ? ` (${positions.length})`
                : tab === "Orders"
                  ? ` (${orders.length})`
                  : "";
          return (
            <button
              key={tab}
              type="button"
              className={active === tab ? "active" : ""}
              aria-pressed={active === tab}
              aria-label={`${label}${countText}`}
              title={`${label}${countText}`}
              onClick={() => setActive(tab)}
            >
              {label}
              {countText}
            </button>
          );
        })}
      </nav>
      <div className="bottom-grid">
        <div className="bottom-main">
          {active === "Performance" && <PerformanceTable t={t} rows={performance} />}
          {active === "Positions" && <PositionsTable t={t} rows={positions} />}
          {active === "Orders" && <OrdersTable t={t} rows={orders} />}
          {active === "AI Steps" && <AutopilotStepsView t={t} rows={autopilotSteps} />}
          {active === "Metrics" && <MetricsView t={t} features={features} verdict={verdict} performance={performance} />}
          {active === "Risk" && <RiskView t={t} profile={riskProfile} strategyProfile={strategyProfile} verdict={verdict} events={allEvents} mode={mode} meta={meta} paperAccount={paperAccount} />}
          {active === "Trades" && (
            <PaperLedgerView
              t={t}
              account={paperAccount}
              rows={paperExecutions}
              resetStatus={paperResetStatus}
              isResetting={isResettingPaper}
              isResetDisabled={isPaperResetDisabled}
              onReset={onPaperReset}
            />
          )}
          {active !== "Performance" && active !== "Positions" && active !== "Orders" && active !== "AI Steps" && active !== "Trades" && active !== "Metrics" && active !== "Risk" && (
            <EventLog t={t} events={events} />
          )}
        </div>
        <div className="bottom-side">
          <div className="log-header">
            <h2>{t("panels.eventLog", "Event Log")}</h2>
            <div
              className="log-tools"
              ref={logToolsRef}
              onBlur={(event) => {
                if (!event.currentTarget.contains(event.relatedTarget)) {
                  setIsLogSettingsOpen(false);
                }
              }}
              onKeyDown={(event) => {
                handleMenuListKeyDown(event, { container: logToolsRef.current, closeMenu: () => setIsLogSettingsOpen(false) });
              }}
            >
              <div className="icon-row">
                <button
                  type="button"
                  aria-haspopup="menu"
                  aria-expanded={isLogSettingsOpen}
                  onClick={() => setIsLogSettingsOpen((value) => !value)}
                  onKeyDown={(event) => handleMenuTriggerKeyDown(event, openLogSettingsMenu)}
                  title={t("panels.eventLogSettings", "Event log settings")}
                  aria-label={t("panels.eventLogSettings", "Event log settings")}
                >
                  <Settings size={14} />
                </button>
                <button type="button" onClick={exportVisibleEvents} title={t("panels.exportEvents", "Export events")} aria-label={t("panels.exportEvents", "Export events")}>
                  <Download size={14} />
                </button>
              </div>
              {isLogSettingsOpen ? (
                <div className="log-settings-menu" role="menu">
                  <button className={logAutoScroll ? "active" : ""} type="button" role="menuitemcheckbox" aria-checked={logAutoScroll} onMouseDown={(event) => event.preventDefault()} onClick={handleToggleLogAutoScroll}>
                    <span>
                      {logAutoScroll ? <Check size={13} /> : <Square size={12} />}
                      {t("panels.logAutoScroll", "Log Auto-Scroll")}
                    </span>
                    <small>{t("panels.logAutoScrollDetail", "Keep newest events in view")}</small>
                  </button>
                  <button className={compactLog ? "active" : ""} type="button" role="menuitemcheckbox" aria-checked={compactLog} onMouseDown={(event) => event.preventDefault()} onClick={handleToggleCompactLog}>
                    <span>
                      {compactLog ? <Check size={13} /> : <Square size={12} />}
                      {t("panels.compactLog", "Compact rows")}
                    </span>
                    <small>{t("panels.compactLogDetail", "Use narrow event columns")}</small>
                  </button>
                </div>
              ) : null}
            </div>
          </div>
          <div className="filter-row" role="group" aria-label={t("panels.eventFilters", "Event filters")}>
            {["All", "AI Decision", "Sim Fill", "Shadow", "Risk"].map((filter) => {
              const label = choiceLabel(t, filter);
              return (
                <button
                  key={filter}
                  type="button"
                  className={eventFilter === filter ? "active" : ""}
                  aria-pressed={eventFilter === filter}
                  aria-label={label}
                  title={label}
                  onClick={() => setEventFilter(filter)}
                >
                  {label}
                </button>
              );
            })}
          </div>
          <EventLog t={t} events={events} compact={compactLog} autoScroll={logAutoScroll} />
        </div>
      </div>
      <footer className="model-footer">
        <span>{t("panels.slippageModel", "Slippage Model")}: {modelMetaText(t, meta.slippageModel)}</span>
        <span>{t("panels.fees", "Fees")}: {meta.feeModel}</span>
        <span>{t("panels.funding", "Funding")}: {choiceLabel(t, meta.fundingModel)}</span>
        <button className="footer-toggle" type="button" onClick={handleToggleLogAutoScroll} aria-label={t("panels.logAutoScroll", "Log Auto-Scroll")}>
          <span>{t("panels.logAutoScroll", "Log Auto-Scroll")}</span>
          <strong>{logAutoScroll ? t("panels.enabled", "Enabled") : t("panels.paused", "Paused")}</strong>
          <i className={logAutoScroll ? "toggle-on" : "toggle-off"} />
        </button>
      </footer>
    </section>
  );
}

function AnalyticsStat({ label, value, sub, tone }) {
  return (
    <div className={classNames("analytics-stat", tone)}>
      <span>{label}</span>
      <strong>{value}</strong>
      {sub ? <small>{sub}</small> : null}
    </div>
  );
}

function MetricsView({ t, features = [], verdict = {}, performance = [] }) {
  const returnRow = performance.find((row) => row.metric === "Return");
  const confidence = Number(verdict?.confidence || 0);
  return (
    <div className="analytics-view">
      <div className="analytics-strip">
        <AnalyticsStat label={t("panels.currentSignal", "Current Signal")} value={choiceLabel(t, verdict?.signal || "-")} sub={choiceLabel(t, verdict?.regime || "-")} tone={String(verdict?.signal || "").toLowerCase() === "sell" ? "danger" : "success"} />
        <AnalyticsStat label={t("common.confidence", "Confidence")} value={`${Number.isFinite(confidence) ? Math.round(confidence) : 0}%`} sub={`${t("panels.uncertainty", "Uncertainty")} ${verdict?.uncertaintyScore ?? "-"}`} tone={confidence >= 70 ? "success" : "warn"} />
        <AnalyticsStat label={t("panels.return7d", "Ret. 7D")} value={returnRow?.sevenDay || "-"} sub={t("panels.equityCurveSim", "Equity Curve (Sim)")} tone={String(returnRow?.sevenDay || "").startsWith("-") ? "danger" : "success"} />
        <AnalyticsStat label={t("panels.regime", "Regime")} value={choiceLabel(t, verdict?.regime || "-")} sub={verdict?.riskOverride ? `${t("panels.riskOverride", "Risk Override")}: ${choiceLabel(t, verdict.riskOverride)}` : ""} />
      </div>
      <div className="analytics-table-wrap">
        <table className="data-table analytics-table">
          <thead>
            <tr>
              <th>{t("panels.feature", "Feature")}</th>
              <th>{t("panels.score", "Score")}</th>
              <th>{t("panels.impact", "Impact")}</th>
              <th>{t("panels.contribution", "Contribution")}</th>
            </tr>
          </thead>
          <tbody>
            {features.map((feature) => {
              const isNegative = feature.impact === "negative" || Number(feature.value) < 0;
              return (
                <tr key={feature.name}>
                  <th>{choiceLabel(t, feature.name)}</th>
                  <td className={isNegative ? "negative-text" : "positive-text"}>{feature.value > 0 ? "+" : ""}{Number(feature.value || 0).toFixed(2)}</td>
                  <td>{choiceLabel(t, feature.impact || "-")}</td>
                  <td>
                    <div className="inline-track">
                      <i className={isNegative ? "negative" : ""} style={{ width: `${Math.min(100, Math.abs(Number(feature.value || 0)) * 82)}%` }} />
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function RiskView({ t, profile = defaultRiskProfile, strategyProfile = defaultStrategyProfile, verdict = {}, events = [], mode, meta, paperAccount }) {
  const confidence = Number(verdict?.confidence || 0) / 100;
  const paperReturn = Number(paperAccount?.returnPct ?? Number.NaN);
  const dailyDrawdown = Number.isFinite(paperReturn) ? Math.abs(Math.min(0, paperReturn)) : Math.abs(Number(meta?.dailyDrawdown || 0));
  const orderSizeUsdt = Number(strategyProfile?.orderSizeUsdt || defaultStrategyProfile.orderSizeUsdt || 0);
  const riskEvents = events.filter((event) => String(event.type || "").includes("Risk") || event.level === "danger");
  const checks = [
    {
      label: t("guard.maxOrder", "Max Order"),
      limit: `${formatMoney(Number(profile.maxOrderUsdt || 0))} USDT`,
      current: `${formatMoney(orderSizeUsdt)} USDT`,
      ok: orderSizeUsdt <= Number(profile.maxOrderUsdt || 0),
    },
    {
      label: t("guard.totalExposure", "Total Exp."),
      limit: `${formatMoney(Number(profile.maxTotalExposureUsdt || 0))} USDT`,
      current: `${formatMoney(Number(profile.maxSymbolExposureUsdt || 0))} USDT / ${t("common.symbol", "Symbol")}`,
      ok: Number(profile.maxSymbolExposureUsdt || 0) <= Number(profile.maxTotalExposureUsdt || 0),
    },
    {
      label: t("panels.dailyDrawdown", "Daily Drawdown"),
      limit: `${Number(profile.maxDailyDrawdownPct || 0).toFixed(2)}%`,
      current: `${dailyDrawdown.toFixed(2)}%`,
      ok: dailyDrawdown <= Number(profile.maxDailyDrawdownPct || 0),
    },
    {
      label: t("panels.confidenceGate", "Confidence Gate"),
      limit: `${Math.round(Number(profile.minConfidence || 0) * 100)}%`,
      current: `${Math.round(confidence * 100)}%`,
      ok: confidence >= Number(profile.minConfidence || 0),
    },
    {
      label: t("guard.spread", "Spread %"),
      limit: `${Number(profile.maxSpreadPct || 0).toFixed(3)}%`,
      current: t("panels.marketReady", "Market ready"),
      ok: true,
    },
  ];
  const allClear = checks.every((check) => check.ok) && String(verdict?.riskOverride || "").toLowerCase() !== "block";
  return (
    <div className="analytics-view">
      <div className="analytics-strip">
        <AnalyticsStat label={t("panels.guardStatus", "Guard Status")} value={allClear ? t("common.readyUpper", "READY") : t("common.blockedUpper", "Blocked")} sub={mode ? choiceLabel(t, mode) : "-"} tone={allClear ? "success" : "danger"} />
        <AnalyticsStat label={t("panels.riskOverride", "Risk Override")} value={choiceLabel(t, verdict?.riskOverride || "-")} sub={choiceLabel(t, verdict?.regime || "-")} tone={String(verdict?.riskOverride || "").toLowerCase() === "none" ? "success" : "warn"} />
        <AnalyticsStat label={t("panels.riskEvents", "Risk Events")} value={String(riskEvents.length)} sub={riskEvents[0]?.note ? choiceLabel(t, riskEvents[0].note) : t("panels.noRiskEvents", "No blocking event")} tone={riskEvents.length ? "warn" : "success"} />
        <AnalyticsStat label={t("panels.liveUnlock", "Live Unlock")} value={profile.requireLiveUnlock ? t("panels.required", "Required") : t("panels.optional", "Optional")} sub={t("top.guard", "Guard")} />
      </div>
      <div className="analytics-table-wrap">
        <table className="data-table analytics-table risk-check-table">
          <thead>
            <tr>
              <th>{t("panels.guardrail", "Guardrail")}</th>
              <th>{t("panels.limit", "Limit")}</th>
              <th>{t("panels.current", "Current")}</th>
              <th>{t("common.status", "Status")}</th>
            </tr>
          </thead>
          <tbody>
            {checks.map((check) => (
              <tr key={check.label}>
                <th>{check.label}</th>
                <td>{check.limit}</td>
                <td>{check.current}</td>
                <td>
                  <span className={check.ok ? "risk-low" : "order-status danger"}>{check.ok ? t("common.readyUpper", "READY") : t("common.blockedUpper", "Blocked")}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function PerformanceTable({ t, rows }) {
  return (
    <WideTableFrame
      t={t}
      className="performance-table-frame"
      controlsId="performance-horizontal-table"
      label={t("table.performanceHorizontalScroll", "Performance table horizontal scroll")}
    >
      <table className="data-table performance-table" id="performance-horizontal-table">
        <thead>
          <tr>
            <th>{t("table.metric", "Metric")}</th>
            <th>{t("table.sevenDay", "7D (Sim)")}</th>
            <th>{t("table.thirtyDay", "30D (Sim)")}</th>
            <th>{t("table.allTime", "All Time (Sim)")}</th>
            <th>{t("table.benchmark7d", "Benchmark 7D")}</th>
            <th>{t("table.benchmark30d", "Benchmark 30D")}</th>
            <th>{t("table.benchmarkAll", "Benchmark All")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.metric}>
              <th>{choiceLabel(t, row.metric)}</th>
              <td className={row.trend === "negative" ? "negative-text" : row.trend === "positive" ? "positive-text" : ""}>{row.sevenDay}</td>
              <td>{row.thirtyDay}</td>
              <td>{row.allTime}</td>
              <td>{row.benchmark7d}</td>
              <td>{row.benchmark30d}</td>
              <td>{row.benchmarkAll}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </WideTableFrame>
  );
}

function PositionsTable({ t, rows }) {
  return (
    <WideTableFrame
      t={t}
      className="positions-table-frame"
      controlsId="positions-horizontal-table"
      label={t("table.positionsHorizontalScroll", "Positions table horizontal scroll")}
    >
      <table className="data-table positions-table" id="positions-horizontal-table">
        <thead>
          <tr>
            <th>{t("common.symbol", "Symbol")}</th>
            <th>{t("common.side", "Side")}</th>
            <th>{t("common.size", "Size")}</th>
            <th>{t("table.entry", "Entry")}</th>
            <th>{t("table.mark", "Mark")}</th>
            <th>{t("table.pnlUsdt", "PnL (USDT)")}</th>
            <th>{t("table.pnlPct", "PnL (%)")}</th>
            <th>{t("common.risk", "Risk")}</th>
            <th>{t("table.age", "Age")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.symbol}>
              <th>{row.symbol}</th>
              <td className="positive-text">{choiceLabel(t, row.side)}</td>
              <td>{row.size}</td>
              <td>{numberFormat.format(row.entry)}</td>
              <td>{numberFormat.format(row.mark)}</td>
              <td className="positive-text">+{formatMoney(row.pnl)}</td>
              <td className="positive-text">+{row.pnlPct.toFixed(2)}%</td>
              <td><span className="risk-low">{choiceLabel(t, row.risk)}</span></td>
              <td>{row.age}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </WideTableFrame>
  );
}

function WideTableFrame({ t, children, className = "", controlsId, label }) {
  const scrollRef = useRef(null);
  const railRef = useRef(null);
  const [scrollState, setScrollState] = useState({ canScroll: false, left: 0, max: 0, thumbWidth: 100, thumbLeft: 0 });

  useEffect(() => {
    const element = scrollRef.current;
    if (!element) return undefined;
    let animationFrame = 0;
    const update = () => {
      window.cancelAnimationFrame(animationFrame);
      animationFrame = window.requestAnimationFrame(() => {
        const max = Math.max(0, element.scrollWidth - element.clientWidth);
        const ratio = max > 0 ? element.clientWidth / element.scrollWidth : 1;
        const thumbWidth = Math.max(14, Math.min(100, ratio * 100));
        setScrollState({
          canScroll: max > 1,
          left: element.scrollLeft,
          max,
          thumbWidth,
          thumbLeft: max > 0 ? (element.scrollLeft / max) * (100 - thumbWidth) : 0,
        });
      });
    };
    update();
    element.addEventListener("scroll", update, { passive: true });
    const resizeObserver = typeof ResizeObserver !== "undefined" ? new ResizeObserver(update) : null;
    resizeObserver?.observe(element);
    if (element.firstElementChild) resizeObserver?.observe(element.firstElementChild);
    window.addEventListener("resize", update);
    return () => {
      window.cancelAnimationFrame(animationFrame);
      element.removeEventListener("scroll", update);
      resizeObserver?.disconnect();
      window.removeEventListener("resize", update);
    };
  }, []);

  const handleWheel = (event) => {
    const element = scrollRef.current;
    if (!element || scrollState.max <= 0) return;
    const hasVerticalScroll = element.scrollHeight > element.clientHeight + 4;
    const hasHorizontalGesture = Math.abs(event.deltaX) > Math.abs(event.deltaY);
    const shouldTranslateVerticalWheel = event.shiftKey || (!hasVerticalScroll && Math.abs(event.deltaY) > Math.abs(event.deltaX));
    if (!hasHorizontalGesture && !shouldTranslateVerticalWheel) return;
    event.preventDefault();
    const delta = hasHorizontalGesture ? event.deltaX : event.deltaY + event.deltaX;
    element.scrollLeft = Math.max(0, Math.min(scrollState.max, element.scrollLeft + delta));
  };

  const scrollToClientX = (clientX) => {
    const element = scrollRef.current;
    const rail = railRef.current;
    if (!element || !rail || scrollState.max <= 0) return;
    const rect = rail.getBoundingClientRect();
    const thumbWidthPx = rect.width * (scrollState.thumbWidth / 100);
    const usableWidth = Math.max(1, rect.width - thumbWidthPx);
    const position = clientX - rect.left - thumbWidthPx / 2;
    const nextLeft = (position / usableWidth) * scrollState.max;
    element.scrollLeft = Math.max(0, Math.min(scrollState.max, nextLeft));
  };

  const handleRailPointerDown = (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    scrollToClientX(event.clientX);
  };

  const handleRailPointerMove = (event) => {
    if (!event.currentTarget.hasPointerCapture?.(event.pointerId)) return;
    event.preventDefault();
    scrollToClientX(event.clientX);
  };

  const handleRailKeyDown = (event) => {
    const element = scrollRef.current;
    if (!element) return;
    const step = Math.max(28, element.clientWidth * 0.18);
    const pageStep = Math.max(64, element.clientWidth * 0.72);
    const keySteps = {
      ArrowLeft: -step,
      ArrowRight: step,
      PageUp: -pageStep,
      PageDown: pageStep,
      Home: -Infinity,
      End: Infinity,
    };
    if (!(event.key in keySteps)) return;
    event.preventDefault();
    const delta = keySteps[event.key];
    if (delta === -Infinity) {
      element.scrollLeft = 0;
      return;
    }
    if (delta === Infinity) {
      element.scrollLeft = scrollState.max;
      return;
    }
    element.scrollLeft = Math.max(0, Math.min(scrollState.max, element.scrollLeft + delta));
  };

  const ariaLabel = label || t("table.horizontalScroll", "Horizontal table scroll");

  return (
    <div
      className={classNames(
        "wide-table-frame",
        scrollState.canScroll && "has-rail",
        scrollState.left > 1 && "has-left-overflow",
        scrollState.canScroll && scrollState.left < scrollState.max - 1 && "has-right-overflow",
        className,
      )}
    >
      <div className="wide-table-scroll" ref={scrollRef} onWheel={handleWheel}>
        {children}
      </div>
      {scrollState.canScroll ? (
        <div
          className="wide-table-rail"
          ref={railRef}
          role="scrollbar"
          tabIndex={0}
          aria-orientation="horizontal"
          aria-label={ariaLabel}
          aria-controls={controlsId}
          aria-valuemin={0}
          aria-valuemax={Math.round(scrollState.max)}
          aria-valuenow={Math.round(scrollState.left)}
          onPointerDown={handleRailPointerDown}
          onPointerMove={handleRailPointerMove}
          onKeyDown={handleRailKeyDown}
        >
          <span className="wide-table-track" aria-hidden="true" />
          <span
            className="wide-table-thumb"
            aria-hidden="true"
            style={{
              width: `${scrollState.thumbWidth}%`,
              left: `${scrollState.thumbLeft}%`,
            }}
          />
        </div>
      ) : null}
    </div>
  );
}

function OrdersTable({ t, rows }) {
  return (
    <WideTableFrame
      t={t}
      className="orders-table-frame"
      controlsId="orders-horizontal-table"
      label={t("table.ordersHorizontalScroll", "Orders table horizontal scroll")}
    >
      <table className="data-table orders-table" id="orders-horizontal-table">
        <thead>
          <tr>
            <th>{t("common.symbol", "Symbol")}</th>
            <th>{t("common.side", "Side")}</th>
            <th>{t("table.type", "Type")}</th>
            <th>{t("common.size", "Size")}</th>
            <th>{t("common.price", "Price")}</th>
            <th>{t("common.status", "Status")}</th>
            <th>{t("table.created", "Created")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={`${row.symbol}-${row.created}`}>
              <th>{row.symbol}</th>
              <td className={row.side === "Sell" ? "negative-text" : "positive-text"}>{choiceLabel(t, row.side)}</td>
              <td>{choiceLabel(t, row.type)}</td>
              <td>{row.size}</td>
              <td>{numberFormat.format(row.price)}</td>
              <td><span className="order-status">{choiceLabel(t, row.status)}</span></td>
              <td>{row.created}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </WideTableFrame>
  );
}

function AutopilotStepsView({ t, rows }) {
  return (
    <div className="autopilot-steps-view">
      <table className="data-table autopilot-steps-table">
        <thead>
          <tr>
            <th>{t("table.step", "Step")}</th>
            <th>{t("common.intent", "Intent")}</th>
            <th>{t("common.side", "Side")}</th>
            <th>{t("panels.conf", "Conf")}</th>
            <th>{t("common.risk", "Risk")}</th>
            <th>{t("common.outcome", "Outcome")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={6}>
                <strong>{t("panels.noAutopilotSteps", "No Autopilot steps")}</strong>
                <small>{t("panels.startAutopilot", "Start AI Autopilot to persist step history")}</small>
              </td>
            </tr>
          ) : (
            rows.map((row) => {
              const intent = autopilotStepIntent(row);
              const decision = autopilotStepDecision(row);
              const side = String(intent.side || "-").toLowerCase();
              const approved = decision.approved === true;
              const rejected = decision.approved === false;
              return (
                <tr key={row.id}>
                  <th>
                    <strong>#{row.runId}.{row.stepNumber}</strong>
                    <small>{formatClock(row.createdAt)}</small>
                  </th>
                  <td>
                    <strong>{intent.symbol || "-"}</strong>
                    <small>{intent.exchange || "local"} / {intent.sizeUsdt ? formatMoney(intent.sizeUsdt) : "-"}</small>
                  </td>
                  <td className={side === "sell" ? "negative-text" : "positive-text"}>{side === "-" ? "-" : choiceLabel(t, side)}</td>
                  <td>{formatConfidence(intent.confidence)}</td>
                  <td>
                    <span className={approved ? "risk-low" : rejected ? "order-status danger" : "order-status"}>
                      {approved ? t("choices.ok", "ok") : rejected ? t("choices.reject", "reject") : statusText(t, row.status)}
                    </span>
                  </td>
                  <td>{eventText(t, autopilotStepOutcome(row))}</td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}

function PaperResetDialog({ t, open, onClose, phrase, setPhrase, status, isResetting, onConfirm }) {
  useEscapeToClose(open && !isResetting, onClose);
  const dialogRef = useDialogFocus(open);
  if (!open) return null;

  function handleSubmit(event) {
    event.preventDefault();
    onConfirm();
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && !isResetting && onClose()}>
      <section className="credential-modal paper-reset-modal" role="dialog" aria-modal="true" aria-labelledby="paper-reset-title" aria-describedby="paper-reset-copy" ref={dialogRef}>
        <header className="credential-modal-header">
          <div>
            <h2 id="paper-reset-title">{t("panels.resetPaperTitle", "Reset paper ledger")}</h2>
            <span><AlertTriangle size={13} /> {t("panels.resetPaperSubtitle", "Clear local paper execution history")}</span>
          </div>
          <button className="icon-close" type="button" onClick={onClose} aria-label={t("panels.resetPaperClose", "Close reset confirmation")} disabled={isResetting}>
            <X size={16} />
          </button>
        </header>

        <form className="paper-reset-body" onSubmit={handleSubmit}>
          <div className="paper-reset-warning" id="paper-reset-copy">
            <Trash2 size={18} />
            <div>
              <strong>{t("panels.resetPaperWarningTitle", "Local paper ledger will be cleared")}</strong>
              <span>{t("panels.resetPaperWarningBody", "This removes saved paper execution rows from the local database. Strategy profiles, vault keys, and audit logs remain intact.")}</span>
            </div>
          </div>

          <label className="field paper-reset-phrase">
            <span>{t("panels.resetPaperPhraseLabel", "Confirmation phrase")}</span>
            <input
              data-autofocus
              value={phrase}
              onChange={(event) => setPhrase(event.target.value)}
              placeholder={t("panels.resetPaperPhrasePlaceholder", "Type RESET PAPER")}
              autoComplete="off"
              spellCheck="false"
              disabled={isResetting}
            />
          </label>

          <div className={classNames("paper-reset-status", status?.tone)}>
            <span>{statusText(t, status?.message || "Confirm reset")}</span>
            <code>RESET PAPER</code>
          </div>

          <footer className="paper-reset-actions">
            <button className="header-ghost-button" type="button" onClick={onClose} disabled={isResetting}>
              {t("common.cancel", "Cancel")}
            </button>
            <button className="save-credential danger-confirm" type="submit" disabled={isResetting}>
              <Trash2 size={13} />
              {isResetting ? t("panels.resetting", "RESETTING") : t("panels.confirmReset", "Confirm reset")}
            </button>
          </footer>
        </form>
      </section>
    </div>
  );
}

function PaperLedgerView({ t, account, rows, resetStatus, isResetting, isResetDisabled, onReset }) {
  return (
    <div className="paper-ledger-view">
      <div className="paper-account-strip">
        <span>
          {t("panels.paperEquity", "Equity")}
          <strong className={account.totalPnlUsdt >= 0 ? "positive-text" : "negative-text"}>{formatMoney(account.equityUsdt)}</strong>
        </span>
        <span>
          {t("panels.cash", "Cash")}
          <strong>{formatMoney(account.cashUsdt)}</strong>
        </span>
        <span>
          {t("panels.realized", "Realized")}
          <strong className={account.realizedPnlUsdt >= 0 ? "positive-text" : "negative-text"}>{formatSignedMoney(account.realizedPnlUsdt)}</strong>
        </span>
        <span>
          {t("panels.unrealized", "Unreal.")}
          <strong className={account.unrealizedPnlUsdt >= 0 ? "positive-text" : "negative-text"}>{formatSignedMoney(account.unrealizedPnlUsdt)}</strong>
        </span>
        <span>
          {t("panels.return", "Return")}
          <strong className={account.returnPct >= 0 ? "positive-text" : "negative-text"}>{formatSignedPct(account.returnPct)}</strong>
        </span>
      </div>
      <div className="paper-ledger-toolbar">
        <span className={resetStatus?.tone ? `${resetStatus.tone}-text` : ""}>{statusText(t, resetStatus?.message || "Ready")}</span>
        <button className={classNames(isResetDisabled && "blocked-action")} type="button" onClick={onReset} disabled={isResetting} title={isResetDisabled ? t("toast.stopAiFirst", "Stop AI before resetting paper ledger") : t("panels.resetTitle", "Type RESET PAPER to confirm")}>
          <Trash2 size={12} />
          {isResetting ? t("panels.resetting", "RESETTING") : t("panels.reset", "RESET")}
        </button>
      </div>
      <div className="paper-table-wrap">
        <PaperExecutionsTable t={t} rows={rows} />
      </div>
    </div>
  );
}

function PaperExecutionsTable({ t, rows }) {
  return (
    <table className="data-table paper-executions-table">
      <thead>
          <tr>
          <th>{t("common.intent", "Intent")}</th>
          <th>{t("common.mode", "Mode")}</th>
          <th>{t("common.side", "Side")}</th>
          <th>{t("common.size", "Size")}</th>
          <th>{t("table.intentPx", "Intent Px")}</th>
          <th>{t("common.risk", "Risk")}</th>
          <th>{t("common.fill", "Fill")}</th>
          <th>{t("common.fee", "Fee")}</th>
        </tr>
      </thead>
      <tbody>
        {rows.length === 0 ? (
          <tr>
            <td colSpan={8}>
              <strong>{t("panels.noPaperRecords", "No paper execution records")}</strong>
              <small>{t("panels.runPaperAutopilot", "Run AI STEP or Paper/Shadow Autopilot")}</small>
            </td>
          </tr>
        ) : (
          rows.map((row) => (
            <tr key={row.id}>
              <th>
                <strong>{row.symbol}</strong>
                <small>{formatDateTime(row.createdAt)} / {row.source}{row.runId ? ` #${row.runId}` : ""}</small>
              </th>
              <td>{choiceLabel(t, row.mode)}</td>
              <td className={row.side === "sell" ? "negative-text" : "positive-text"}>{choiceLabel(t, row.side)}</td>
              <td>{formatMoney(row.sizeUsdt)}</td>
              <td>{formatMoney(row.intentPrice)}</td>
              <td>
                <span className={row.riskStatus === "approved" ? "risk-low" : "order-status"}>{choiceLabel(t, row.riskStatus)}</span>
              </td>
              <td>{choiceLabel(t, row.fillStatus)}</td>
              <td>{formatMoney(row.feeUsdt || 0)}</td>
            </tr>
          ))
        )}
      </tbody>
    </table>
  );
}

function EventLog({ t, events, compact = false, autoScroll = true }) {
  const logRef = useRef(null);
  useEffect(() => {
    if (!autoScroll || !logRef.current) return;
    logRef.current.scrollTop = 0;
  }, [autoScroll, compact, events]);

  return (
    <div className={classNames("event-log", compact && "compact-log")} ref={logRef}>
      <table className="data-table">
        <thead>
          <tr>
            <th>{t("common.time", "Time")}</th>
            <th>{t("common.type", "Type")}</th>
            <th>{t("common.symbol", "Symbol")}</th>
            {!compact ? <th>{t("common.action", "Action")}</th> : null}
            <th>{t("common.price", "Price")}</th>
            <th>{t("panels.resultNote", "Result / Note")}</th>
          </tr>
        </thead>
        <tbody>
          {events.map((event, index) => (
            <tr key={`${event.time}-${event.type}-${index}`}>
              <td>{event.time}</td>
              <td className={`${event.level}-text`}>{choiceLabel(t, event.type)}</td>
              <td>{event.symbol}</td>
              {!compact ? <td className={event.action === "SELL" ? "negative-text" : event.action === "BUY" ? "positive-text" : ""}>{eventText(t, event.action)}</td> : null}
              <td>{event.price ? numberFormat.format(event.price) : "-"}</td>
              <td>
                <strong>{eventText(t, event.result)}</strong>
                <small>{eventText(t, event.note)}</small>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
