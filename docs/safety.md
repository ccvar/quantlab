# Safety Contract

CCVar AI Quant Lab is being built in phases. The current phase is market-data-backed simulation plus guarded testnet/demo execution.

## Current Guarantees

- API keys can be saved only through the Exchange Vault and are encrypted before SQLite persistence.
- Local API CORS accepts only same-machine origins such as `127.0.0.1`, `localhost`, and `::1`; remote webpage origins are rejected before hitting API handlers.
- Duplicate `--open` or desktop launches reuse an occupied local address only after `/api/health` confirms the existing process reports `service: ccvar-quant`; unrelated port conflicts fail closed.
- `/api/app-info` reports client version, bind address, runtime, SQLite database file information, packaged runbook/safety document availability, supported exchanges, and explicit safety flags without exposing credential payloads or document contents.
- `/api/preflight` reports local readiness without secrets: Origin guard, SQLite, production-disabled flags, Kill Switch, Live Guard, Autopilot, audit hash verification, Vault credential count, Risk Profile, Strategy Profile, Binance/OKX public-market connectivity, and a `live_autopilot` aggregate readiness check.
- The `live_autopilot` preflight aggregate combines Guard, Vault, recent Account Sync, market connectivity, and profile readiness without exposing credentials, account payloads, or passphrases.
- Preflight blocks on unsafe or broken local state such as active Kill Switch, failed audit verification, missing database, invalid profile wiring, or blocked Live Autopilot dependencies; missing trade credentials, locked Guard state, missing/stale account sync, and temporary public-market connectivity failures are warnings.
- The current AI layer is `Local AI Policy v0.2.0`, a local deterministic policy. It does not call an external model provider and does not transmit market data, account snapshots, credentials, or passphrases off the machine.
- AI policy outputs include model identity, policy version, signal, confidence, TTL, feature impacts, and reasoning so simulation, Autopilot, and Live Autopilot steps are reviewable.
- Passphrases are not stored.
- Credential list APIs return only metadata and masked API keys, never secrets.
- Withdrawal permission is rejected at the vault boundary.
- OKX API passphrase is stored only inside the encrypted credential payload; it is separate from the local vault passphrase.
- Binance and OKX adapters use public market-data endpoints only.
- `PlaceOrder` and `CancelOrder` return `ErrTradingDisabled` in the public adapters.
- Saved credentials are wired only to the guarded `/api/live-execute` path, never to production/mainnet adapters.
- Live Guard can unlock only `testnet` or `demo` sessions, requires the exact phrase `ENABLE TESTNET LIVE`, and expires within 60-900 seconds.
- Production/mainnet live trading unlock is rejected.
- Live Guard unlock, rejection, and lock events are written to a hash-chained audit log.
- The server-side Kill Switch can be activated through `/api/kill-switch`; it immediately locks Live Guard and rejects AI simulation steps and guarded live execution until resumed.
- Kill Switch activate/resume events are written to the hash-chained audit log.
- Resuming the Kill Switch does not unlock Live Guard; a fresh explicit Live Guard unlock is still required for any testnet/demo execution attempt.
- `/api/risk-profile` persists the local risk guardrails in SQLite and writes updates to the hash-chained audit log.
- Simulation, Paper/Shadow Autopilot, Live Autopilot, and guarded live execution all read the persisted Risk Profile before approving an AI intent.
- Live Guard's session-level `maxOrderUsdt` is an additional cap; guarded execution uses the stricter of the persisted Risk Profile and the current unlock session.
- `/api/strategy-profile` persists AI intent defaults in SQLite and writes updates to the hash-chained audit log.
- Manual simulation and Autopilot use the Strategy Profile to choose exchange, symbol, side, notional, interval, and max-step defaults.
- Strategy Profile settings cannot bypass Risk Profile, Kill Switch, Live Guard, Vault, Account Sync, or guarded execution requirements.
- `/api/backtest/run` uses public exchange candles or clearly marked local seed candles only. It persists sanitized backtest results to SQLite for `/api/backtest-runs`, has no credential or vault access, does not write paper/live ledgers, and cannot submit exchange orders.
- `/api/autopilot` can run AI steps automatically in `shadow`, `paper`, or guarded `live` mode.
- Shadow/Paper Autopilot uses the simulation runner and public market data only.
- Live Autopilot performs read-only account sync before each execution attempt, creates a local AI live plan from public market data, the sanitized account snapshot, and the Strategy Profile, then routes that planned intent through the same Live Guard, Vault, risk, and testnet/demo execution path as `/api/live-execute`.
- Live Autopilot stores and surfaces only sanitized `aiPlan` fields. External `/api/live-execute` JSON callers cannot set internal planned-intent or AI-trace fields, so confidence and policy metadata cannot be forged through the public API.
- Live Autopilot requires credential id and vault passphrase at start time; the passphrase is held only in memory for the active run and is cleared on stop, completion, halt, or process exit.
- Autopilot API responses, audit entries, and UI state never include API keys, secrets, API passphrases, or vault passphrases.
- Kill Switch activation stops a running Autopilot immediately and rejects new Autopilot starts while active.
- Autopilot run summaries and step results are persisted in SQLite as sanitized records. `/api/autopilot-runs` returns run metadata, `/api/autopilot-steps` returns step result/events JSON by run id, and the UI shows a compact AI Steps ledger without secrets.
- Manual AI simulation and Shadow/Paper Autopilot write sanitized paper execution records to SQLite. `/api/paper-executions` returns intent/risk/fill/event JSON and summary fields only; it has no credential or vault access.
- `/api/paper-account` derives a local simulation account snapshot from sanitized paper execution records. It reports cash, equity, realized/unrealized PnL, fees, return percentage, and simulated positions without exchange credential access.
- `/api/paper-executions/reset` clears only local paper/shadow simulation records after the exact `RESET PAPER` confirmation phrase. It rejects while Autopilot is running and writes approved or rejected attempts into the hash-chained audit log.
- Paper execution records are simulation artifacts. They do not submit exchange orders and cannot bypass Risk Profile, Kill Switch, Live Guard, Vault, Account Sync, or guarded execution requirements.
- `/api/live-execute` requires Live Guard unlocked, a decryptable vault credential, trade permission, a fresh public market snapshot, a recent persisted account snapshot, and risk approval before exchange submission.
- Missing, stale, or non-tradable account snapshots are rejected before risk evaluation and written to audit.
- Live execution risk uses the persisted account snapshot's available balance, equity estimate, open-order notional, and symbol exposure instead of a default account assumption.
- Binance private execution is restricted to Spot Testnet/Demo base URLs. Validation mode calls `POST /api/v3/order/test`; submit mode calls `POST /api/v3/order` only on testnet/demo.
- OKX private execution is restricted to Demo Trading. Validation mode signs a local preflight without network submission; submit mode calls `POST /api/v5/trade/order` with `x-simulated-trading: 1`.
- Generated live intents are persisted in a structured SQLite execution ledger after risk evaluation, including risk-rejected, failed, validated, and submitted outcomes.
- `GET /api/live-executions` returns only sanitized ledger records: intent view, risk decision, optional execution report, order ids, statuses, exchange, environment, symbol, and notional. It never decrypts credentials and never returns API keys, secrets, API passphrases, or vault passphrases.
- `/api/live-reconcile` is read-only, requires a decryptable vault credential and passphrase on every request, and supports only `testnet` or `demo`.
- Live reconciliation rejects validation-only executions and risk-rejected/unsubmitted executions because they do not represent an exchange order to query.
- Binance order reconciliation is restricted to signed Spot Testnet/Demo `GET /api/v3/order` reads by `orderId` or `origClientOrderId`.
- OKX order reconciliation is restricted to signed Demo Trading `GET /api/v5/trade/order` reads by `ordId` or `clOrdId` with `x-simulated-trading: 1`.
- Successful reconciliation checks are persisted locally as sanitized SQLite JSON plus order ids, status, filled notional, exchange, environment, and symbol; API keys, secrets, API passphrases, and vault passphrases are never included.
- `GET /api/live-reconciliations` returns only latest sanitized reconciliation records and never decrypts a credential.
- Reconciliation approved, rejected, and failed outcomes are written to the hash-chained audit log without secrets.
- `/api/account-sync` is read-only, requires a decryptable vault credential and passphrase on every request, and supports only `testnet` or `demo`.
- Binance account sync is restricted to signed Spot Testnet/Demo account and open-order reads.
- OKX account sync is restricted to signed Demo Trading balance and pending-order reads with `x-simulated-trading: 1`.
- Production/mainnet account sync is rejected.
- Successful account sync snapshots are persisted locally as sanitized SQLite JSON plus counts and metadata; API keys, secrets, API passphrases, and vault passphrases are never included.
- `GET /api/account-sync` returns only the latest sanitized persisted snapshot and never decrypts a credential.
- Account sync approved, rejected, and failed outcomes are written to the hash-chained audit log without secrets.
- `GET /api/export` returns a no-store workspace JSON package for local backup/review: Strategy Profile, Risk Profile, sanitized Autopilot state/runs/steps, backtest run history, paper account snapshot, paper execution records, live execution ledger, reconciliation records, audit verification, and recent audit entries.
- Workspace export deliberately excludes the credential table, API keys, API secrets, API passphrases, vault passphrases, credential ciphertext, salts, and nonces.
- `GET /api/local-data` reports local SQLite table counts without exposing record payloads.
- `POST /api/local-data/prune` requires the exact `PRUNE LOCAL DATA` phrase, rejects while Autopilot is running, and writes approved or rejected attempts into the hash-chained audit log.
- Local data prune deletes only older backtest runs, Autopilot runs/steps, paper execution records, and account snapshots beyond configured retention counts.
- Local data prune never deletes credentials, audit entries, live execution ledger records, live reconciliation records, Risk Profile, or Strategy Profile.
- Release verification may route private Binance/OKX requests only to explicit loopback mock URLs when `CCVAR_ENABLE_LOOPBACK_EXCHANGE_MOCKS=true`; remote hosts, URL paths, query strings, fragments, and userinfo are rejected at startup.
- The release verifier exercises both Binance validation-only `/api/v3/order/test` and OKX demo signed-preflight with fake loopback credentials, then verifies sanitized acceptance reports without secrets or encrypted credential material.
- AI output is represented as a trade intent before it becomes an exchange request.
- The risk engine must approve any executable intent before a simulator, Autopilot loop, or guarded private exchange executor can act on it.
- The simulator refuses risk-rejected intents.
- The UI's Live mode is a controlled visual state; Live Guard unlock and `/api/live-execute` are both required for any testnet/demo execution attempt.
- The UI Account Sync control is separate from order execution and remains disabled until a saved credential and vault passphrase are present.
- The UI disables AI Execute until Live Guard is unlocked, a trade credential is selected, and a recent account snapshot is loaded.
- The UI Live Setup checklist summarizes Vault, passphrase, Guard, Account Sync, preflight, and Execute readiness from local sanitized state only; it never renders credential secrets or passphrases.
- The UI Autopilot control starts/stops the server-side Autopilot, surfaces sanitized step state, and disables Live Autopilot until Live Guard, credential id, and vault passphrase are present.
- The UI `STOP ALL` control calls the server-side Kill Switch; the local `STOP RUN` control remains simulation-only and cannot clear a server-side halt.

## Future Live Trading Requirements

Before live trading can be enabled:

- Prefer OS keychain integration over passphrase-derived SQLite vault storage.
- Require API keys with trading permission only; withdrawal permission must stay disabled.
- Add user-controlled exchange/account allowlists.
- Promote Live Guard from testnet/demo unlock to production only after exchange testnet/demo coverage and user allowlists are complete.
- Extend reconciliation from submitted exchange orders to account snapshots and local simulator state.
- Add retention and archival policies for persisted Autopilot run/step history and workspace export packages.
- Route every order through the risk engine.
- Promote audit and execution ledger retention policies for long-running production workspaces.
- Add explicit production deployment gates, allowlists, and operator approvals before production credentials.

## Risk Gates

Minimum live gates:

- Persisted Risk Profile approval.
- Maximum single order notional.
- Maximum symbol exposure.
- Maximum total exposure.
- Maximum daily drawdown.
- Maximum consecutive losses.
- Maximum spread and slippage.
- TTL expiration for AI trade intents.
- Kill switch that cancels pending automation.
