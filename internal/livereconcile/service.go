package livereconcile

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
	ErrLiveExecutionRequired   = errors.New("live execution id is required")
	ErrLiveExecutionNotFound   = errors.New("live execution record not found")
	ErrCredentialPassRequired  = errors.New("credential passphrase is required")
	ErrCredentialExchange      = errors.New("credential exchange does not match execution")
	ErrUnsupportedExchange     = errors.New("unsupported live reconciliation exchange")
	ErrUnsupportedEnvironment  = errors.New("live reconciliation supports only testnet or demo")
	ErrValidationOnlyExecution = errors.New("validation-only execution has no exchange order to reconcile")
	ErrUnsubmittedExecution    = errors.New("live execution was not submitted to an exchange")
)

type Client interface {
	Reconcile(context.Context, ClientRequest) (Report, error)
}

type Service struct {
	Store   *storage.Store
	Clients map[string]Client
	Now     func() time.Time
}

type Request struct {
	Operator        string `json:"operator"`
	LiveExecutionID int64  `json:"liveExecutionId"`
	Passphrase      string `json:"passphrase"`
}

type ClientRequest struct {
	Environment     string
	Symbol          string
	ClientOrderID   string
	ExchangeOrderID string
	Credential      vault.PlainCredential
	Now             time.Time
}

type Report struct {
	Exchange        string         `json:"exchange"`
	Environment     string         `json:"environment"`
	Endpoint        string         `json:"endpoint"`
	ClientOrderID   string         `json:"clientOrderId"`
	ExchangeOrderID string         `json:"exchangeOrderId,omitempty"`
	Status          string         `json:"status"`
	Message         string         `json:"message"`
	FilledUSDT      float64        `json:"filledUsdt"`
	CheckedAt       string         `json:"checkedAt"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type Result struct {
	Reconciliation storage.LiveReconciliationRecord `json:"reconciliation"`
	Report         Report                           `json:"report"`
	Execution      storage.LiveExecutionRecord      `json:"execution"`
}

func New(store *storage.Store, clients map[string]Client) Service {
	return Service{
		Store:   store,
		Clients: clients,
		Now:     time.Now,
	}
}

func (service Service) Reconcile(ctx context.Context, request Request) (Result, error) {
	now := service.clock()
	operator := defaultString(request.Operator, "local")
	if request.LiveExecutionID <= 0 {
		_ = service.audit(ctx, audit.Record{
			Actor:   operator,
			Action:  "live_reconcile.execution",
			Entity:  "live_execution",
			Status:  "rejected",
			Summary: ErrLiveExecutionRequired.Error(),
			Payload: map[string]any{"liveExecutionId": request.LiveExecutionID},
		})
		return Result{}, ErrLiveExecutionRequired
	}

	execution, err := service.Store.GetLiveExecution(ctx, request.LiveExecutionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = service.audit(ctx, audit.Record{
				Actor:    operator,
				Action:   "live_reconcile.execution",
				Entity:   "live_execution",
				EntityID: strconv.FormatInt(request.LiveExecutionID, 10),
				Status:   "rejected",
				Summary:  ErrLiveExecutionNotFound.Error(),
				Payload:  map[string]any{"liveExecutionId": request.LiveExecutionID},
			})
			return Result{}, ErrLiveExecutionNotFound
		}
		return Result{}, err
	}

	if err := validateExecution(execution); err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_reconcile.execution",
			Entity:   "live_execution",
			EntityID: strconv.FormatInt(execution.ID, 10),
			Status:   "rejected",
			Summary:  err.Error(),
			Payload:  executionPayload(execution, err.Error()),
		})
		return Result{Execution: execution}, err
	}
	if strings.TrimSpace(request.Passphrase) == "" {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_reconcile.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(execution.CredentialID, 10),
			Status:   "rejected",
			Summary:  ErrCredentialPassRequired.Error(),
			Payload:  executionPayload(execution, ErrCredentialPassRequired.Error()),
		})
		return Result{Execution: execution}, ErrCredentialPassRequired
	}

	encrypted, err := service.Store.GetEncryptedCredential(ctx, execution.CredentialID)
	if err != nil {
		return Result{Execution: execution}, err
	}
	exchangeName := canonicalExchange(execution.Exchange)
	if !strings.EqualFold(exchangeName, encrypted.Exchange) {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_reconcile.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  ErrCredentialExchange.Error(),
			Payload: map[string]any{
				"executionExchange":  exchangeName,
				"credentialExchange": encrypted.Exchange,
				"credentialId":       encrypted.ID,
				"liveExecutionId":    execution.ID,
			},
		})
		return Result{Execution: execution}, ErrCredentialExchange
	}
	plain, err := vault.DecryptCredential(encrypted, request.Passphrase)
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_reconcile.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  "credential decrypt failed",
			Payload: map[string]any{
				"exchange":        encrypted.Exchange,
				"label":           encrypted.Label,
				"liveExecutionId": execution.ID,
			},
		})
		return Result{Execution: execution}, err
	}
	client, ok := service.Clients[exchangeName]
	if !ok {
		return Result{Execution: execution}, ErrUnsupportedExchange
	}

	report, err := client.Reconcile(ctx, ClientRequest{
		Environment:     normalizeEnvironment(execution.Environment),
		Symbol:          execution.Symbol,
		ClientOrderID:   execution.ClientOrderID,
		ExchangeOrderID: execution.ExchangeOrderID,
		Credential:      plain,
		Now:             now,
	})
	report.Exchange = defaultString(report.Exchange, exchangeName)
	report.Environment = defaultString(report.Environment, normalizeEnvironment(execution.Environment))
	report.ClientOrderID = defaultString(report.ClientOrderID, execution.ClientOrderID)
	report.ExchangeOrderID = defaultString(report.ExchangeOrderID, execution.ExchangeOrderID)
	report.Status = defaultString(report.Status, "unknown")
	report.CheckedAt = defaultString(report.CheckedAt, now.UTC().Format(time.RFC3339))
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_reconcile.order",
			Entity:   "order",
			EntityID: execution.ClientOrderID,
			Status:   "failed",
			Summary:  err.Error(),
			Payload:  reconciliationPayload(execution, report, err.Error()),
		})
		return Result{Execution: execution, Report: report}, err
	}

	reportJSON, err := json.Marshal(report)
	if err != nil {
		return Result{Execution: execution, Report: report}, err
	}
	record, err := service.Store.SaveLiveReconciliation(ctx, storage.LiveReconciliationRecord{
		LiveExecutionID: execution.ID,
		CredentialID:    execution.CredentialID,
		Exchange:        exchangeName,
		Environment:     normalizeEnvironment(execution.Environment),
		Symbol:          execution.Symbol,
		ClientOrderID:   report.ClientOrderID,
		ExchangeOrderID: report.ExchangeOrderID,
		Status:          report.Status,
		FilledUSDT:      report.FilledUSDT,
		ReportJSON:      reportJSON,
		CreatedAt:       now.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Result{Execution: execution, Report: report}, err
	}
	_ = service.audit(ctx, audit.Record{
		Actor:    operator,
		Action:   "live_reconcile.order",
		Entity:   "order",
		EntityID: report.ClientOrderID,
		Status:   "approved",
		Summary:  reconcileSummary(report),
		Payload:  reconciliationPayload(execution, report, ""),
	})
	return Result{Reconciliation: record, Report: report, Execution: execution}, nil
}

func (service Service) Latest(ctx context.Context, liveExecutionID int64, limit int) ([]storage.LiveReconciliationRecord, error) {
	if service.Store == nil {
		return []storage.LiveReconciliationRecord{}, nil
	}
	return service.Store.ListLiveReconciliations(ctx, liveExecutionID, limit)
}

func validateExecution(execution storage.LiveExecutionRecord) error {
	environment := normalizeEnvironment(execution.Environment)
	if environment != "testnet" && environment != "demo" {
		return ErrUnsupportedEnvironment
	}
	if execution.ValidationOnly {
		return ErrValidationOnlyExecution
	}
	if !isSubmittedStatus(execution.ExecutionStatus) {
		return ErrUnsubmittedExecution
	}
	if strings.TrimSpace(execution.ClientOrderID) == "" && strings.TrimSpace(execution.ExchangeOrderID) == "" {
		return ErrUnsubmittedExecution
	}
	return nil
}

func isSubmittedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "not_submitted", "failed", "validated", "signed-preflight", "rejected":
		return false
	default:
		return true
	}
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

func executionPayload(execution storage.LiveExecutionRecord, errorText string) map[string]any {
	return map[string]any{
		"liveExecutionId": execution.ID,
		"credentialId":    execution.CredentialID,
		"exchange":        execution.Exchange,
		"environment":     execution.Environment,
		"symbol":          execution.Symbol,
		"clientOrderId":   execution.ClientOrderID,
		"exchangeOrderId": execution.ExchangeOrderID,
		"executionStatus": execution.ExecutionStatus,
		"validationOnly":  execution.ValidationOnly,
		"error":           errorText,
	}
}

func reconciliationPayload(execution storage.LiveExecutionRecord, report Report, errorText string) map[string]any {
	return map[string]any{
		"liveExecutionId": execution.ID,
		"credentialId":    execution.CredentialID,
		"exchange":        report.Exchange,
		"environment":     report.Environment,
		"symbol":          execution.Symbol,
		"endpoint":        report.Endpoint,
		"clientOrderId":   report.ClientOrderID,
		"exchangeOrderId": report.ExchangeOrderID,
		"status":          report.Status,
		"filledUsdt":      report.FilledUSDT,
		"checkedAt":       report.CheckedAt,
		"error":           errorText,
	}
}

func reconcileSummary(report Report) string {
	return fmt.Sprintf("%s %s order %s, filled %.2f USDT", report.Exchange, report.Environment, report.Status, report.FilledUSDT)
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
