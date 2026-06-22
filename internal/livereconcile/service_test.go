package livereconcile

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

func TestReconcileRejectsValidationOnlyExecutionBeforeClient(t *testing.T) {
	store := openTestStore(t)
	execution := saveLiveExecution(t, store, storage.LiveExecutionRecord{
		ValidationOnly:  true,
		ExecutionStatus: "validated",
	})
	client := &fakeReconcileClient{}
	service := New(store, map[string]Client{"Binance": client})
	service.Now = fixedNow

	_, err := service.Reconcile(context.Background(), Request{
		Operator:        "qa",
		LiveExecutionID: execution.ID,
		Passphrase:      "correct horse battery",
	})
	if !errors.Is(err, ErrValidationOnlyExecution) {
		t.Fatalf("Reconcile() err = %v, want ErrValidationOnlyExecution", err)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "live_reconcile.execution" || entries[0].Status != "rejected" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestReconcileRejectsUnsubmittedExecutionBeforeCredentialUse(t *testing.T) {
	store := openTestStore(t)
	execution := saveLiveExecution(t, store, storage.LiveExecutionRecord{
		ExecutionStatus: "not_submitted",
	})
	service := New(store, map[string]Client{"Binance": &fakeReconcileClient{}})
	service.Now = fixedNow

	_, err := service.Reconcile(context.Background(), Request{
		Operator:        "qa",
		LiveExecutionID: execution.ID,
		Passphrase:      "correct horse battery",
	})
	if !errors.Is(err, ErrUnsubmittedExecution) {
		t.Fatalf("Reconcile() err = %v, want ErrUnsubmittedExecution", err)
	}
}

func TestReconcileRejectsCredentialExchangeMismatch(t *testing.T) {
	store := openTestStore(t)
	meta := saveCredential(t, store, "OKX")
	execution := saveLiveExecution(t, store, storage.LiveExecutionRecord{
		CredentialID:    meta.ID,
		Exchange:        "Binance",
		ExecutionStatus: "submitted",
	})
	service := New(store, map[string]Client{"Binance": &fakeReconcileClient{}})
	service.Now = fixedNow

	_, err := service.Reconcile(context.Background(), Request{
		Operator:        "qa",
		LiveExecutionID: execution.ID,
		Passphrase:      "correct horse battery",
	})
	if !errors.Is(err, ErrCredentialExchange) {
		t.Fatalf("Reconcile() err = %v, want ErrCredentialExchange", err)
	}
}

func TestReconcilePersistsReportAndAudit(t *testing.T) {
	store := openTestStore(t)
	meta := saveCredential(t, store, "Binance")
	execution := saveLiveExecution(t, store, storage.LiveExecutionRecord{
		CredentialID:    meta.ID,
		ExecutionStatus: "submitted",
		ClientOrderID:   "CCVAR123",
		ExchangeOrderID: "98765",
		ValidationOnly:  false,
		ExecutionJSON:   []byte(`{"status":"submitted","exchangeOrderId":"98765"}`),
	})
	client := &fakeReconcileClient{report: Report{
		Exchange:        "Binance",
		Environment:     "testnet",
		Endpoint:        "/api/v3/order",
		ClientOrderID:   "CCVAR123",
		ExchangeOrderID: "98765",
		Status:          "filled",
		Message:         "Binance order status: FILLED",
		FilledUSDT:      101.5,
		CheckedAt:       fixedNow().Format(time.RFC3339),
		Raw:             map[string]any{"status": "FILLED"},
	}}
	service := New(store, map[string]Client{"Binance": client})
	service.Now = fixedNow

	result, err := service.Reconcile(context.Background(), Request{
		Operator:        "qa",
		LiveExecutionID: execution.ID,
		Passphrase:      "correct horse battery",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if client.last.ClientOrderID != "CCVAR123" || client.last.ExchangeOrderID != "98765" || client.last.Credential.APIKey != "BINANCEKEY123456" {
		t.Fatalf("client request = %#v", client.last)
	}
	if result.Reconciliation.ID == 0 || result.Reconciliation.Status != "filled" || result.Reconciliation.FilledUSDT != 101.5 {
		t.Fatalf("result = %#v", result)
	}
	records, err := store.ListLiveReconciliations(context.Background(), execution.ID, 10)
	if err != nil {
		t.Fatalf("ListLiveReconciliations() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != result.Reconciliation.ID {
		t.Fatalf("records = %#v", records)
	}
	verification, err := store.VerifyAudit(context.Background())
	if err != nil {
		t.Fatalf("VerifyAudit() error = %v", err)
	}
	if !verification.Valid || verification.Checked != 1 {
		t.Fatalf("verification = %#v", verification)
	}
}

type fakeReconcileClient struct {
	calls  int
	last   ClientRequest
	report Report
	err    error
}

func (client *fakeReconcileClient) Reconcile(_ context.Context, request ClientRequest) (Report, error) {
	client.calls++
	client.last = request
	if client.err != nil {
		return Report{}, client.err
	}
	return client.report, nil
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func saveCredential(t *testing.T, store *storage.Store, exchange string) vault.CredentialMeta {
	t.Helper()
	input := vault.CredentialInput{
		Exchange:   exchange,
		Label:      exchange + " qa key",
		APIKey:     strings.ToUpper(exchange) + "KEY123456",
		Secret:     strings.ToUpper(exchange) + "SECRET789",
		Passphrase: "correct horse battery",
		Permissions: vault.Permissions{
			Read:  true,
			Trade: true,
		},
	}
	if exchange == "OKX" {
		input.APIPassphrase = "okx-api-passphrase"
	}
	encrypted, err := vault.EncryptCredential(input, fixedNow())
	if err != nil {
		t.Fatalf("EncryptCredential() error = %v", err)
	}
	meta, err := store.SaveCredential(context.Background(), encrypted)
	if err != nil {
		t.Fatalf("SaveCredential() error = %v", err)
	}
	return meta
}

func saveLiveExecution(t *testing.T, store *storage.Store, overrides storage.LiveExecutionRecord) storage.LiveExecutionRecord {
	t.Helper()
	record := storage.LiveExecutionRecord{
		IntentID:        "CCVAR123",
		ClientOrderID:   "CCVAR123",
		ExchangeOrderID: "98765",
		CredentialID:    1,
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		Side:            "buy",
		SizeUSDT:        100,
		ValidationOnly:  false,
		RiskStatus:      "approved",
		ExecutionStatus: "submitted",
		IntentJSON:      []byte(`{"id":"CCVAR123","symbol":"BTCUSDT"}`),
		DecisionJSON:    []byte(`{"approved":true,"reasons":[]}`),
		ExecutionJSON:   []byte(`{"status":"submitted"}`),
		CreatedAt:       fixedNow().Format(time.RFC3339),
		UpdatedAt:       fixedNow().Format(time.RFC3339),
	}
	if overrides.IntentID != "" {
		record.IntentID = overrides.IntentID
	}
	if overrides.ClientOrderID != "" {
		record.ClientOrderID = overrides.ClientOrderID
	}
	if overrides.ExchangeOrderID != "" {
		record.ExchangeOrderID = overrides.ExchangeOrderID
	}
	if overrides.CredentialID != 0 {
		record.CredentialID = overrides.CredentialID
	}
	if overrides.Exchange != "" {
		record.Exchange = overrides.Exchange
	}
	if overrides.Environment != "" {
		record.Environment = overrides.Environment
	}
	if overrides.Symbol != "" {
		record.Symbol = overrides.Symbol
	}
	if overrides.Side != "" {
		record.Side = overrides.Side
	}
	if overrides.SizeUSDT != 0 {
		record.SizeUSDT = overrides.SizeUSDT
	}
	record.ValidationOnly = overrides.ValidationOnly
	if overrides.RiskStatus != "" {
		record.RiskStatus = overrides.RiskStatus
	}
	if overrides.ExecutionStatus != "" {
		record.ExecutionStatus = overrides.ExecutionStatus
	}
	if len(overrides.IntentJSON) > 0 {
		record.IntentJSON = overrides.IntentJSON
	}
	if len(overrides.DecisionJSON) > 0 {
		record.DecisionJSON = overrides.DecisionJSON
	}
	if len(overrides.ExecutionJSON) > 0 {
		record.ExecutionJSON = overrides.ExecutionJSON
	}
	saved, err := store.SaveLiveExecution(context.Background(), record)
	if err != nil {
		t.Fatalf("SaveLiveExecution() error = %v", err)
	}
	return saved
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)
}
