# CCVar Quant Lab

Lightweight local-first AI quant trading client for Web, macOS, and Windows.

CCVar Quant Lab is built around a safe progression: AI simulation, shadow/paper trading, backtesting, audited risk gates, and finally guarded testnet/demo execution. Production/mainnet trading is intentionally disabled in this version.

中文简介：CCVar Quant Lab 是一个轻量、本地优先的 Web3 虚拟币 AI 量化交易客户端，支持 Web、macOS 客户端和 Windows 客户端。当前版本只面向模拟、纸面交易、Binance Spot Testnet/Demo 和 OKX Demo Trading；不支持 production/mainnet 实盘交易。

## What Ships

- Web app served by the Go local API at `http://127.0.0.1:8787/`.
- Native macOS desktop package with two apps: `CCVar Quant Lab.app` as the standalone WKWebView client, and `CCVar Quant Lab Web.app` as the browser launcher.
- Native Windows desktop package with two launchers: `CCVar Quant Lab.exe` as the standalone WebView2 client, and `Start CCVar Quant Lab Web.cmd` as the browser launcher.
- Portable browser-launcher desktop packages under `dist/desktop/` for deterministic release verification.
- SQLite local storage, encrypted credential Vault, hash-chained audit log, paper ledger, backtest history, account snapshots, and guarded execution ledger.
- Bilingual UI with Simplified Chinese and English language switching.
- Final audit scripts that block completion until both Binance and OKX real sandbox acceptance reports pass.

## Stack

- Go local API
- SQLite local storage via pure Go `modernc.org/sqlite`
- React + Vite frontend
- Lightweight Charts for market/equity views
- IBM Plex Sans/Mono typography
- macOS AppKit + WKWebView native host
- Windows WinForms + WebView2 native host

## Internationalization

The UI language files live under `src/i18n/`, one module per locale:

- `src/i18n/zh-CN.js`
- `src/i18n/en-US.js`
- `src/i18n/index.js`

The language switcher is in the top bar and persists the selected locale in `localStorage` as `ccvar.locale`. The same embedded UI is used by the Web server, macOS native app, Windows native app, and browser launchers, so all delivery targets share the same i18n resources.

When adding new UI copy, add the key to both locale files and call the translator through `t("path.to.key", "English fallback")`. Keep protocol values, exchange names, symbols, environment names, and audit/model output as raw data unless they are purely UI chrome.

中文维护说明：所有页面文案都放在 `src/i18n/`，每个语种一个文件。新增页面或交互状态时，请同时补齐 `zh-CN.js` 和 `en-US.js`，不要把新的固定文案直接写死在组件里。

## Run

For development, start the API:

```bash
go run ./cmd/ccvar-quant
```

Start the Vite UI in another terminal:

```bash
npm run dev
```

For a single-process local client, bundle the UI into the Go binary:

```bash
npm run build:app
./bin/ccvar-quant
```

Then open `http://127.0.0.1:8787/`. The same process serves the React workstation and `/api/*` routes. The UI falls back to bundled seed data when the API is not running in development mode. The local API persists seed data in `data/ccvar_quant.db`.

Runtime options:

```bash
./bin/ccvar-quant --addr 127.0.0.1:8787 --db data/ccvar_quant.db --open
./bin/ccvar-quant --version
```

Environment variable equivalents are `CCVAR_ADDR`, `CCVAR_DB_PATH`, and `CCVAR_OPEN_BROWSER=true`. The API is intended for local browser clients: CORS accepts same-machine origins such as `127.0.0.1`, `localhost`, and `::1`, and rejects remote webpage origins.

If the client is launched with `--open` while another CCVar Quant instance is already serving the same address, the new process verifies `/api/health`, opens the existing browser URL, and exits cleanly. If the port is occupied by a different program, startup fails instead of attaching to it.

## GitHub Actions

The repository includes `.github/workflows/build-clients.yml`, `.github/workflows/native-desktop-clients.yml`, and `.github/workflows/real-sandbox-acceptance.yml`.

It runs on push, pull request, and manual dispatch:

- `Test and Build Web UI`: installs Node/Go dependencies, builds the embedded Web UI, runs `go test ./...`, builds the local Web client binary, and uploads `ccvar-web-embedded-ui`.
- `Package macOS and Windows Clients`: runs `npm run verify:release`, which rebuilds and verifies deterministic macOS/Windows browser-launcher desktop packages, then uploads `ccvar-desktop-release`.
- `Native macOS Client`: builds the standalone AppKit/WKWebView client and the macOS browser-launcher app on a macOS runner, then uploads `ccvar-native-macos`.
- `Native Windows Client`: builds the standalone WinForms/WebView2 client and the Windows browser-launcher command on a Windows runner, then uploads `ccvar-native-windows`.

CI artifacts:

- `ccvar-web-embedded-ui`: static assets under `cmd/ccvar-quant/web`.
- `ccvar-desktop-release`: portable browser-launcher macOS arm64 zip, Windows amd64 zip, `SHA256SUMS.txt`, and `release-manifest.json`.
- `ccvar-native-macos`: native macOS zip containing `CCVar Quant Lab.app`, `CCVar Quant Lab Web.app`, and `SHA256SUMS.txt`.
- `ccvar-native-windows`: native Windows zip containing `CCVar Quant Lab.exe`, `Start CCVar Quant Lab Web.cmd`, and `SHA256SUMS.txt`.

No extra GitHub Secrets are required to build unsigned native desktop packages. Apple Developer ID, notarization credentials, or Windows code-signing certificates can be added later when you want notarized/signed public distribution.

Real sandbox completion is intentionally manual. Add these repository secrets, then run the `Real Sandbox Acceptance` workflow from GitHub Actions:

- `BINANCE_TESTNET_API_KEY`
- `BINANCE_TESTNET_API_SECRET`
- `OKX_DEMO_API_KEY`
- `OKX_DEMO_API_SECRET`
- `OKX_DEMO_API_PASSPHRASE`

That workflow starts the local client on `127.0.0.1:8787`, runs `audit:final` with `CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true`, enforces `npm run audit:complete`, and uploads sanitized final audit reports. It does not print or artifact API keys, API secrets, OKX passphrases, Vault passphrases, ciphertext, salt, or nonce.

For a step-by-step operator checklist, see `docs/real-sandbox-acceptance.zh-CN.md`. For local runs, copy `.env.acceptance.example` to `.env.acceptance.local`, fill sandbox-only credentials, configure the non-secret acceptance parameters if needed, and source it before `npm run audit:final`.

Cross-compile examples:

```bash
GOOS=darwin GOARCH=arm64 go build -o bin/ccvar-quant-darwin-arm64 ./cmd/ccvar-quant
GOOS=windows GOARCH=amd64 go build -o bin/ccvar-quant-windows-amd64.exe ./cmd/ccvar-quant
```

Desktop delivery packages:

```bash
npm run package:desktop
npm run package:native:macos
npm run package:native:windows
```

`package:desktop` creates deterministic portable browser-launcher packages used by the release verifier. `package:native:macos` must run on macOS because it compiles the AppKit/WKWebView host; it outputs a universal macOS native package. `package:native:windows` must run on Windows with .NET 8 because it compiles the WinForms/WebView2 host.

Full release verification:

```bash
npm run verify:release
```

`verify:release` runs Go tests, rebuilds desktop packages, verifies consecutive desktop package SHA-256 and release-manifest reproducibility, verifies zip SHA-256 checksums and release manifest, checks i18n resources in source and the embedded UI bundle, extracts both desktop zip files, validates critical extracted files against the manifest, checks macOS executable bits and packaged Go build metadata, checks package cleanliness, starts an isolated local client on `127.0.0.1:8790` with a temporary SQLite database, runs no-secret smoke and acceptance skip checks, then starts a loopback-only mock exchange to exercise the full Vault -> Account Sync -> Live Guard -> Live Execute acceptance path with fake Binance testnet and OKX demo credentials.

Desktop packaging uses a deterministic release timestamp by default so repeated builds from the same source produce the same zip hashes and manifest. Official release stamping can be overridden with `SOURCE_DATE_EPOCH` or `CCVAR_RELEASE_GENERATED_AT`.

Final delivery audit:

```bash
npm run audit:final
npm run audit:complete
```

`audit:final` runs shell syntax checks, delegates to `verify:release`, optionally smoke-tests the currently running local client at `http://127.0.0.1:8787`, and writes a sanitized machine-readable handoff report to `dist/final-audit/final-audit-latest.json`. By default it records real exchange sandbox acceptance as not run, because that proof requires operator-provided Binance Spot Testnet or OKX Demo Trading environment variables. To include external sandbox proof for both exchanges in the same report, set `CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true` and `CCVAR_FINAL_AUDIT_REAL_EXCHANGES=Binance,OKX` with exchange-specific sandbox credentials.

`audit:complete` is the final completion gate. It reads `dist/final-audit/final-audit-latest.json`, verifies release artifacts, safety evidence, sandbox credential readiness, and fails until both Binance and OKX real sandbox acceptance reports are present and passed.

This creates:

- `dist/desktop/CCVar Quant Lab.app`
- `dist/desktop/ccvar-quant-lab-macos-arm64.zip`
- `dist/desktop/CCVar Quant Lab Windows x64/`
- `dist/desktop/ccvar-quant-lab-windows-amd64.zip`
- `dist/desktop/SHA256SUMS.txt`
- `dist/desktop/release-manifest.json`
- `dist/native/macos/ccvar-quant-lab-macos-native.zip`
- `dist/native/windows/ccvar-quant-lab-windows-native.zip`

The native macOS package contains two apps. `CCVar Quant Lab.app` starts the local API and renders the UI inside a native WKWebView window. `CCVar Quant Lab Web.app` starts the same local API and opens the default browser. The native Windows package mirrors that split with `CCVar Quant Lab.exe` for the WebView2 desktop client and `Start CCVar Quant Lab Web.cmd` for the browser launcher.

The macOS clients store data in `~/Library/Application Support/CCVar Quant Lab/ccvar_quant.db` and logs in `~/Library/Application Support/CCVar Quant Lab/logs/client.log`. The Windows clients store data in `%APPDATA%\CCVar Quant Lab\ccvar_quant.db` and logs in `%APPDATA%\CCVar Quant Lab\logs\client.log`. All desktop entries start the local API on `127.0.0.1:8787`; browser launchers also open the default browser. `CCVAR_ADDR` and `CCVAR_DB_PATH` can override those defaults.

`SHA256SUMS.txt` and `release-manifest.json` are generated by the packaging script so desktop zip files can be verified before distribution. The release manifest records zip artifact hashes plus critical package file hashes for README files, launchers, desktop binaries, bundled docs, and macOS metadata. The macOS and Windows packages also include README files that state the local URL, data paths, environment overrides, checksum workflow, and production/mainnet-disabled safety boundary.
Both desktop packages include `docs/operator-runbook.zh-CN.md` and `docs/safety.md`; the macOS app also embeds the same docs under `Contents/Resources/docs`.

Local no-secret smoke test:

```bash
npm run smoke:local
```

Sandbox credential environment readiness, without printing secret values:

```bash
npm run acceptance:env
```

Testnet/demo acceptance with real exchange sandbox credentials:

```bash
CCVAR_ACCEPTANCE_EXCHANGE=Binance \
BINANCE_TESTNET_API_KEY=... \
BINANCE_TESTNET_API_SECRET=... \
npm run acceptance:live
```

```bash
CCVAR_ACCEPTANCE_EXCHANGE=OKX \
OKX_DEMO_API_KEY=... \
OKX_DEMO_API_SECRET=... \
OKX_DEMO_API_PASSPHRASE=... \
npm run acceptance:live
```

`npm run acceptance:live` skips safely when `CCVAR_ACCEPTANCE_EXCHANGE` is unset. When credentials are provided it creates a temporary encrypted Vault key, performs read-only account sync, unlocks Live Guard, runs validation-only live execution, checks redaction/export/audit, then locks Guard and deletes the temporary key by default. Non-validation demo/testnet submission requires both `CCVAR_ACCEPTANCE_VALIDATION_ONLY=false` and `CCVAR_ACCEPTANCE_ALLOW_DEMO_SUBMIT=true`.

The acceptance run writes a sanitized JSON report to `dist/acceptance/live-acceptance-latest.json` by default. Override with `CCVAR_ACCEPTANCE_REPORT_PATH=/path/report.json` or disable report writing with `CCVAR_ACCEPTANCE_REPORT_PATH=none`.

Final audit with both real sandbox exchanges, once those keys are available:

```bash
CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true \
CCVAR_FINAL_AUDIT_REAL_EXCHANGES=Binance,OKX \
BINANCE_TESTNET_API_KEY=... \
BINANCE_TESTNET_API_SECRET=... \
OKX_DEMO_API_KEY=... \
OKX_DEMO_API_SECRET=... \
OKX_DEMO_API_PASSPHRASE=... \
npm run audit:final
```

When multiple real exchanges are requested, use exchange-specific variables as shown above rather than generic `CCVAR_ACCEPTANCE_API_KEY` / `CCVAR_ACCEPTANCE_API_SECRET`, so credentials cannot be accidentally reused across exchanges.

Release verification can redirect private Binance/OKX testnet/demo HTTP calls to a local mock only when `CCVAR_ENABLE_LOOPBACK_EXCHANGE_MOCKS=true` is set and `CCVAR_BINANCE_PRIVATE_MOCK_URL` / `CCVAR_OKX_PRIVATE_MOCK_URL` point to `localhost`, `127.0.0.1`, or `::1`. Remote hosts, paths, query strings, and credentials in those URLs are rejected at startup. This switch is for automated local acceptance only; normal runs keep using the real Binance Spot Testnet/Demo and OKX Demo private endpoints.

For Binance Spot Testnet and OKX Demo Trading credential setup, guarded execution checks, and acceptance criteria, see [docs/operator-runbook.zh-CN.md](docs/operator-runbook.zh-CN.md).

## Market Data

The current build connects only to public market-data APIs:

- Binance: `https://data-api.binance.vision/api/v3/ticker/bookTicker`, `ticker/24hr`, and `klines`
- OKX: `https://www.okx.com/api/v5/market/ticker` and `market/candles`

The UI can switch between Binance and OKX. If an exchange request fails, the API keeps the lab usable by returning SQLite seed data and adds a warning event to the event log.

## AI Policy

The current AI layer is a local deterministic policy, not a hosted LLM call. `Local AI Policy v0.2.0` evaluates public-market features such as spread quality, liquidity depth, momentum, trend alignment, and funding pressure, then emits a trade intent with model identity, policy version, signal, confidence, TTL, feature impacts, and reasoning.

`/api/lab-state`, `/api/simulate/step`, Shadow/Paper Autopilot, and Live Autopilot use this same policy path, so the UI verdict, simulation intent, and guarded execution plan are aligned. The policy never reads exchange secrets, never sends data to a remote model provider, and cannot bypass Risk Profile, Kill Switch, Live Guard, Vault, Account Sync, or guarded execution checks.

## Credential Vault

The Exchange Vault can save Binance and OKX API credentials locally for guarded testnet/demo execution. Credentials are encrypted before they reach SQLite:

- AES-GCM encrypted payload.
- PBKDF2-SHA256 key derivation from a user passphrase.
- Passphrases are never stored.
- The list API returns only exchange, label, permissions, timestamps, and a masked API key.
- Withdrawal permission is rejected.
- OKX private credentials require a separate OKX API passphrase; the vault passphrase is only for local encryption.
- The Vault includes a read-only `Connection Test` that decrypts a selected key only in memory, calls testnet/demo account sync, persists a sanitized snapshot, and reports can-trade, balance count, open-order count, snapshot id, and sync time without placing orders.

API surface:

- `GET /api/credentials`
- `POST /api/credentials`
- `DELETE /api/credentials?id={id}`

## Live Guard And Audit

The current build includes the control layer needed for guarded AI execution:

- `GET /api/live-guard`
- `GET /api/app-info`
- `GET /api/preflight`
- `POST /api/live-guard` with `action: "unlock"` or `action: "lock"`
- `GET /api/kill-switch`
- `POST /api/kill-switch` with `action: "activate"` or `action: "resume"`
- `GET /api/risk-profile`
- `PUT /api/risk-profile`
- `GET /api/strategy-profile`
- `PUT /api/strategy-profile`
- `POST /api/backtest/run`
- `GET /api/backtest-runs`
- `GET /api/autopilot`
- `POST /api/autopilot` with `action: "start"`, `action: "stop"`, or `action: "step"`
- `GET /api/autopilot-runs`
- `GET /api/autopilot-steps?runId={id}`
- `GET /api/paper-executions`
- `POST /api/paper-executions/reset`
- `GET /api/paper-account`
- `POST /api/live-execute`
- `GET /api/live-executions`
- `POST /api/live-reconcile`
- `GET /api/live-reconciliations`
- `GET /api/account-sync`
- `POST /api/account-sync`
- `GET /api/audit-log`
- `GET /api/local-data`
- `POST /api/local-data/prune`
- `GET /api/export`

Live Guard only unlocks `testnet` or `demo` sessions, requires the exact phrase `ENABLE TESTNET LIVE`, expires within 60-900 seconds, and writes every approved or rejected attempt into a hash-chained audit log. The server-side Kill Switch is separate from Live Guard: activating it immediately locks Live Guard, rejects `/api/live-execute`, rejects `/api/simulate/step`, and writes the transition to audit. Resuming the Kill Switch clears the global halt but does not unlock Live Guard.

`/api/app-info` reports the local client version, bind address, runtime architecture, SQLite database path/size, packaged runbook/safety document availability, supported exchanges, and safety flags. The Live Guard modal displays the same client diagnostics so packaged macOS/Windows builds are easier to verify in place.

`/api/preflight` runs a local readiness check before AI execution. It reports ready/warn/block counts for local Origin protection, SQLite availability, production trading disablement, Kill Switch state, Live Guard state, Autopilot state, audit hash verification, Vault credential availability, Risk Profile, Strategy Profile, and Binance/OKX public-market connectivity. It also includes a `live_autopilot` aggregate check that combines Guard, Vault, recent Account Sync, market connectivity, and profile readiness before the UI presents Live Autopilot as ready. Missing trade credentials, locked Guard state, missing or stale account snapshots, or temporary public-market failures are warnings; audit failure, missing database, active Kill Switch, profile blocks, adapter blocks, or broken runtime wiring are blocking checks.

The Live Guard modal also shows a local `Live Setup` checklist for the guarded execution path: Vault trade key, in-memory vault passphrase, Guard session, recent Account Sync, Live Autopilot preflight, and Execute readiness. The checklist is derived from local state only and never displays secrets.

`/api/risk-profile` stores the local AI risk guardrails in SQLite: minimum confidence, max order notional, max symbol exposure, max total exposure, max daily drawdown, max consecutive losses, and max spread. Simulation, Paper/Shadow Autopilot, Live Autopilot, and guarded live execution all read this profile before approving an AI intent. Live Guard's per-session `maxOrderUsdt` remains an additional cap; the stricter of the saved profile and the current unlock session is used.

`/api/strategy-profile` stores the AI intent defaults in SQLite: strategy name, exchange, symbol, side, order notional, Autopilot interval, and optional max steps. Manual AI simulation and Autopilot use this profile to seed trade-intent generation; manual Live Execute still uses the guarded form for credential-bound order details.

`/api/backtest/run` runs a lightweight local moving-average momentum backtest using the current Strategy Profile defaults unless request fields override exchange, symbol, side, notional, interval, candle limit, or fast/slow windows. It first tries public exchange candles and explicitly marks the result as `live public`; if public candles are unavailable it falls back to local seed candles and returns a warning with `marketDataSource: "local seed"`. Each run is persisted to SQLite and listed through `/api/backtest-runs` for local review. Backtest never reads credentials and never writes orders.

`/api/autopilot` runs the same AI decision loop without manual clicking. Shadow/Paper modes call the simulation runner on an interval. Live mode first performs read-only account sync, generates a local AI live plan from public market data, the sanitized account snapshot, and the Strategy Profile, then routes that planned intent through the existing Live Guard -> Vault -> Risk -> exchange execution path. Live Autopilot still supports only testnet/demo, can be validation-only, and requires the vault passphrase at start time; that passphrase is held only in memory for the active run and is not returned by API responses or audit entries.

- Autopilot starts with an immediate first step, then continues at `intervalSeconds` until stopped or until `maxSteps` is reached.
- Kill Switch activation stops any running Autopilot immediately and rejects new Autopilot starts while active.
- Autopilot state returns sanitized run id, mode, exchange, symbol, step counts, latest events, latest result, and status. It never returns API keys, secrets, API passphrases, or vault passphrases.
- Autopilot run summaries and step payloads are persisted in SQLite for local review through `/api/autopilot-runs` and `/api/autopilot-steps`; the UI exposes a compact `AI Steps` ledger with run.step id, intent, side, confidence, risk status, and outcome.
- Manual AI simulation and Shadow/Paper Autopilot persist sanitized paper execution records to SQLite. `GET /api/paper-executions` returns the latest intent, risk, fill, event, mode, source, run id, symbol, side, notional, fee, and slippage records for the UI Trades ledger.
- `POST /api/paper-executions/reset` clears the local paper/shadow execution ledger only after the exact confirmation phrase `RESET PAPER`, rejects while Autopilot is running, and writes approved or rejected reset attempts to the hash-chained audit log. It never touches credentials, live execution ledgers, account snapshots, or audit entries.
- `GET /api/paper-account` derives a local simulation account snapshot from recent paper execution records: cash, equity, realized/unrealized PnL, fees, open notional, return percentage, counts, and current simulated positions. It is a local paper/shadow view only and never touches exchange credentials.

`/api/export` returns a no-store JSON workspace export for local review and backup. It includes Strategy Profile, Risk Profile, sanitized Autopilot state/runs/steps, backtest run history, paper account snapshot, paper execution records, live execution ledger records, live reconciliation records, audit verification, and recent audit entries. It deliberately excludes the credential table, API keys, API secrets, API passphrases, vault passphrases, credential ciphertext, salts, and nonces.

`/api/local-data` reports local SQLite record counts for research, paper, live, audit, and credential tables. `/api/local-data/prune` applies a conservative retention cleanup only after the exact phrase `PRUNE LOCAL DATA`, rejects while Autopilot is running, writes the attempt to audit, and prunes only local research/simulation history: backtest runs, Autopilot runs/steps, paper execution records, and account snapshots. It deliberately never deletes credentials, audit entries, live execution ledger rows, live reconciliation rows, Risk Profile, or Strategy Profile.

`/api/live-execute` runs the current AI live intent through Live Guard, Vault decryption, trade-permission checks, public market snapshot loading, a recent persisted account snapshot, deterministic risk approval, exchange-specific private signing, and audit logging. Live Autopilot can pass an internal planned intent and AI trace into this path, but external JSON callers cannot set those internal fields. It never targets production/mainnet endpoints.

- Binance supports Spot Testnet/Demo validation via `POST /api/v3/order/test`; disabling `validationOnly` sends to `POST /api/v3/order` on the testnet/demo base URL only.
- OKX supports local signed preflight when `validationOnly` is enabled; disabling `validationOnly` sends to `POST /api/v5/trade/order` with `x-simulated-trading: 1` on the OKX Demo Trading URL.
- Live execution requires a successful `/api/account-sync` snapshot from the same credential, exchange, environment, and symbol within five minutes; missing, stale, or non-tradable snapshots are rejected before risk evaluation.
- Risk evaluation uses the persisted snapshot's available quote/base balance, total equity estimate, open-order notional, and current symbol exposure instead of a default account assumption.
- Generated live intents are persisted to a structured SQLite execution ledger after risk evaluation. The ledger stores sanitized intent JSON, risk verdict JSON, optional execution report JSON, client order id, exchange order id, risk status, execution status, exchange, environment, symbol, and notional.
- `GET /api/live-executions` returns the latest ledger records for review and reconciliation.

`/api/live-reconcile` is a read-only testnet/demo order-status check for submitted execution ledger rows. It rejects validation-only rows and risk-rejected/unsubmitted rows, decrypts the saved credential only for the request lifetime, queries the exchange order status, persists a sanitized reconciliation record in SQLite, and writes the outcome to audit.

- Binance reconciliation uses signed `GET /api/v3/order` with `orderId` or `origClientOrderId` on Spot Testnet/Demo.
- OKX reconciliation uses signed `GET /api/v5/trade/order` with `ordId` or `clOrdId` and `x-simulated-trading: 1` on Demo Trading.
- `GET /api/live-reconciliations` returns latest sanitized reconciliation records by optional `liveExecutionId` and never returns API keys, secrets, API passphrases, or vault passphrases.

`/api/account-sync` is read-only and testnet/demo-only. It requires a saved encrypted credential and the vault passphrase for each request, then returns a sanitized account snapshot for balances and open orders:

- Binance uses signed `GET /api/v3/account` and `GET /api/v3/openOrders` on Spot Testnet/Demo.
- OKX uses signed `GET /api/v5/account/balance` and `GET /api/v5/trade/orders-pending` with `x-simulated-trading: 1` on Demo Trading.
- The Exchange Vault `Connection Test` uses this same read-only path so operators can verify a saved key before unlocking Live Guard or attempting validation-only execution.
- Successful snapshots are persisted locally in SQLite with snapshot id, exchange, environment, symbol, balance count, open-order count, and sanitized JSON.
- `GET /api/account-sync` restores the latest persisted snapshot by optional `credentialId`, `exchange`, `environment`, and `symbol` filters.
- Every approved, rejected, or failed sync attempt is written to the hash-chained audit log without secrets.

## Safety

This prototype does not submit production/mainnet exchange orders and does not sync production/mainnet account data. Live execution is limited to Binance Spot Testnet/Demo and OKX Demo Trading, and only after Live Guard unlock, encrypted vault decryption, trade permission, recent read-only account sync, and deterministic risk approval. Account sync is read-only and limited to testnet/demo. Public market adapters still return `ErrTradingDisabled` from their generic order methods; guarded private execution is routed through `/api/live-execute`.

See [docs/safety.md](docs/safety.md) for the current safety contract.
