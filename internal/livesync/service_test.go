package livesync

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

func TestSyncRejectsProductionEnvironmentAndAudits(t *testing.T) {
	store := openTestStore(t)
	service := New(store, map[string]Client{})

	_, err := service.Sync(context.Background(), Request{
		Environment:  "production",
		CredentialID: 1,
		Passphrase:   "correct horse battery",
	})
	if !errors.Is(err, ErrUnsupportedEnvironment) {
		t.Fatalf("Sync() err = %v, want ErrUnsupportedEnvironment", err)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "account_sync.rejected" || entries[0].Status != "rejected" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestSyncDecryptsCredentialAndAuditsSnapshot(t *testing.T) {
	store := openTestStore(t)
	meta := saveBinanceCredential(t, store)
	client := fakeSyncClient{}
	service := New(store, map[string]Client{"Binance": &client})
	service.Now = fixedNow

	result, err := service.Sync(context.Background(), Request{
		Operator:     "qa",
		CredentialID: meta.ID,
		Passphrase:   "correct horse battery",
		Exchange:     "Binance",
		Environment:  "testnet",
		Symbol:       "BTCUSDT",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if client.request.Credential.APIKey != "BINANCEKEY123456" || client.request.Environment != "testnet" {
		t.Fatalf("client request = %#v", client.request)
	}
	if result.Snapshot.Exchange != "Binance" || len(result.Snapshot.Balances) != 1 || len(result.Snapshot.OpenOrders) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.SnapshotID == 0 || result.PersistedAt == "" {
		t.Fatalf("snapshot persistence metadata missing: %#v", result)
	}
	latest, err := service.Latest(context.Background(), LatestRequest{
		CredentialID: meta.ID,
		Exchange:     "Binance",
		Environment:  "testnet",
		Symbol:       "BTCUSDT",
	})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if latest.Snapshot == nil || latest.SnapshotID != result.SnapshotID || len(latest.Snapshot.Balances) != 1 {
		t.Fatalf("latest = %#v", latest)
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
	if entries[0].Action != "account_sync.snapshot" || entries[0].Status != "approved" {
		t.Fatalf("entry = %#v", entries[0])
	}
}

func TestLatestReturnsNilSnapshotWhenEmpty(t *testing.T) {
	store := openTestStore(t)
	service := New(store, map[string]Client{})

	result, err := service.Latest(context.Background(), LatestRequest{Exchange: "Binance"})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if result.Snapshot != nil || result.SnapshotID != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSyncAuditsMissingCredential(t *testing.T) {
	store := openTestStore(t)
	service := New(store, map[string]Client{})

	_, err := service.Sync(context.Background(), Request{
		Environment: "testnet",
	})
	if !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("Sync() err = %v, want ErrCredentialRequired", err)
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if entries[0].Action != "account_sync.credential" || entries[0].Status != "rejected" {
		t.Fatalf("entry = %#v", entries[0])
	}
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "ccvar-livesync-test.db"))
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

func fixedNow() time.Time {
	return time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)
}

type fakeSyncClient struct {
	request ClientRequest
}

func (client *fakeSyncClient) Sync(_ context.Context, request ClientRequest) (AccountSnapshot, error) {
	client.request = request
	return AccountSnapshot{
		Exchange:    "Binance",
		Environment: request.Environment,
		AccountType: "SPOT",
		CanTrade:    true,
		Balances: []Balance{{
			Asset: "USDT",
			Free:  1000,
			Total: 1000,
		}},
		OpenOrders: []OpenOrder{{
			Symbol:        "BTCUSDT",
			OrderID:       "1",
			ClientOrderID: "CCVAR1",
			Status:        "NEW",
		}},
	}, nil
}
