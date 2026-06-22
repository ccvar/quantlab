package liveexec

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/liveguard"
	"ccvar.com/web3quant/internal/risk"
	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

func TestExecuteRejectsLockedGuardBeforeCredentialUse(t *testing.T) {
	store := openTestStore(t)
	guard := liveguard.New()
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{})

	_, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		ValidationOnly: true,
	})
	if !errors.Is(err, ErrLiveGuardLocked) {
		t.Fatalf("Execute() err = %v, want ErrLiveGuardLocked", err)
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 1 {
		t.Fatalf("verification = %#v", verification)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if entries[0].Action != "live_execute.rejected" || entries[0].Status != "rejected" {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestExecuteRejectsKillSwitchBeforeGuardAndCredentialUse(t *testing.T) {
	store := openTestStore(t)
	guard := liveguard.New()
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{})
	service.Halted = func() bool { return true }

	_, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		ValidationOnly: true,
	})
	if !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("Execute() err = %v, want ErrKillSwitchActive", err)
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 1 {
		t.Fatalf("verification = %#v", verification)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if entries[0].Action != "live_execute.kill_switch" || entries[0].Status != "rejected" {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestExecuteRejectsOversizedOrderWithoutHTTPCall(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 50,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	executor := countingExecutor{}
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{"Binance": &executor})
	service.Now = fixedNow
	saveFreshAccountSnapshot(t, store, meta.ID, 20000)

	result, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		CredentialID:   meta.ID,
		Passphrase:     "correct horse battery",
		ValidationOnly: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Decision.Approved {
		t.Fatal("expected risk rejection")
	}
	if result.LedgerID == 0 {
		t.Fatalf("LedgerID = %d, want non-zero", result.LedgerID)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
	if len(result.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(result.Events))
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Action != "live_execute.risk" || entries[0].Status != "rejected" {
		t.Fatalf("entries = %#v", entries)
	}
	records, err := store.ListLiveExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListLiveExecutions() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != result.LedgerID || records[0].RiskStatus != "rejected" || records[0].ExecutionStatus != "not_submitted" {
		t.Fatalf("records = %#v", records)
	}
}

func TestExecuteUsesRiskProviderLimits(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 1000,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	executor := countingExecutor{}
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{"Binance": &executor})
	service.Now = fixedNow
	service.RiskProvider = func(context.Context) (risk.Limits, error) {
		return risk.Limits{
			MinConfidence:        0.1,
			MaxOrderUSDT:         50,
			MaxSymbolExposure:    8000,
			MaxTotalExposure:     12000,
			MaxDailyDrawdownPct:  3,
			MaxConsecutiveLosses: 3,
			MaxSpreadPct:         0.08,
			RequireLiveUnlock:    true,
		}, nil
	}
	saveFreshAccountSnapshot(t, store, meta.ID, 20000)

	result, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		CredentialID:   meta.ID,
		Passphrase:     "correct horse battery",
		ValidationOnly: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Decision.Approved || result.Decision.ReasonText() != "order size 100.00 exceeds 50.00" {
		t.Fatalf("decision = %#v", result.Decision)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
	if result.LedgerID == 0 {
		t.Fatalf("LedgerID = %d, want non-zero", result.LedgerID)
	}
}

func TestExecuteRequiresRecentAccountSnapshot(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 1000,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	executor := countingExecutor{}
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{"Binance": &executor})
	service.Now = fixedNow

	result, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		CredentialID:   meta.ID,
		Passphrase:     "correct horse battery",
		ValidationOnly: true,
	})
	if !errors.Is(err, ErrAccountSnapshotRequired) {
		t.Fatalf("Execute() err = %v, want ErrAccountSnapshotRequired", err)
	}
	if result.Intent.ID == "" {
		t.Fatalf("expected generated intent in result: %#v", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
	records, err := store.ListLiveExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListLiveExecutions() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %#v, want none before risk", records)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Action != "live_execute.account" || entries[0].Status != "rejected" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestExecuteRejectsStaleAccountSnapshot(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 1000,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	saveAccountSnapshot(t, store, meta.ID, fixedNow().Add(-10*time.Minute), 20000, nil)
	executor := countingExecutor{}
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{"Binance": &executor})
	service.Now = fixedNow

	_, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		CredentialID:   meta.ID,
		Passphrase:     "correct horse battery",
		ValidationOnly: true,
	})
	if !errors.Is(err, ErrAccountSnapshotStale) {
		t.Fatalf("Execute() err = %v, want ErrAccountSnapshotStale", err)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestExecuteUsesAccountSnapshotBalanceForRisk(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 1000,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	saveFreshAccountSnapshot(t, store, meta.ID, 25)
	executor := countingExecutor{}
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{"Binance": &executor})
	service.Now = fixedNow

	result, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		CredentialID:   meta.ID,
		Passphrase:     "correct horse battery",
		ValidationOnly: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Decision.Approved || result.Decision.ReasonText() != "insufficient available balance" {
		t.Fatalf("decision = %#v", result.Decision)
	}
	if result.LedgerID == 0 {
		t.Fatalf("LedgerID = %d, want non-zero", result.LedgerID)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
	records, err := store.ListLiveExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListLiveExecutions() error = %v", err)
	}
	if len(records) != 1 || records[0].RiskStatus != "rejected" || records[0].ExecutionStatus != "not_submitted" {
		t.Fatalf("records = %#v", records)
	}
}

func TestExecuteAuditsMissingCredentialWhenGuardUnlocked(t *testing.T) {
	store := openTestStore(t)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 1000,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{})

	_, err := service.Execute(context.Background(), Request{
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		ValidationOnly: true,
	})
	if !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("Execute() err = %v, want ErrCredentialRequired", err)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "live_execute.credential" || entries[0].Status != "rejected" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestExecuteBinanceValidationCallsSignedEndpointAndAudits(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	guard := liveguard.New()
	if _, err := guard.Unlock(liveguard.UnlockRequest{
		Operator:     "qa",
		Environment:  "testnet",
		Phrase:       liveguard.UnlockPhrase,
		TTLSeconds:   120,
		MaxOrderUSDT: 1000,
		Reason:       "unit test",
	}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v3/order/test" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-MBX-APIKEY"); got != "BINANCEKEY123456" {
			t.Errorf("X-MBX-APIKEY = %q", got)
		}
		query := r.URL.Query()
		if query.Get("symbol") != "BTCUSDT" || query.Get("side") != "BUY" || query.Get("type") != "MARKET" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if query.Get("quoteOrderQty") != "100" {
			t.Errorf("quoteOrderQty = %q", query.Get("quoteOrderQty"))
		}
		if query.Get("signature") == "" || query.Get("timestamp") == "" {
			t.Errorf("missing signature/timestamp in %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	service := New(store, exchange.NewRegistry(fakeMarketAdapter{}), guard, map[string]Executor{
		"Binance": BinanceExecutor{BaseURL: server.URL, Client: server.Client()},
	})
	service.Now = fixedNow
	saveFreshAccountSnapshot(t, store, meta.ID, 20000)

	result, err := service.Execute(context.Background(), Request{
		Operator:       "qa",
		Exchange:       "Binance",
		Symbol:         "BTCUSDT",
		SizeUSDT:       100,
		CredentialID:   meta.ID,
		Passphrase:     "correct horse battery",
		ValidationOnly: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected Binance endpoint to be called")
	}
	if !result.Decision.Approved || result.Execution == nil || result.Execution.Status != "validated" {
		t.Fatalf("result = %#v", result)
	}
	if result.LedgerID == 0 {
		t.Fatalf("LedgerID = %d, want non-zero", result.LedgerID)
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 3 {
		t.Fatalf("verification = %#v", verification)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if entries[0].Action != "live_execute.order" || entries[0].Status != "validated" {
		t.Fatalf("latest audit entry = %#v", entries[0])
	}
	records, err := store.ListLiveExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListLiveExecutions() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != result.LedgerID || records[0].RiskStatus != "approved" || records[0].ExecutionStatus != "validated" {
		t.Fatalf("records = %#v", records)
	}
	if len(records[0].IntentJSON) == 0 || len(records[0].DecisionJSON) == 0 || len(records[0].ExecutionJSON) == 0 {
		t.Fatalf("record json fields missing: %#v", records[0])
	}
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "ccvar-liveexec-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func saveBinanceCredential(t *testing.T, store *storage.Store) vault.CredentialMeta {
	t.Helper()
	encrypted, err := vault.EncryptCredential(vault.CredentialInput{
		Exchange:   "Binance",
		Label:      "qa binance",
		APIKey:     "BINANCEKEY123456",
		Secret:     "BINANCESECRET789",
		Passphrase: "correct horse battery",
		Permissions: vault.Permissions{
			Trade: true,
		},
	}, time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EncryptCredential() error = %v", err)
	}
	meta, err := store.SaveCredential(context.Background(), encrypted)
	if err != nil {
		t.Fatalf("SaveCredential() error = %v", err)
	}
	return meta
}

func saveFreshAccountSnapshot(t *testing.T, store *storage.Store, credentialID int64, availableUSDT float64) storage.AccountSnapshotRecord {
	t.Helper()
	return saveAccountSnapshot(t, store, credentialID, fixedNow().Add(-time.Minute), availableUSDT, nil)
}

func saveAccountSnapshot(t *testing.T, store *storage.Store, credentialID int64, syncedAt time.Time, availableUSDT float64, orders []accountOpenOrder) storage.AccountSnapshotRecord {
	t.Helper()
	balances := []accountBalance{
		{
			Asset: "USDT",
			Free:  availableUSDT,
			Total: availableUSDT,
			USD:   availableUSDT,
		},
	}
	snapshot := accountSnapshot{
		Exchange:    "Binance",
		Environment: "testnet",
		AccountType: "SPOT",
		CanTrade:    true,
		Balances:    balances,
		OpenOrders:  orders,
		SyncedAt:    syncedAt.UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal(snapshot) error = %v", err)
	}
	record, err := store.SaveAccountSnapshot(context.Background(), storage.AccountSnapshotRecord{
		CredentialID:   credentialID,
		Exchange:       "Binance",
		Environment:    "testnet",
		Symbol:         "BTCUSDT",
		BalanceCount:   len(balances),
		OpenOrderCount: len(orders),
		SnapshotJSON:   payload,
		CreatedAt:      syncedAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("SaveAccountSnapshot() error = %v", err)
	}
	return record
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC)
}

type fakeMarketAdapter struct{}

func (fakeMarketAdapter) Name() string { return "Binance" }

func (fakeMarketAdapter) FetchSnapshot(context.Context, string) (core.MarketSnapshot, error) {
	return core.MarketSnapshot{
		Exchange:      "Binance",
		Symbol:        "BTCUSDT",
		BestBid:       67000,
		BestAsk:       67001,
		Last:          67000.5,
		SpreadPct:     0.0015,
		LiquidityUSDT: 2000000,
		ObservedAt:    fixedNow(),
	}, nil
}

func (fakeMarketAdapter) FetchCandles(context.Context, string, string, int) ([]core.Candle, error) {
	return nil, nil
}

func (fakeMarketAdapter) PlaceOrder(context.Context, core.OrderRequest) (core.Fill, error) {
	return core.Fill{}, exchange.ErrTradingDisabled
}

func (fakeMarketAdapter) CancelOrder(context.Context, string) error {
	return exchange.ErrTradingDisabled
}

type countingExecutor struct {
	calls int
}

func (executor *countingExecutor) Execute(context.Context, ExecuteRequest) (ExecutionReport, error) {
	executor.calls++
	return ExecutionReport{Status: "called"}, nil
}
