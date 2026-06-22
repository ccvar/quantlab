package livesync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/audit"
	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

var (
	ErrCredentialRequired     = errors.New("credential id is required")
	ErrCredentialPassRequired = errors.New("credential passphrase is required")
	ErrCredentialExchange     = errors.New("credential exchange does not match request")
	ErrUnsupportedExchange    = errors.New("unsupported account sync exchange")
	ErrUnsupportedEnvironment = errors.New("account sync supports only testnet or demo")
)

type Client interface {
	Sync(context.Context, ClientRequest) (AccountSnapshot, error)
}

type Service struct {
	Store   *storage.Store
	Clients map[string]Client
	Now     func() time.Time
}

type Request struct {
	Operator     string `json:"operator"`
	CredentialID int64  `json:"credentialId"`
	Passphrase   string `json:"passphrase"`
	Exchange     string `json:"exchange"`
	Environment  string `json:"environment"`
	Symbol       string `json:"symbol"`
}

type ClientRequest struct {
	Environment string
	Symbol      string
	Credential  vault.PlainCredential
	Now         time.Time
}

type Result struct {
	Snapshot    AccountSnapshot `json:"snapshot"`
	Credential  CredentialView  `json:"credential"`
	SnapshotID  int64           `json:"snapshotId,omitempty"`
	PersistedAt string          `json:"persistedAt,omitempty"`
}

type LatestRequest struct {
	CredentialID int64  `json:"credentialId"`
	Exchange     string `json:"exchange"`
	Environment  string `json:"environment"`
	Symbol       string `json:"symbol"`
}

type LatestResult struct {
	Snapshot     *AccountSnapshot `json:"snapshot"`
	SnapshotID   int64            `json:"snapshotId,omitempty"`
	CredentialID int64            `json:"credentialId,omitempty"`
	PersistedAt  string           `json:"persistedAt,omitempty"`
}

type CredentialView struct {
	ID         int64  `json:"id"`
	Exchange   string `json:"exchange"`
	Label      string `json:"label"`
	APIKeyMask string `json:"apiKeyMask"`
}

type AccountSnapshot struct {
	Exchange     string      `json:"exchange"`
	Environment  string      `json:"environment"`
	AccountType  string      `json:"accountType"`
	CanTrade     bool        `json:"canTrade"`
	Balances     []Balance   `json:"balances"`
	OpenOrders   []OpenOrder `json:"openOrders"`
	RawUpdatedAt string      `json:"rawUpdatedAt,omitempty"`
	SyncedAt     string      `json:"syncedAt"`
}

type Balance struct {
	Asset  string  `json:"asset"`
	Free   float64 `json:"free"`
	Locked float64 `json:"locked"`
	Total  float64 `json:"total"`
	USD    float64 `json:"usd,omitempty"`
}

type OpenOrder struct {
	Symbol        string  `json:"symbol"`
	OrderID       string  `json:"orderId"`
	ClientOrderID string  `json:"clientOrderId"`
	Side          string  `json:"side"`
	Type          string  `json:"type"`
	Status        string  `json:"status"`
	Price         float64 `json:"price"`
	OrigQty       float64 `json:"origQty"`
	ExecutedQty   float64 `json:"executedQty"`
	QuoteQty      float64 `json:"quoteQty"`
	UpdatedAt     string  `json:"updatedAt"`
}

func New(store *storage.Store, clients map[string]Client) Service {
	return Service{
		Store:   store,
		Clients: clients,
		Now:     time.Now,
	}
}

func (service Service) Sync(ctx context.Context, request Request) (Result, error) {
	now := service.clock()
	operator := defaultString(request.Operator, "local")
	environment := normalizeEnvironment(request.Environment)
	if environment == "" {
		environment = "testnet"
	}
	if environment != "testnet" && environment != "demo" {
		_ = service.audit(ctx, audit.Record{
			Actor:   operator,
			Action:  "account_sync.rejected",
			Entity:  "account",
			Status:  "rejected",
			Summary: ErrUnsupportedEnvironment.Error(),
			Payload: safeRequestPayload(request, environment, ErrUnsupportedEnvironment.Error()),
		})
		return Result{}, ErrUnsupportedEnvironment
	}
	if request.CredentialID <= 0 {
		_ = service.audit(ctx, audit.Record{
			Actor:   operator,
			Action:  "account_sync.credential",
			Entity:  "credential",
			Status:  "rejected",
			Summary: ErrCredentialRequired.Error(),
			Payload: safeRequestPayload(request, environment, ErrCredentialRequired.Error()),
		})
		return Result{}, ErrCredentialRequired
	}
	if strings.TrimSpace(request.Passphrase) == "" {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "account_sync.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(request.CredentialID, 10),
			Status:   "rejected",
			Summary:  ErrCredentialPassRequired.Error(),
			Payload:  safeRequestPayload(request, environment, ErrCredentialPassRequired.Error()),
		})
		return Result{}, ErrCredentialPassRequired
	}

	encrypted, err := service.Store.GetEncryptedCredential(ctx, request.CredentialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Result{}, fmt.Errorf("credential %d not found", request.CredentialID)
		}
		return Result{}, err
	}
	exchangeName := canonicalExchange(defaultString(request.Exchange, encrypted.Exchange))
	if !strings.EqualFold(exchangeName, encrypted.Exchange) {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "account_sync.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  ErrCredentialExchange.Error(),
			Payload: map[string]any{
				"requestExchange":    exchangeName,
				"credentialExchange": encrypted.Exchange,
				"credentialId":       encrypted.ID,
				"environment":        environment,
			},
		})
		return Result{Credential: credentialView(encrypted)}, ErrCredentialExchange
	}
	client, ok := service.Clients[exchangeName]
	if !ok {
		return Result{Credential: credentialView(encrypted)}, ErrUnsupportedExchange
	}
	plain, err := vault.DecryptCredential(encrypted, request.Passphrase)
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "account_sync.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  "credential decrypt failed",
			Payload: map[string]any{
				"exchange": encrypted.Exchange,
				"label":    encrypted.Label,
			},
		})
		return Result{Credential: credentialView(encrypted)}, err
	}

	symbol := strings.ToUpper(defaultString(request.Symbol, "BTCUSDT"))
	snapshot, err := client.Sync(ctx, ClientRequest{
		Environment: environment,
		Symbol:      symbol,
		Credential:  plain,
		Now:         now,
	})
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "account_sync.snapshot",
			Entity:   "account",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "failed",
			Summary:  err.Error(),
			Payload:  safeRequestPayload(request, environment, err.Error()),
		})
		return Result{Credential: credentialView(encrypted)}, err
	}
	if snapshot.SyncedAt == "" {
		snapshot.SyncedAt = now.UTC().Format(time.RFC3339)
	}
	record, err := service.persistSnapshot(ctx, encrypted.ID, symbol, snapshot, now)
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "account_sync.snapshot",
			Entity:   "account",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "failed",
			Summary:  "account snapshot persistence failed",
			Payload:  safeRequestPayload(request, environment, err.Error()),
		})
		return Result{Snapshot: snapshot, Credential: credentialView(encrypted)}, err
	}
	_ = service.audit(ctx, audit.Record{
		Actor:    operator,
		Action:   "account_sync.snapshot",
		Entity:   "account",
		EntityID: strconv.FormatInt(encrypted.ID, 10),
		Status:   "approved",
		Summary:  syncSummary(snapshot),
		Payload: map[string]any{
			"exchange":      snapshot.Exchange,
			"environment":   snapshot.Environment,
			"credentialId":  encrypted.ID,
			"snapshotId":    record.ID,
			"balanceCount":  len(snapshot.Balances),
			"openOrders":    len(snapshot.OpenOrders),
			"symbol":        symbol,
			"accountType":   snapshot.AccountType,
			"canTrade":      snapshot.CanTrade,
			"nonZeroAssets": nonZeroAssets(snapshot.Balances),
		},
	})
	return Result{Snapshot: snapshot, Credential: credentialView(encrypted), SnapshotID: record.ID, PersistedAt: record.CreatedAt}, nil
}

func (service Service) Latest(ctx context.Context, request LatestRequest) (LatestResult, error) {
	if service.Store == nil {
		return LatestResult{}, nil
	}
	record, err := service.Store.LatestAccountSnapshot(ctx, storage.AccountSnapshotFilter{
		CredentialID: request.CredentialID,
		Exchange:     canonicalExchange(request.Exchange),
		Environment:  normalizeEnvironment(request.Environment),
		Symbol:       strings.ToUpper(strings.TrimSpace(request.Symbol)),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LatestResult{Snapshot: nil}, nil
		}
		return LatestResult{}, err
	}
	var snapshot AccountSnapshot
	if err := json.Unmarshal(record.SnapshotJSON, &snapshot); err != nil {
		return LatestResult{}, err
	}
	return LatestResult{
		Snapshot:     &snapshot,
		SnapshotID:   record.ID,
		CredentialID: record.CredentialID,
		PersistedAt:  record.CreatedAt,
	}, nil
}

func (service Service) clock() time.Time {
	if service.Now == nil {
		return time.Now().UTC()
	}
	return service.Now().UTC()
}

func (service Service) audit(ctx context.Context, record audit.Record) error {
	if service.Store == nil {
		return nil
	}
	_, err := service.Store.AppendAudit(ctx, record)
	return err
}

func (service Service) persistSnapshot(ctx context.Context, credentialID int64, symbol string, snapshot AccountSnapshot, now time.Time) (storage.AccountSnapshotRecord, error) {
	if service.Store == nil {
		return storage.AccountSnapshotRecord{}, nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return storage.AccountSnapshotRecord{}, err
	}
	return service.Store.SaveAccountSnapshot(ctx, storage.AccountSnapshotRecord{
		CredentialID:   credentialID,
		Exchange:       snapshot.Exchange,
		Environment:    snapshot.Environment,
		Symbol:         symbol,
		BalanceCount:   len(snapshot.Balances),
		OpenOrderCount: len(snapshot.OpenOrders),
		SnapshotJSON:   payload,
		CreatedAt:      now.UTC().Format(time.RFC3339),
	})
}

func syncSummary(snapshot AccountSnapshot) string {
	return fmt.Sprintf("%s %s account sync: %d balances, %d open orders", snapshot.Exchange, snapshot.Environment, len(snapshot.Balances), len(snapshot.OpenOrders))
}

func nonZeroAssets(balances []Balance) []string {
	assets := []string{}
	for _, balance := range balances {
		if balance.Total > 0 {
			assets = append(assets, balance.Asset)
		}
	}
	return assets
}

func credentialView(credential vault.EncryptedCredential) CredentialView {
	return CredentialView{
		ID:         credential.ID,
		Exchange:   credential.Exchange,
		Label:      credential.Label,
		APIKeyMask: credential.APIKeyMask,
	}
}

func safeRequestPayload(request Request, environment string, errorText string) map[string]any {
	return map[string]any{
		"exchange":     request.Exchange,
		"symbol":       request.Symbol,
		"credentialId": request.CredentialID,
		"environment":  environment,
		"error":        errorText,
	}
}

func normalizeEnvironment(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func canonicalExchange(value string) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(value), "Binance"):
		return "Binance"
	case strings.EqualFold(strings.TrimSpace(value), "OKX"):
		return "OKX"
	default:
		return strings.TrimSpace(value)
	}
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
