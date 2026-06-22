package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/autopilot"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/killswitch"
	"ccvar.com/web3quant/internal/liveguard"
	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

func TestLoadAppConfigUsesEnvAndFlags(t *testing.T) {
	t.Setenv("CCVAR_ADDR", "127.0.0.1:9999")
	t.Setenv("CCVAR_DB_PATH", filepath.Join("tmp", "env.db"))
	t.Setenv("CCVAR_OPEN_BROWSER", "true")

	config, err := loadAppConfig([]string{
		"--addr", "127.0.0.1:8888",
		"--db", filepath.Join("tmp", "flag.db"),
		"--open=false",
	})
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}
	if config.Addr != "127.0.0.1:8888" {
		t.Fatalf("Addr = %q", config.Addr)
	}
	if config.DBPath != filepath.Join("tmp", "flag.db") {
		t.Fatalf("DBPath = %q", config.DBPath)
	}
	if config.OpenBrowser {
		t.Fatalf("OpenBrowser = true, want false")
	}
}

func TestWithCORSAllowsOnlyLocalOrigins(t *testing.T) {
	handler := withCORS(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	})

	localRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	localRequest.Header.Set("Origin", "http://127.0.0.1:5173")
	localRecorder := httptest.NewRecorder()
	handler(localRecorder, localRequest)
	if localRecorder.Code != http.StatusOK {
		t.Fatalf("local status = %d", localRecorder.Code)
	}
	if got := localRecorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow origin = %q", got)
	}

	remoteRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	remoteRequest.Header.Set("Origin", "https://example.com")
	remoteRecorder := httptest.NewRecorder()
	handler(remoteRecorder, remoteRequest)
	if remoteRecorder.Code != http.StatusForbidden {
		t.Fatalf("remote status = %d, want %d", remoteRecorder.Code, http.StatusForbidden)
	}
	if got := remoteRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("remote allow origin = %q, want empty", got)
	}
}

func TestLoadAppConfigVersionFlag(t *testing.T) {
	t.Setenv(loopbackExchangeMocksEnv, "true")
	t.Setenv(binancePrivateMockURLEnv, "https://api.binance.com")

	config, err := loadAppConfig([]string{"--version"})
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}
	if !config.ShowVersion {
		t.Fatalf("ShowVersion = false, want true")
	}
}

func TestLoadPrivateExchangeMockConfigAcceptsOnlyExplicitLoopbackURLs(t *testing.T) {
	t.Setenv(loopbackExchangeMocksEnv, "true")
	t.Setenv(binancePrivateMockURLEnv, "http://127.0.0.1:8791/")
	t.Setenv(okxPrivateMockURLEnv, "http://[::1]:8792")

	config, err := loadPrivateExchangeMockConfig()
	if err != nil {
		t.Fatalf("loadPrivateExchangeMockConfig() error = %v", err)
	}
	if !config.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if config.BinanceBaseURL != "http://127.0.0.1:8791" {
		t.Fatalf("BinanceBaseURL = %q", config.BinanceBaseURL)
	}
	if config.OKXBaseURL != "http://[::1]:8792" {
		t.Fatalf("OKXBaseURL = %q", config.OKXBaseURL)
	}
}

func TestLoadPrivateExchangeMockConfigRejectsURLsWhenDisabled(t *testing.T) {
	t.Setenv(binancePrivateMockURLEnv, "http://127.0.0.1:8791")

	if _, err := loadPrivateExchangeMockConfig(); err == nil {
		t.Fatalf("loadPrivateExchangeMockConfig() error = nil, want error")
	}
}

func TestLoadPrivateExchangeMockConfigRejectsRemoteAndPathURLs(t *testing.T) {
	tests := map[string]string{
		"https://api.binance.com":    "remote",
		"http://192.168.1.50:8791":   "lan",
		"http://127.0.0.1:8791/mock": "path",
		"http://user@127.0.0.1:8791": "userinfo",
		"file:///tmp/mock-exchange":  "scheme",
		"http://127.0.0.1.evil.test": "suffix",
		"http://127.0.0.1:8791/?x=1": "query",
	}
	for rawURL, name := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv(loopbackExchangeMocksEnv, "true")
			t.Setenv(binancePrivateMockURLEnv, rawURL)
			if _, err := loadPrivateExchangeMockConfig(); err == nil {
				t.Fatalf("loadPrivateExchangeMockConfig(%q) error = nil, want error", rawURL)
			}
		})
	}
}

func TestLocalOriginGuard(t *testing.T) {
	allowed := []string{
		"http://127.0.0.1:8787",
		"http://localhost:5173",
		"https://localhost",
		"http://[::1]:8787",
	}
	for _, origin := range allowed {
		if !isAllowedLocalOrigin(origin) {
			t.Fatalf("isAllowedLocalOrigin(%q) = false, want true", origin)
		}
	}

	blocked := []string{
		"",
		"null",
		"file://local/index.html",
		"https://example.com",
		"http://192.168.1.9:8787",
		"http://127.0.0.1.evil.test",
	}
	for _, origin := range blocked {
		if isAllowedLocalOrigin(origin) {
			t.Fatalf("isAllowedLocalOrigin(%q) = true, want false", origin)
		}
	}
}

func TestBrowserAddressUsesLoopbackForWildcardBinds(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0:8787":   "127.0.0.1:8787",
		":8787":          "127.0.0.1:8787",
		"127.0.0.1:8787": "127.0.0.1:8787",
		"[::1]:8787":     "[::1]:8787",
	}
	for input, want := range tests {
		if got := browserAddress(input); got != want {
			t.Fatalf("browserAddress(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildAppInfoReportsDatabaseAndSafety(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ccvar.db")
	if err := os.WriteFile(dbPath, []byte("sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	startedAt := time.Date(2026, 6, 22, 8, 30, 0, 0, time.UTC)

	info := buildAppInfo(appConfig{
		Addr:   "127.0.0.1:8787",
		DBPath: dbPath,
	}, startedAt)

	if info.Service != "ccvar-quant" || info.Version != serviceVersion {
		t.Fatalf("service/version = %q/%q", info.Service, info.Version)
	}
	if info.URL != "http://127.0.0.1:8787" {
		t.Fatalf("URL = %q", info.URL)
	}
	if !info.Database.Exists || info.Database.SizeBytes != 6 {
		t.Fatalf("database info = %+v", info.Database)
	}
	if info.Docs.Runbook.Path == "" || info.Docs.Safety.Path == "" {
		t.Fatalf("docs info = %+v", info.Docs)
	}
	if info.Runtime.GOOS != runtime.GOOS || info.Runtime.GOARCH != runtime.GOARCH {
		t.Fatalf("runtime info = %+v", info.Runtime)
	}
	if !info.Security.LocalOriginOnly || info.Security.ProductionTradingEnabled || info.Security.ProductionAccountSyncEnabled {
		t.Fatalf("security info = %+v", info.Security)
	}
	if len(info.Exchanges) != 2 {
		t.Fatalf("exchanges = %+v", info.Exchanges)
	}
}

func TestBuildAppDocsInfoFindsMacAppResourceDocs(t *testing.T) {
	dir := t.TempDir()
	executablePath := filepath.Join(dir, "CCVar Quant Lab.app", "Contents", "MacOS", "ccvar-quant")
	docsDir := filepath.Join(dir, "CCVar Quant Lab.app", "Contents", "Resources", "docs")
	if err := os.MkdirAll(filepath.Dir(executablePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(executable) error = %v", err)
	}
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(docs) error = %v", err)
	}
	runbookPath := filepath.Join(docsDir, "operator-runbook.zh-CN.md")
	safetyPath := filepath.Join(docsDir, "safety.md")
	if err := os.WriteFile(runbookPath, []byte("runbook"), 0o644); err != nil {
		t.Fatalf("WriteFile(runbook) error = %v", err)
	}
	if err := os.WriteFile(safetyPath, []byte("safety"), 0o644); err != nil {
		t.Fatalf("WriteFile(safety) error = %v", err)
	}

	info := buildAppDocsInfoFor(executablePath, filepath.Join(dir, "empty"))
	if !info.Available {
		t.Fatalf("Available = false, info = %+v", info)
	}
	if info.Runbook.Path != runbookPath || info.Runbook.SizeBytes != int64(len("runbook")) {
		t.Fatalf("runbook info = %+v, want path %q", info.Runbook, runbookPath)
	}
	if info.Safety.Path != safetyPath || info.Safety.SizeBytes != int64(len("safety")) {
		t.Fatalf("safety info = %+v, want path %q", info.Safety, safetyPath)
	}
}

func TestBuildAppDocsInfoFindsPortableDocsAndWorkingDirFallback(t *testing.T) {
	dir := t.TempDir()
	executableDir := filepath.Join(dir, "portable")
	portableDocs := filepath.Join(executableDir, "docs")
	if err := os.MkdirAll(portableDocs, 0o755); err != nil {
		t.Fatalf("MkdirAll(portableDocs) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(portableDocs, "operator-runbook.zh-CN.md"), []byte("portable runbook"), 0o644); err != nil {
		t.Fatalf("WriteFile(portable runbook) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(portableDocs, "safety.md"), []byte("portable safety"), 0o644); err != nil {
		t.Fatalf("WriteFile(portable safety) error = %v", err)
	}

	portable := buildAppDocsInfoFor(filepath.Join(executableDir, "ccvar-quant.exe"), filepath.Join(dir, "empty"))
	if !portable.Available || portable.Runbook.SizeBytes != int64(len("portable runbook")) {
		t.Fatalf("portable docs info = %+v", portable)
	}

	workingDir := filepath.Join(dir, "workspace")
	workingDocs := filepath.Join(workingDir, "docs")
	if err := os.MkdirAll(workingDocs, 0o755); err != nil {
		t.Fatalf("MkdirAll(workingDocs) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDocs, "operator-runbook.zh-CN.md"), []byte("workspace runbook"), 0o644); err != nil {
		t.Fatalf("WriteFile(workspace runbook) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDocs, "safety.md"), []byte("workspace safety"), 0o644); err != nil {
		t.Fatalf("WriteFile(workspace safety) error = %v", err)
	}
	fallback := buildAppDocsInfoFor(filepath.Join(dir, "missing", "ccvar-quant"), workingDir)
	if !fallback.Available || fallback.Runbook.SizeBytes != int64(len("workspace runbook")) {
		t.Fatalf("fallback docs info = %+v", fallback)
	}
}

func TestExistingClientHealthyRecognizesOnlyCCVarHealth(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{"ok": true, "service": "ccvar-quant"})
	}))
	defer okServer.Close()
	if !existingClientHealthy(okServer.URL) {
		t.Fatalf("existingClientHealthy() = false, want true")
	}

	wrongServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "service": "other-service"})
	}))
	defer wrongServer.Close()
	if existingClientHealthy(wrongServer.URL) {
		t.Fatalf("wrong service recognized as existing client")
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer badServer.Close()
	if existingClientHealthy(badServer.URL) {
		t.Fatalf("invalid health response recognized as existing client")
	}
}

func TestBuildPreflightReportWarnsWithoutCredentials(t *testing.T) {
	store, dbPath := openTestStore(t)
	defer store.Close()

	report, err := buildPreflightReport(context.Background(), preflightDeps{
		Config: appConfig{
			Addr:   "127.0.0.1:8787",
			DBPath: dbPath,
		},
		Store:           store,
		Registry:        exchange.NewRegistry(preflightAdapter{name: "Binance"}, preflightAdapter{name: "OKX"}),
		GuardState:      liveguard.New().State(),
		KillSwitchState: killswitch.New().State(),
		AutopilotState:  autopilot.New(nil, nil, nil, nil, nil).State(),
	})
	if err != nil {
		t.Fatalf("buildPreflightReport() error = %v", err)
	}
	if report.Overall != preflightStatusWarn {
		t.Fatalf("overall = %q, want warn; report = %+v", report.Overall, report)
	}
	assertPreflightStatus(t, report, "audit", preflightStatusReady)
	assertPreflightStatus(t, report, "database", preflightStatusReady)
	assertPreflightStatus(t, report, "vault", preflightStatusWarn)
	assertPreflightStatus(t, report, "live_autopilot", preflightStatusWarn)
	assertPreflightStatus(t, report, "market_binance", preflightStatusReady)
	assertPreflightStatus(t, report, "market_okx", preflightStatusReady)
}

func TestBuildPreflightReportBlocksOnKillSwitchAndMissingAdapter(t *testing.T) {
	store, dbPath := openTestStore(t)
	defer store.Close()
	killSwitch := killswitch.New()
	killState := killSwitch.Activate(killswitch.Request{Operator: "qa", Reason: "test halt"})

	report, err := buildPreflightReport(context.Background(), preflightDeps{
		Config: appConfig{
			Addr:   "127.0.0.1:8787",
			DBPath: dbPath,
		},
		Store:           store,
		Registry:        exchange.NewRegistry(preflightAdapter{name: "Binance", err: errors.New("offline")}),
		GuardState:      liveguard.New().State(),
		KillSwitchState: killState,
		AutopilotState:  autopilot.New(nil, nil, nil, nil, nil).State(),
	})
	if err != nil {
		t.Fatalf("buildPreflightReport() error = %v", err)
	}
	if report.Overall != preflightStatusBlock {
		t.Fatalf("overall = %q, want block; report = %+v", report.Overall, report)
	}
	assertPreflightStatus(t, report, "kill_switch", preflightStatusBlock)
	assertPreflightStatus(t, report, "live_autopilot", preflightStatusBlock)
	assertPreflightStatus(t, report, "market_binance", preflightStatusWarn)
	assertPreflightStatus(t, report, "market_okx", preflightStatusBlock)
}

func TestBuildPreflightReportLiveAutopilotReady(t *testing.T) {
	store, dbPath := openTestStore(t)
	defer store.Close()
	now := time.Now().UTC()
	encrypted, err := vault.EncryptCredential(vault.CredentialInput{
		Exchange:   "Binance",
		Label:      "QA testnet",
		APIKey:     "test-key",
		Secret:     "test-secret",
		Passphrase: "correct horse battery",
		Permissions: vault.Permissions{
			Read:  true,
			Trade: true,
		},
	}, now)
	if err != nil {
		t.Fatalf("EncryptCredential() error = %v", err)
	}
	credential, err := store.SaveCredential(context.Background(), encrypted)
	if err != nil {
		t.Fatalf("SaveCredential() error = %v", err)
	}
	_, err = store.SaveAccountSnapshot(context.Background(), storage.AccountSnapshotRecord{
		CredentialID:   credential.ID,
		Exchange:       "Binance",
		Environment:    "testnet",
		Symbol:         "BTCUSDT",
		BalanceCount:   1,
		OpenOrderCount: 0,
		SnapshotJSON:   []byte(`{"environment":"testnet","balances":[{"asset":"USDT","free":1000,"total":1000}],"openOrders":[]}`),
		CreatedAt:      now.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("SaveAccountSnapshot() error = %v", err)
	}
	guard := liveguard.New()
	guardState, err := guard.Unlock(liveguard.UnlockRequest{
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   300,
		MaxOrderUSDT: 1000,
	})
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	report, err := buildPreflightReport(context.Background(), preflightDeps{
		Config: appConfig{
			Addr:   "127.0.0.1:8787",
			DBPath: dbPath,
		},
		Store:           store,
		Registry:        exchange.NewRegistry(preflightAdapter{name: "Binance"}, preflightAdapter{name: "OKX"}),
		GuardState:      guardState,
		KillSwitchState: killswitch.New().State(),
		AutopilotState:  autopilot.New(nil, nil, nil, nil, nil).State(),
	})
	if err != nil {
		t.Fatalf("buildPreflightReport() error = %v", err)
	}
	assertPreflightStatus(t, report, "vault", preflightStatusReady)
	assertPreflightStatus(t, report, "live_autopilot", preflightStatusReady)
}

func openTestStore(t *testing.T) (*storage.Store, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ccvar.db")
	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	return store, dbPath
}

func assertPreflightStatus(t *testing.T, report preflightReport, id, want string) {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			if check.Status != want {
				t.Fatalf("check %s status = %q, want %q", id, check.Status, want)
			}
			return
		}
	}
	t.Fatalf("check %s not found in %+v", id, report.Checks)
}

type preflightAdapter struct {
	name string
	err  error
}

func (adapter preflightAdapter) Name() string { return adapter.name }

func (adapter preflightAdapter) FetchSnapshot(context.Context, string) (core.MarketSnapshot, error) {
	if adapter.err != nil {
		return core.MarketSnapshot{}, adapter.err
	}
	return core.MarketSnapshot{
		Exchange:  adapter.name,
		Symbol:    "BTCUSDT",
		BestBid:   66999,
		BestAsk:   67001,
		Last:      67000,
		SpreadPct: 0.003,
	}, nil
}

func (adapter preflightAdapter) FetchCandles(context.Context, string, string, int) ([]core.Candle, error) {
	if adapter.err != nil {
		return nil, adapter.err
	}
	return []core.Candle{{Time: 1, Open: 66000, High: 67100, Low: 65900, Close: 67000, Volume: 100}}, nil
}

func (preflightAdapter) PlaceOrder(context.Context, core.OrderRequest) (core.Fill, error) {
	return core.Fill{}, exchange.ErrTradingDisabled
}

func (preflightAdapter) CancelOrder(context.Context, string) error {
	return exchange.ErrTradingDisabled
}
