package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/audit"
	"ccvar.com/web3quant/internal/vault"
)

func TestCredentialStorageRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	encrypted, err := vault.EncryptCredential(vault.CredentialInput{
		Exchange:   "Binance",
		Label:      "main desk",
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
	if meta.ID == 0 {
		t.Fatal("credential ID was not assigned")
	}
	if meta.APIKeyMask != "BINA...3456" {
		t.Fatalf("APIKeyMask = %q", meta.APIKeyMask)
	}

	list, err := store.ListCredentials(context.Background())
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].APIKeyMask != meta.APIKeyMask || !list[0].Permissions.Read || !list[0].Permissions.Trade {
		t.Fatalf("list[0] = %#v", list[0])
	}

	stored, err := store.GetEncryptedCredential(context.Background(), meta.ID)
	if err != nil {
		t.Fatalf("GetEncryptedCredential() error = %v", err)
	}
	plain, err := vault.DecryptCredential(stored, "correct horse battery")
	if err != nil {
		t.Fatalf("DecryptCredential() error = %v", err)
	}
	if plain.APIKey != "BINANCEKEY123456" || plain.Secret != "BINANCESECRET789" {
		t.Fatalf("plain = %#v", plain)
	}

	if err := store.DeleteCredential(context.Background(), meta.ID); err != nil {
		t.Fatalf("DeleteCredential() error = %v", err)
	}
	list, err = store.ListCredentials(context.Background())
	if err != nil {
		t.Fatalf("ListCredentials() after delete error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("len(list) after delete = %d, want 0", len(list))
	}
}

func TestAuditLogAppendAndVerify(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.AppendAudit(context.Background(), audit.Record{
		Actor:   "local",
		Action:  "live.unlock",
		Entity:  "live_guard",
		Status:  "approved",
		Summary: "testnet unlock",
		Payload: map[string]any{"environment": "testnet"},
	})
	if err != nil {
		t.Fatalf("AppendAudit() first error = %v", err)
	}
	second, err := store.AppendAudit(context.Background(), audit.Record{
		Actor:   "local",
		Action:  "live.lock",
		Entity:  "live_guard",
		Status:  "approved",
		Summary: "manual lock",
	})
	if err != nil {
		t.Fatalf("AppendAudit() second error = %v", err)
	}
	if second.PrevHash != first.Hash {
		t.Fatalf("PrevHash = %q, want %q", second.PrevHash, first.Hash)
	}

	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Action != "live.lock" || entries[1].Action != "live.unlock" {
		t.Fatalf("entries = %#v", entries)
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 2 {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestAuditLogVerifyDetectsTamper(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if _, err := store.AppendAudit(context.Background(), audit.Record{Action: "live.unlock", Entity: "live_guard", Status: "approved"}); err != nil {
		t.Fatalf("AppendAudit() error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `UPDATE audit_log SET summary = 'tampered' WHERE id = 1`); err != nil {
		t.Fatalf("tamper update error = %v", err)
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if verification.Valid || verification.Error == "" {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestAccountSnapshotRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.SaveAccountSnapshot(context.Background(), AccountSnapshotRecord{
		CredentialID:   7,
		Exchange:       "Binance",
		Environment:    "testnet",
		Symbol:         "BTCUSDT",
		BalanceCount:   1,
		OpenOrderCount: 0,
		SnapshotJSON:   []byte(`{"exchange":"Binance","balances":[{"asset":"USDT","total":10}]}`),
		CreatedAt:      "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("SaveAccountSnapshot() first error = %v", err)
	}
	second, err := store.SaveAccountSnapshot(context.Background(), AccountSnapshotRecord{
		CredentialID:   7,
		Exchange:       "Binance",
		Environment:    "testnet",
		Symbol:         "BTCUSDT",
		BalanceCount:   2,
		OpenOrderCount: 1,
		SnapshotJSON:   []byte(`{"exchange":"Binance","balances":[{"asset":"BTC","total":1},{"asset":"USDT","total":20}],"openOrders":[{"orderId":"1"}]}`),
		CreatedAt:      "2026-06-22T10:31:00Z",
	})
	if err != nil {
		t.Fatalf("SaveAccountSnapshot() second error = %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("snapshot IDs = first %d second %d", first.ID, second.ID)
	}

	latest, err := store.LatestAccountSnapshot(context.Background(), AccountSnapshotFilter{
		CredentialID: 7,
		Exchange:     "Binance",
		Environment:  "testnet",
		Symbol:       "BTCUSDT",
	})
	if err != nil {
		t.Fatalf("LatestAccountSnapshot() error = %v", err)
	}
	if latest.ID != second.ID || latest.BalanceCount != 2 || latest.OpenOrderCount != 1 {
		t.Fatalf("latest = %#v", latest)
	}

	_, err = store.LatestAccountSnapshot(context.Background(), AccountSnapshotFilter{
		CredentialID: 7,
		Exchange:     "OKX",
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("LatestAccountSnapshot() err = %v, want sql.ErrNoRows", err)
	}
}

func TestAccountSnapshotRejectsInvalidJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.SaveAccountSnapshot(context.Background(), AccountSnapshotRecord{
		CredentialID: 1,
		Exchange:     "Binance",
		Environment:  "testnet",
		Symbol:       "BTCUSDT",
		SnapshotJSON: []byte(`not-json`),
	})
	if !errors.Is(err, ErrAccountSnapshotRequired) {
		t.Fatalf("SaveAccountSnapshot() err = %v, want ErrAccountSnapshotRequired", err)
	}
}

func TestLiveExecutionRecordRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.SaveLiveExecution(context.Background(), LiveExecutionRecord{
		IntentID:        "CCVAR1",
		ClientOrderID:   "CCVAR1",
		CredentialID:    3,
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		Side:            "buy",
		SizeUSDT:        100,
		ValidationOnly:  true,
		RiskStatus:      "approved",
		ExecutionStatus: "validated",
		IntentJSON:      []byte(`{"id":"CCVAR1","symbol":"BTCUSDT"}`),
		DecisionJSON:    []byte(`{"approved":true,"reasons":[]}`),
		ExecutionJSON:   []byte(`{"status":"validated"}`),
		CreatedAt:       "2026-06-22T10:30:00Z",
		UpdatedAt:       "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("SaveLiveExecution() first error = %v", err)
	}
	second, err := store.SaveLiveExecution(context.Background(), LiveExecutionRecord{
		IntentID:        "CCVAR2",
		ClientOrderID:   "CCVAR2",
		CredentialID:    3,
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "ETHUSDT",
		Side:            "sell",
		SizeUSDT:        50,
		ValidationOnly:  true,
		RiskStatus:      "rejected",
		ExecutionStatus: "not_submitted",
		IntentJSON:      []byte(`{"id":"CCVAR2","symbol":"ETHUSDT"}`),
		DecisionJSON:    []byte(`{"approved":false,"reasons":["limit"]}`),
		CreatedAt:       "2026-06-22T10:31:00Z",
		UpdatedAt:       "2026-06-22T10:31:00Z",
	})
	if err != nil {
		t.Fatalf("SaveLiveExecution() second error = %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("execution IDs = first %d second %d", first.ID, second.ID)
	}

	records, err := store.ListLiveExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListLiveExecutions() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].IntentID != "CCVAR2" || records[0].ExecutionStatus != "not_submitted" || records[1].IntentID != "CCVAR1" {
		t.Fatalf("records = %#v", records)
	}

	loaded, err := store.GetLiveExecution(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("GetLiveExecution() error = %v", err)
	}
	if loaded.IntentID != "CCVAR1" || loaded.ExecutionStatus != "validated" || string(loaded.ExecutionJSON) == "" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestLiveExecutionRecordRejectsInvalidJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.SaveLiveExecution(context.Background(), LiveExecutionRecord{
		IntentID:        "CCVAR1",
		ClientOrderID:   "CCVAR1",
		ExecutionStatus: "validated",
		IntentJSON:      []byte(`not-json`),
		DecisionJSON:    []byte(`{"approved":true}`),
	})
	if !errors.Is(err, ErrLiveExecutionRequired) {
		t.Fatalf("SaveLiveExecution() err = %v, want ErrLiveExecutionRequired", err)
	}
}

func TestPaperExecutionRecordRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.SavePaperExecution(context.Background(), PaperExecutionRecord{
		RunID:        7,
		Mode:         "paper",
		Source:       "autopilot",
		IntentID:     "sim-1",
		Exchange:     "Binance",
		Symbol:       "BTCUSDT",
		Side:         "buy",
		SizeUSDT:     500,
		IntentPrice:  67000,
		Confidence:   0.78,
		RiskStatus:   "approved",
		FillStatus:   "filled",
		FillPrice:    67013.4,
		FeeUSDT:      0.25,
		SlippageUSDT: 0.1,
		IntentJSON:   []byte(`{"id":"sim-1","symbol":"BTCUSDT"}`),
		DecisionJSON: []byte(`{"approved":true}`),
		FillJSON:     []byte(`{"price":67013.4}`),
		EventsJSON:   []byte(`[{"type":"Sim Fill"}]`),
		CreatedAt:    "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("SavePaperExecution() first error = %v", err)
	}
	second, err := store.SavePaperExecution(context.Background(), PaperExecutionRecord{
		Mode:         "shadow",
		Source:       "manual",
		IntentID:     "sim-2",
		Exchange:     "OKX",
		Symbol:       "ETH-USDT",
		Side:         "sell",
		SizeUSDT:     250,
		IntentPrice:  3500,
		Confidence:   0.55,
		RiskStatus:   "rejected",
		FillStatus:   "not_filled",
		IntentJSON:   []byte(`{"id":"sim-2","symbol":"ETH-USDT"}`),
		DecisionJSON: []byte(`{"approved":false,"reasons":["confidence"]}`),
		EventsJSON:   []byte(`[{"type":"Risk Check"}]`),
		CreatedAt:    "2026-06-22T10:31:00Z",
	})
	if err != nil {
		t.Fatalf("SavePaperExecution() second error = %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("paper IDs = first %d second %d", first.ID, second.ID)
	}

	records, err := store.ListPaperExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPaperExecutions() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].IntentID != "sim-2" || records[0].RiskStatus != "rejected" || string(records[0].FillJSON) != "null" {
		t.Fatalf("records[0] = %#v fill=%s", records[0], string(records[0].FillJSON))
	}
	if records[1].RunID != 7 || records[1].Mode != "paper" || records[1].FillStatus != "filled" || records[1].FeeUSDT != 0.25 {
		t.Fatalf("records[1] = %#v", records[1])
	}
}

func TestPaperExecutionRecordRejectsInvalidJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.SavePaperExecution(context.Background(), PaperExecutionRecord{
		IntentID:     "sim-1",
		IntentJSON:   []byte(`not-json`),
		DecisionJSON: []byte(`{"approved":true}`),
		EventsJSON:   []byte(`[]`),
	})
	if !errors.Is(err, ErrPaperExecutionRequired) {
		t.Fatalf("SavePaperExecution() err = %v, want ErrPaperExecutionRequired", err)
	}
}

func TestResetPaperExecutionsDeletesRecordsAndAudits(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	for _, intentID := range []string{"sim-1", "sim-2"} {
		if _, err := store.SavePaperExecution(context.Background(), PaperExecutionRecord{
			Mode:         "paper",
			Source:       "manual",
			IntentID:     intentID,
			Exchange:     "Binance",
			Symbol:       "BTCUSDT",
			Side:         "buy",
			SizeUSDT:     500,
			IntentPrice:  67000,
			RiskStatus:   "approved",
			FillStatus:   "filled",
			FillPrice:    67010,
			IntentJSON:   []byte(`{"symbol":"BTCUSDT"}`),
			DecisionJSON: []byte(`{"approved":true}`),
			FillJSON:     []byte(`{"price":67010}`),
			EventsJSON:   []byte(`[{"type":"Sim Fill"}]`),
		}); err != nil {
			t.Fatalf("SavePaperExecution(%s) error = %v", intentID, err)
		}
	}

	deleted, entry, err := store.ResetPaperExecutions(context.Background(), audit.Record{
		Actor:   "qa",
		Action:  "paper.reset",
		Entity:  "paper_execution_records",
		Status:  "approved",
		Summary: "paper simulation ledger reset",
		Payload: map[string]any{"reason": "unit test"},
	})
	if err != nil {
		t.Fatalf("ResetPaperExecutions() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if entry.Action != "paper.reset" || entry.Entity != "paper_execution_records" {
		t.Fatalf("entry = %#v", entry)
	}
	if !strings.Contains(string(entry.Payload), `"deletedRecords":2`) {
		t.Fatalf("entry payload = %s", string(entry.Payload))
	}
	records, err := store.ListPaperExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPaperExecutions() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0", len(records))
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 1 {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestLiveReconciliationRecordRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.SaveLiveReconciliation(context.Background(), LiveReconciliationRecord{
		LiveExecutionID: 42,
		CredentialID:    3,
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		ClientOrderID:   "CCVAR1",
		ExchangeOrderID: "123456",
		Status:          "filled",
		FilledUSDT:      100.25,
		ReportJSON:      []byte(`{"status":"filled","filledUsdt":100.25}`),
		CreatedAt:       "2026-06-22T10:32:00Z",
	})
	if err != nil {
		t.Fatalf("SaveLiveReconciliation() first error = %v", err)
	}
	second, err := store.SaveLiveReconciliation(context.Background(), LiveReconciliationRecord{
		LiveExecutionID: 43,
		CredentialID:    3,
		Exchange:        "OKX",
		Environment:     "demo",
		Symbol:          "ETH-USDT",
		ClientOrderID:   "CCVAR2",
		Status:          "live",
		ReportJSON:      []byte(`{"status":"live"}`),
		CreatedAt:       "2026-06-22T10:33:00Z",
	})
	if err != nil {
		t.Fatalf("SaveLiveReconciliation() second error = %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("reconciliation IDs = first %d second %d", first.ID, second.ID)
	}

	records, err := store.ListLiveReconciliations(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("ListLiveReconciliations() error = %v", err)
	}
	if len(records) != 2 || records[0].LiveExecutionID != 43 || records[1].Status != "filled" {
		t.Fatalf("records = %#v", records)
	}
	filtered, err := store.ListLiveReconciliations(context.Background(), 42, 10)
	if err != nil {
		t.Fatalf("ListLiveReconciliations() filtered error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != first.ID || filtered[0].FilledUSDT != 100.25 {
		t.Fatalf("filtered = %#v", filtered)
	}
}

func TestLiveReconciliationRecordRejectsInvalidJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.SaveLiveReconciliation(context.Background(), LiveReconciliationRecord{
		LiveExecutionID: 42,
		Status:          "filled",
		ReportJSON:      []byte(`not-json`),
	})
	if !errors.Is(err, ErrLiveReconcileRequired) {
		t.Fatalf("SaveLiveReconciliation() err = %v, want ErrLiveReconcileRequired", err)
	}
}

func TestRiskProfileDefaultsAndRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	profile, err := store.RiskProfile(context.Background())
	if err != nil {
		t.Fatalf("RiskProfile() default error = %v", err)
	}
	if profile.Name != "Local Guardrails" || profile.MaxOrderUSDT != 1000 || !profile.RequireLiveUnlock {
		t.Fatalf("default profile = %#v", profile)
	}

	saved, err := store.SaveRiskProfile(context.Background(), RiskProfileRecord{
		Name:                  "Conservative",
		MinConfidence:         0.72,
		MaxOrderUSDT:          250,
		MaxSymbolExposureUSDT: 1500,
		MaxTotalExposureUSDT:  2500,
		MaxDailyDrawdownPct:   1.5,
		MaxConsecutiveLosses:  2,
		MaxSpreadPct:          0.04,
		RequireLiveUnlock:     false,
		UpdatedAt:             "2026-06-22T10:40:00Z",
	})
	if err != nil {
		t.Fatalf("SaveRiskProfile() error = %v", err)
	}
	if saved.ID != 1 || !saved.RequireLiveUnlock {
		t.Fatalf("saved profile = %#v", saved)
	}

	loaded, err := store.RiskProfile(context.Background())
	if err != nil {
		t.Fatalf("RiskProfile() loaded error = %v", err)
	}
	if loaded.Name != "Conservative" || loaded.MaxOrderUSDT != 250 || loaded.MaxConsecutiveLosses != 2 || loaded.UpdatedAt != "2026-06-22T10:40:00Z" {
		t.Fatalf("loaded profile = %#v", loaded)
	}
}

func TestStrategyProfileDefaultsAndRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	profile, err := store.StrategyProfile(context.Background())
	if err != nil {
		t.Fatalf("StrategyProfile() default error = %v", err)
	}
	if profile.Name != "AI Momentum Pro" || profile.Exchange != "Binance" || profile.Symbol != "BTCUSDT" || profile.ModelProfile != "local_policy" || profile.Concurrency != 2 || profile.OrderSizeUSDT != 500 {
		t.Fatalf("default profile = %#v", profile)
	}

	saved, err := store.SaveStrategyProfile(context.Background(), StrategyProfileRecord{
		Name:            "Mean Reversion",
		Exchange:        "OKX",
		Symbol:          "eth-usdt",
		Side:            "sell",
		ModelProfile:    "claude_cli",
		ModelFallback:   "local_policy",
		Concurrency:     12,
		OrderSizeUSDT:   250,
		IntervalSeconds: 1,
		MaxSteps:        4,
		UpdatedAt:       "2026-06-22T10:45:00Z",
	})
	if err != nil {
		t.Fatalf("SaveStrategyProfile() error = %v", err)
	}
	if saved.ID != 1 || saved.Symbol != "ETH-USDT" || saved.ModelProfile != "claude_cli" || saved.ModelFallback != "local_policy" || saved.Concurrency != 8 || saved.IntervalSeconds != 5 {
		t.Fatalf("saved profile = %#v", saved)
	}

	loaded, err := store.StrategyProfile(context.Background())
	if err != nil {
		t.Fatalf("StrategyProfile() loaded error = %v", err)
	}
	if loaded.Name != "Mean Reversion" || loaded.Exchange != "OKX" || loaded.Side != "sell" || loaded.ModelProfile != "claude_cli" || loaded.Concurrency != 8 || loaded.MaxSteps != 4 {
		t.Fatalf("loaded profile = %#v", loaded)
	}
}

func TestAutopilotRunAndStepRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	run, err := store.CreateAutopilotRun(context.Background(), AutopilotRunRecord{
		Mode:            "shadow",
		Exchange:        "Binance",
		Symbol:          "BTCUSDT",
		Operator:        "qa",
		IntervalSeconds: 15,
		MaxSteps:        2,
		Status:          "running",
		StartedAt:       "2026-06-22T10:30:00Z",
		UpdatedAt:       "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("CreateAutopilotRun() error = %v", err)
	}
	if run.ID == 0 {
		t.Fatal("autopilot run ID was not assigned")
	}
	first, err := store.SaveAutopilotStep(context.Background(), AutopilotStepRecord{
		RunID:      run.ID,
		StepNumber: 1,
		Status:     "ok",
		ResultJSON: []byte(`{"intent":{"symbol":"BTCUSDT"}}`),
		EventsJSON: []byte(`[{"type":"AI Decision"}]`),
		CreatedAt:  "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("SaveAutopilotStep() first error = %v", err)
	}
	second, err := store.SaveAutopilotStep(context.Background(), AutopilotStepRecord{
		RunID:      run.ID,
		StepNumber: 2,
		Status:     "failed",
		Error:      "risk rejected",
		ResultJSON: []byte(`{"decision":{"approved":false}}`),
		EventsJSON: []byte(`[]`),
		CreatedAt:  "2026-06-22T10:31:00Z",
	})
	if err != nil {
		t.Fatalf("SaveAutopilotStep() second error = %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("step IDs = first %d second %d", first.ID, second.ID)
	}

	run.Status = "completed"
	run.CompletedSteps = 2
	run.StoppedAt = "2026-06-22T10:31:00Z"
	run.UpdatedAt = "2026-06-22T10:31:00Z"
	updated, err := store.UpdateAutopilotRun(context.Background(), run)
	if err != nil {
		t.Fatalf("UpdateAutopilotRun() error = %v", err)
	}
	if updated.Status != "completed" || updated.CompletedSteps != 2 {
		t.Fatalf("updated = %#v", updated)
	}

	runs, err := store.ListAutopilotRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAutopilotRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != "completed" {
		t.Fatalf("runs = %#v", runs)
	}
	steps, err := store.ListAutopilotSteps(context.Background(), run.ID, 10)
	if err != nil {
		t.Fatalf("ListAutopilotSteps() error = %v", err)
	}
	if len(steps) != 2 || steps[0].StepNumber != 2 || steps[0].Error != "risk rejected" || string(steps[1].EventsJSON) == "" {
		t.Fatalf("steps = %#v", steps)
	}
}

func TestAutopilotStepRejectsInvalidJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.SaveAutopilotStep(context.Background(), AutopilotStepRecord{
		RunID:      1,
		StepNumber: 1,
		Status:     "ok",
		ResultJSON: []byte(`not-json`),
		EventsJSON: []byte(`[]`),
	})
	if !errors.Is(err, ErrAutopilotStepRequired) {
		t.Fatalf("SaveAutopilotStep() err = %v, want ErrAutopilotStepRequired", err)
	}
}

func TestBacktestRunRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.SaveBacktestRun(context.Background(), BacktestRunRecord{
		StrategyName:       "AI Momentum",
		Exchange:           "Binance",
		Symbol:             "btcusdt",
		Interval:           "15m",
		MarketDataSource:   "live public",
		CandleCount:        200,
		TradeCount:         3,
		EndingEquityUSDT:   100450.25,
		ReturnPct:          0.45,
		BenchmarkReturnPct: 0.2,
		MaxDrawdownPct:     1.1,
		FeesUSDT:           2.5,
		ResultJSON:         []byte(`{"summary":{"returnPct":0.45},"equity":[],"trades":[]}`),
		CreatedAt:          "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("SaveBacktestRun() first error = %v", err)
	}
	second, err := store.SaveBacktestRun(context.Background(), BacktestRunRecord{
		StrategyName:       "AI Momentum",
		Exchange:           "OKX",
		Symbol:             "ETH-USDT",
		Interval:           "15m",
		MarketDataSource:   "local seed",
		CandleCount:        180,
		TradeCount:         1,
		EndingEquityUSDT:   99880,
		ReturnPct:          -0.12,
		BenchmarkReturnPct: -0.3,
		MaxDrawdownPct:     2.4,
		FeesUSDT:           1.2,
		ResultJSON:         []byte(`{"summary":{"returnPct":-0.12},"equity":[],"trades":[]}`),
		CreatedAt:          "2026-06-22T10:31:00Z",
	})
	if err != nil {
		t.Fatalf("SaveBacktestRun() second error = %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("backtest run IDs = first %d second %d", first.ID, second.ID)
	}

	records, err := store.ListBacktestRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListBacktestRuns() error = %v", err)
	}
	if len(records) != 2 || records[0].ID != second.ID || records[1].Symbol != "BTCUSDT" {
		t.Fatalf("records = %#v", records)
	}
	if records[0].ReturnPct != second.ReturnPct || string(records[1].ResultJSON) == "" {
		t.Fatalf("records = %#v", records)
	}
}

func TestBacktestRunRejectsInvalidJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.SaveBacktestRun(context.Background(), BacktestRunRecord{
		StrategyName: "AI Momentum",
		Exchange:     "Binance",
		Symbol:       "BTCUSDT",
		ResultJSON:   []byte(`not-json`),
	})
	if !errors.Is(err, ErrBacktestRunRequired) {
		t.Fatalf("SaveBacktestRun() err = %v, want ErrBacktestRunRequired", err)
	}
}

func TestPruneLocalDataKeepsProtectedLedgersAndAudits(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	for index := 1; index <= 4; index++ {
		if _, err := store.SaveBacktestRun(context.Background(), BacktestRunRecord{
			StrategyName:       "AI Momentum",
			Exchange:           "Binance",
			Symbol:             "BTCUSDT",
			Interval:           "15m",
			MarketDataSource:   "live public",
			CandleCount:        200,
			TradeCount:         index,
			EndingEquityUSDT:   100000 + float64(index),
			ReturnPct:          float64(index) / 100,
			BenchmarkReturnPct: 0.1,
			ResultJSON:         []byte(`{"summary":{"symbol":"BTCUSDT"},"equity":[],"trades":[]}`),
			CreatedAt:          "2026-06-22T10:30:00Z",
		}); err != nil {
			t.Fatalf("SaveBacktestRun(%d) error = %v", index, err)
		}
	}

	for runIndex := 1; runIndex <= 3; runIndex++ {
		run, err := store.CreateAutopilotRun(context.Background(), AutopilotRunRecord{
			Mode:            "shadow",
			Exchange:        "Binance",
			Symbol:          "BTCUSDT",
			Operator:        "qa",
			IntervalSeconds: 15,
			MaxSteps:        2,
			Status:          "completed",
			StartedAt:       "2026-06-22T10:30:00Z",
			UpdatedAt:       "2026-06-22T10:30:00Z",
		})
		if err != nil {
			t.Fatalf("CreateAutopilotRun(%d) error = %v", runIndex, err)
		}
		for step := 1; step <= 2; step++ {
			if _, err := store.SaveAutopilotStep(context.Background(), AutopilotStepRecord{
				RunID:      run.ID,
				StepNumber: step,
				Status:     "ok",
				ResultJSON: []byte(`{"decision":{"approved":true}}`),
				EventsJSON: []byte(`[]`),
				CreatedAt:  "2026-06-22T10:30:00Z",
			}); err != nil {
				t.Fatalf("SaveAutopilotStep(%d,%d) error = %v", runIndex, step, err)
			}
		}
	}

	for index := 1; index <= 4; index++ {
		if _, err := store.SavePaperExecution(context.Background(), PaperExecutionRecord{
			Mode:         "paper",
			Source:       "manual",
			IntentID:     "sim-prune-" + string(rune('0'+index)),
			Exchange:     "Binance",
			Symbol:       "BTCUSDT",
			Side:         "buy",
			SizeUSDT:     500,
			IntentPrice:  67000,
			RiskStatus:   "approved",
			FillStatus:   "filled",
			FillPrice:    67010,
			IntentJSON:   []byte(`{"symbol":"BTCUSDT"}`),
			DecisionJSON: []byte(`{"approved":true}`),
			FillJSON:     []byte(`{"price":67010}`),
			EventsJSON:   []byte(`[{"type":"Sim Fill"}]`),
			CreatedAt:    "2026-06-22T10:30:00Z",
		}); err != nil {
			t.Fatalf("SavePaperExecution(%d) error = %v", index, err)
		}
		if _, err := store.SaveAccountSnapshot(context.Background(), AccountSnapshotRecord{
			CredentialID:   1,
			Exchange:       "Binance",
			Environment:    "testnet",
			Symbol:         "BTCUSDT",
			BalanceCount:   1,
			OpenOrderCount: 0,
			SnapshotJSON:   []byte(`{"balances":[{"asset":"USDT","total":1000}]}`),
			CreatedAt:      "2026-06-22T10:30:00Z",
		}); err != nil {
			t.Fatalf("SaveAccountSnapshot(%d) error = %v", index, err)
		}
	}

	execution, err := store.SaveLiveExecution(context.Background(), LiveExecutionRecord{
		IntentID:        "LIVE-1",
		ClientOrderID:   "LIVE-1",
		CredentialID:    3,
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		Side:            "buy",
		SizeUSDT:        100,
		ValidationOnly:  false,
		RiskStatus:      "approved",
		ExecutionStatus: "submitted",
		IntentJSON:      []byte(`{"id":"LIVE-1"}`),
		DecisionJSON:    []byte(`{"approved":true}`),
		ExecutionJSON:   []byte(`{"status":"submitted"}`),
		CreatedAt:       "2026-06-22T10:30:00Z",
		UpdatedAt:       "2026-06-22T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("SaveLiveExecution() error = %v", err)
	}
	if _, err := store.SaveLiveReconciliation(context.Background(), LiveReconciliationRecord{
		LiveExecutionID: execution.ID,
		CredentialID:    3,
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		ClientOrderID:   "LIVE-1",
		Status:          "filled",
		FilledUSDT:      100,
		ReportJSON:      []byte(`{"status":"filled"}`),
		CreatedAt:       "2026-06-22T10:31:00Z",
	}); err != nil {
		t.Fatalf("SaveLiveReconciliation() error = %v", err)
	}

	before, err := store.LocalDataSummary(context.Background())
	if err != nil {
		t.Fatalf("LocalDataSummary() before error = %v", err)
	}
	if before.BacktestRuns != 4 || before.AutopilotRuns != 3 || before.AutopilotSteps != 6 || before.PaperExecutions != 4 || before.AccountSnapshots != 4 || before.LiveExecutions != 1 || before.LiveReconciliations != 1 {
		t.Fatalf("before = %#v", before)
	}

	report, entry, err := store.PruneLocalData(context.Background(), LocalDataPruneOptions{
		KeepBacktestRuns:     2,
		KeepAutopilotRuns:    1,
		KeepPaperExecutions:  2,
		KeepAccountSnapshots: 1,
	}, audit.Record{
		Actor:   "qa",
		Action:  "local_data.prune",
		Entity:  "local_data",
		Status:  "approved",
		Summary: "local research data pruned",
		Payload: map[string]any{"reason": "unit test"},
	})
	if err != nil {
		t.Fatalf("PruneLocalData() error = %v", err)
	}
	if entry.Action != "local_data.prune" || report.AuditID != entry.ID {
		t.Fatalf("entry = %#v report = %#v", entry, report)
	}
	if report.Deleted.BacktestRuns != 2 || report.Deleted.AutopilotRuns != 2 || report.Deleted.AutopilotSteps != 4 || report.Deleted.PaperExecutions != 2 || report.Deleted.AccountSnapshots != 3 {
		t.Fatalf("deleted = %#v", report.Deleted)
	}
	if report.After.BacktestRuns != 2 || report.After.AutopilotRuns != 1 || report.After.AutopilotSteps != 2 || report.After.PaperExecutions != 2 || report.After.AccountSnapshots != 1 {
		t.Fatalf("after = %#v", report.After)
	}
	if report.After.LiveExecutions != 1 || report.After.LiveReconciliations != 1 || report.After.AuditEntries != 1 {
		t.Fatalf("protected after = %#v", report.After)
	}
	if !strings.Contains(string(entry.Payload), `"live_execution_records"`) {
		t.Fatalf("entry payload = %s", string(entry.Payload))
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 1 {
		t.Fatalf("verification = %#v", verification)
	}
}
