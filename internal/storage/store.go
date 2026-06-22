package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/audit"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/vault"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

var (
	ErrAccountSnapshotRequired = errors.New("account snapshot json is required")
	ErrLiveExecutionRequired   = errors.New("live execution intent and decision json are required")
	ErrLiveReconcileRequired   = errors.New("live reconciliation report json is required")
	ErrPaperExecutionRequired  = errors.New("paper execution intent, decision, fill, and events json are required")
	ErrAutopilotRunRequired    = errors.New("autopilot run mode, exchange, symbol, and status are required")
	ErrAutopilotStepRequired   = errors.New("autopilot step run id, status, and valid json payloads are required")
	ErrBacktestRunRequired     = errors.New("backtest run strategy, exchange, symbol, and valid result json are required")
)

type LabState struct {
	Meta        Meta            `json:"meta"`
	Runs        []ExperimentRun `json:"runs"`
	Candles     []Candle        `json:"candles"`
	Equity      []Point         `json:"equity"`
	Benchmark   []Point         `json:"benchmark"`
	Verdict     Verdict         `json:"verdict"`
	Features    []FeatureImpact `json:"features"`
	Performance []MetricRow     `json:"performance"`
	Positions   []Position      `json:"positions"`
	Orders      []Order         `json:"orders"`
	Events      []Event         `json:"events"`
}

type Meta struct {
	DataSource     string  `json:"dataSource"`
	Mode           string  `json:"mode"`
	Strategy       string  `json:"strategy"`
	Model          string  `json:"model"`
	SimCapital     float64 `json:"simCapital"`
	DailyPnL       float64 `json:"dailyPnl"`
	DailyPnLPct    float64 `json:"dailyPnlPct"`
	DailyDrawdown  float64 `json:"dailyDrawdown"`
	DataLatencyMS  int     `json:"dataLatencyMs"`
	LastUpdated    string  `json:"lastUpdated"`
	SelectedSymbol string  `json:"selectedSymbol"`
	SelectedMarket string  `json:"selectedMarket"`
	SlippageModel  string  `json:"slippageModel"`
	FeeModel       string  `json:"feeModel"`
	FundingModel   string  `json:"fundingModel"`
}

type ExperimentRun struct {
	Name     string  `json:"name"`
	Version  string  `json:"version"`
	Run      string  `json:"run"`
	Status   string  `json:"status"`
	Return7D float64 `json:"return7d"`
	MaxDD    float64 `json:"maxDd"`
	WinRate  float64 `json:"winRate"`
	LastRun  string  `json:"lastRun"`
}

type Candle = core.Candle

type Point struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type Verdict struct {
	Signal           string  `json:"signal"`
	Confidence       float64 `json:"confidence"`
	Uncertainty      string  `json:"uncertainty"`
	UncertaintyScore float64 `json:"uncertaintyScore"`
	Regime           string  `json:"regime"`
	RiskOverride     string  `json:"riskOverride"`
	TTL              string  `json:"ttl"`
	ExpiresAt        string  `json:"expiresAt"`
	Reasoning        string  `json:"reasoning"`
}

type FeatureImpact struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Impact string  `json:"impact"`
}

type MetricRow struct {
	Metric       string `json:"metric"`
	SevenDay     string `json:"sevenDay"`
	ThirtyDay    string `json:"thirtyDay"`
	AllTime      string `json:"allTime"`
	Benchmark7D  string `json:"benchmark7d"`
	Benchmark30D string `json:"benchmark30d"`
	BenchmarkAll string `json:"benchmarkAll"`
	Trend        string `json:"trend"`
}

type Position struct {
	Symbol string  `json:"symbol"`
	Side   string  `json:"side"`
	Size   string  `json:"size"`
	Entry  float64 `json:"entry"`
	Mark   float64 `json:"mark"`
	PnL    float64 `json:"pnl"`
	PnLPct float64 `json:"pnlPct"`
	Risk   string  `json:"risk"`
	Age    string  `json:"age"`
}

type Order struct {
	Symbol  string  `json:"symbol"`
	Side    string  `json:"side"`
	Type    string  `json:"type"`
	Size    string  `json:"size"`
	Price   float64 `json:"price"`
	Status  string  `json:"status"`
	Created string  `json:"created"`
}

type Event struct {
	Time   string  `json:"time"`
	Type   string  `json:"type"`
	Symbol string  `json:"symbol"`
	Action string  `json:"action"`
	Price  float64 `json:"price"`
	Result string  `json:"result"`
	Note   string  `json:"note"`
	Level  string  `json:"level"`
}

type AccountSnapshotRecord struct {
	ID             int64           `json:"id"`
	CredentialID   int64           `json:"credentialId"`
	Exchange       string          `json:"exchange"`
	Environment    string          `json:"environment"`
	Symbol         string          `json:"symbol"`
	BalanceCount   int             `json:"balanceCount"`
	OpenOrderCount int             `json:"openOrderCount"`
	SnapshotJSON   json.RawMessage `json:"snapshot"`
	CreatedAt      string          `json:"createdAt"`
}

type AccountSnapshotFilter struct {
	CredentialID int64
	Exchange     string
	Environment  string
	Symbol       string
}

type LiveExecutionRecord struct {
	ID              int64           `json:"id"`
	IntentID        string          `json:"intentId"`
	ClientOrderID   string          `json:"clientOrderId"`
	ExchangeOrderID string          `json:"exchangeOrderId,omitempty"`
	CredentialID    int64           `json:"credentialId"`
	Exchange        string          `json:"exchange"`
	Environment     string          `json:"environment"`
	Symbol          string          `json:"symbol"`
	Side            string          `json:"side"`
	SizeUSDT        float64         `json:"sizeUsdt"`
	ValidationOnly  bool            `json:"validationOnly"`
	RiskStatus      string          `json:"riskStatus"`
	ExecutionStatus string          `json:"executionStatus"`
	IntentJSON      json.RawMessage `json:"intent"`
	DecisionJSON    json.RawMessage `json:"decision"`
	ExecutionJSON   json.RawMessage `json:"execution,omitempty"`
	CreatedAt       string          `json:"createdAt"`
	UpdatedAt       string          `json:"updatedAt"`
}

type LiveReconciliationRecord struct {
	ID              int64           `json:"id"`
	LiveExecutionID int64           `json:"liveExecutionId"`
	CredentialID    int64           `json:"credentialId"`
	Exchange        string          `json:"exchange"`
	Environment     string          `json:"environment"`
	Symbol          string          `json:"symbol"`
	ClientOrderID   string          `json:"clientOrderId"`
	ExchangeOrderID string          `json:"exchangeOrderId,omitempty"`
	Status          string          `json:"status"`
	FilledUSDT      float64         `json:"filledUsdt"`
	ReportJSON      json.RawMessage `json:"report"`
	CreatedAt       string          `json:"createdAt"`
}

type PaperExecutionRecord struct {
	ID           int64           `json:"id"`
	RunID        int64           `json:"runId,omitempty"`
	Mode         string          `json:"mode"`
	Source       string          `json:"source"`
	IntentID     string          `json:"intentId"`
	Exchange     string          `json:"exchange"`
	Symbol       string          `json:"symbol"`
	Side         string          `json:"side"`
	SizeUSDT     float64         `json:"sizeUsdt"`
	IntentPrice  float64         `json:"intentPrice"`
	Confidence   float64         `json:"confidence"`
	RiskStatus   string          `json:"riskStatus"`
	FillStatus   string          `json:"fillStatus"`
	FillPrice    float64         `json:"fillPrice,omitempty"`
	FeeUSDT      float64         `json:"feeUsdt,omitempty"`
	SlippageUSDT float64         `json:"slippageUsdt,omitempty"`
	IntentJSON   json.RawMessage `json:"intent"`
	DecisionJSON json.RawMessage `json:"decision"`
	FillJSON     json.RawMessage `json:"fill"`
	EventsJSON   json.RawMessage `json:"events"`
	CreatedAt    string          `json:"createdAt"`
}

type RiskProfileRecord struct {
	ID                    int64   `json:"id"`
	Name                  string  `json:"name"`
	MinConfidence         float64 `json:"minConfidence"`
	MaxOrderUSDT          float64 `json:"maxOrderUsdt"`
	MaxSymbolExposureUSDT float64 `json:"maxSymbolExposureUsdt"`
	MaxTotalExposureUSDT  float64 `json:"maxTotalExposureUsdt"`
	MaxDailyDrawdownPct   float64 `json:"maxDailyDrawdownPct"`
	MaxConsecutiveLosses  int     `json:"maxConsecutiveLosses"`
	MaxSpreadPct          float64 `json:"maxSpreadPct"`
	RequireLiveUnlock     bool    `json:"requireLiveUnlock"`
	UpdatedAt             string  `json:"updatedAt,omitempty"`
}

type StrategyProfileRecord struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Exchange        string  `json:"exchange"`
	Symbol          string  `json:"symbol"`
	Side            string  `json:"side"`
	OrderSizeUSDT   float64 `json:"orderSizeUsdt"`
	IntervalSeconds int     `json:"intervalSeconds"`
	MaxSteps        int     `json:"maxSteps"`
	UpdatedAt       string  `json:"updatedAt,omitempty"`
}

type AutopilotRunRecord struct {
	ID              int64   `json:"id"`
	Mode            string  `json:"mode"`
	Exchange        string  `json:"exchange"`
	Environment     string  `json:"environment,omitempty"`
	Symbol          string  `json:"symbol"`
	Operator        string  `json:"operator"`
	IntervalSeconds int     `json:"intervalSeconds"`
	MaxSteps        int     `json:"maxSteps"`
	CredentialID    int64   `json:"credentialId,omitempty"`
	Side            string  `json:"side,omitempty"`
	SizeUSDT        float64 `json:"sizeUsdt,omitempty"`
	ValidationOnly  bool    `json:"validationOnly"`
	Status          string  `json:"status"`
	CompletedSteps  int     `json:"completedSteps"`
	LastError       string  `json:"lastError,omitempty"`
	StartedAt       string  `json:"startedAt"`
	StoppedAt       string  `json:"stoppedAt,omitempty"`
	UpdatedAt       string  `json:"updatedAt"`
}

type AutopilotStepRecord struct {
	ID         int64           `json:"id"`
	RunID      int64           `json:"runId"`
	StepNumber int             `json:"stepNumber"`
	Status     string          `json:"status"`
	Error      string          `json:"error,omitempty"`
	ResultJSON json.RawMessage `json:"result"`
	EventsJSON json.RawMessage `json:"events"`
	CreatedAt  string          `json:"createdAt"`
}

type BacktestRunRecord struct {
	ID                 int64           `json:"id"`
	StrategyName       string          `json:"strategyName"`
	Exchange           string          `json:"exchange"`
	Symbol             string          `json:"symbol"`
	Interval           string          `json:"interval"`
	MarketDataSource   string          `json:"marketDataSource"`
	CandleCount        int             `json:"candleCount"`
	TradeCount         int             `json:"tradeCount"`
	EndingEquityUSDT   float64         `json:"endingEquityUsdt"`
	ReturnPct          float64         `json:"returnPct"`
	BenchmarkReturnPct float64         `json:"benchmarkReturnPct"`
	MaxDrawdownPct     float64         `json:"maxDrawdownPct"`
	FeesUSDT           float64         `json:"feesUsdt"`
	ResultJSON         json.RawMessage `json:"result"`
	CreatedAt          string          `json:"createdAt"`
}

type LocalDataSummary struct {
	BacktestRuns        int64 `json:"backtestRuns"`
	AutopilotRuns       int64 `json:"autopilotRuns"`
	AutopilotSteps      int64 `json:"autopilotSteps"`
	PaperExecutions     int64 `json:"paperExecutions"`
	AccountSnapshots    int64 `json:"accountSnapshots"`
	LiveExecutions      int64 `json:"liveExecutions"`
	LiveReconciliations int64 `json:"liveReconciliations"`
	AuditEntries        int64 `json:"auditEntries"`
	Credentials         int64 `json:"credentials"`
}

type LocalDataPruneOptions struct {
	KeepBacktestRuns     int `json:"keepBacktestRuns"`
	KeepAutopilotRuns    int `json:"keepAutopilotRuns"`
	KeepPaperExecutions  int `json:"keepPaperExecutions"`
	KeepAccountSnapshots int `json:"keepAccountSnapshots"`
}

type LocalDataDeleted struct {
	BacktestRuns     int64 `json:"backtestRuns"`
	AutopilotRuns    int64 `json:"autopilotRuns"`
	AutopilotSteps   int64 `json:"autopilotSteps"`
	PaperExecutions  int64 `json:"paperExecutions"`
	AccountSnapshots int64 `json:"accountSnapshots"`
}

type LocalDataPruneReport struct {
	Before    LocalDataSummary      `json:"before"`
	After     LocalDataSummary      `json:"after"`
	Deleted   LocalDataDeleted      `json:"deleted"`
	Keep      LocalDataPruneOptions `json:"keep"`
	Protected []string              `json:"protected"`
	AuditID   int64                 `json:"auditId,omitempty"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveCredential(ctx context.Context, credential vault.EncryptedCredential) (vault.CredentialMeta, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if credential.CreatedAt == "" {
		credential.CreatedAt = now
	}
	if credential.UpdatedAt == "" {
		credential.UpdatedAt = credential.CreatedAt
	}
	permissions, err := json.Marshal(credential.Permissions)
	if err != nil {
		return vault.CredentialMeta{}, err
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO api_credentials (
			exchange,
			label,
			api_key_mask,
			permissions_json,
			kdf_name,
			kdf_iterations,
			salt,
			nonce,
			ciphertext,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		credential.Exchange,
		credential.Label,
		credential.APIKeyMask,
		string(permissions),
		credential.KDFName,
		credential.KDFIterations,
		credential.Salt,
		credential.Nonce,
		credential.Ciphertext,
		credential.CreatedAt,
		credential.UpdatedAt,
	)
	if err != nil {
		return vault.CredentialMeta{}, err
	}
	credential.ID, err = result.LastInsertId()
	if err != nil {
		return vault.CredentialMeta{}, err
	}
	return credential.Meta(), nil
}

func (s *Store) ListCredentials(ctx context.Context) ([]vault.CredentialMeta, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, exchange, label, api_key_mask, permissions_json, created_at, updated_at FROM api_credentials ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []vault.CredentialMeta{}
	for rows.Next() {
		var credential vault.CredentialMeta
		var permissionsJSON string
		if err := rows.Scan(&credential.ID, &credential.Exchange, &credential.Label, &credential.APIKeyMask, &permissionsJSON, &credential.CreatedAt, &credential.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(permissionsJSON), &credential.Permissions); err != nil {
			return nil, err
		}
		result = append(result, credential)
	}
	return result, rows.Err()
}

func (s *Store) GetEncryptedCredential(ctx context.Context, id int64) (vault.EncryptedCredential, error) {
	var credential vault.EncryptedCredential
	var permissionsJSON string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, exchange, label, api_key_mask, permissions_json, kdf_name, kdf_iterations, salt, nonce, ciphertext, created_at, updated_at FROM api_credentials WHERE id = ?`,
		id,
	).Scan(
		&credential.ID,
		&credential.Exchange,
		&credential.Label,
		&credential.APIKeyMask,
		&permissionsJSON,
		&credential.KDFName,
		&credential.KDFIterations,
		&credential.Salt,
		&credential.Nonce,
		&credential.Ciphertext,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	)
	if err != nil {
		return vault.EncryptedCredential{}, err
	}
	if err := json.Unmarshal([]byte(permissionsJSON), &credential.Permissions); err != nil {
		return vault.EncryptedCredential{}, err
	}
	return credential, nil
}

func (s *Store) DeleteCredential(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_credentials WHERE id = ?`, id)
	return err
}

func (s *Store) SaveAccountSnapshot(ctx context.Context, record AccountSnapshotRecord) (AccountSnapshotRecord, error) {
	if len(record.SnapshotJSON) == 0 || !json.Valid(record.SnapshotJSON) {
		return AccountSnapshotRecord{}, ErrAccountSnapshotRequired
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO account_snapshots (
			credential_id,
			exchange,
			environment,
			symbol,
			balance_count,
			open_order_count,
			snapshot_json,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.CredentialID,
		record.Exchange,
		record.Environment,
		record.Symbol,
		record.BalanceCount,
		record.OpenOrderCount,
		string(record.SnapshotJSON),
		record.CreatedAt,
	)
	if err != nil {
		return AccountSnapshotRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return AccountSnapshotRecord{}, err
	}
	return record, nil
}

func (s *Store) LatestAccountSnapshot(ctx context.Context, filter AccountSnapshotFilter) (AccountSnapshotRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			credential_id,
			exchange,
			environment,
			symbol,
			balance_count,
			open_order_count,
			snapshot_json,
			created_at
		FROM account_snapshots
		WHERE (? <= 0 OR credential_id = ?)
			AND (? = '' OR exchange = ?)
			AND (? = '' OR environment = ?)
			AND (? = '' OR symbol = ?)
		ORDER BY id DESC
		LIMIT 1`,
		filter.CredentialID,
		filter.CredentialID,
		filter.Exchange,
		filter.Exchange,
		filter.Environment,
		filter.Environment,
		filter.Symbol,
		filter.Symbol,
	)
	return scanAccountSnapshot(row)
}

func (s *Store) SaveLiveExecution(ctx context.Context, record LiveExecutionRecord) (LiveExecutionRecord, error) {
	if len(record.IntentJSON) == 0 || !json.Valid(record.IntentJSON) || len(record.DecisionJSON) == 0 || !json.Valid(record.DecisionJSON) {
		return LiveExecutionRecord{}, ErrLiveExecutionRequired
	}
	if len(record.ExecutionJSON) > 0 && !json.Valid(record.ExecutionJSON) {
		return LiveExecutionRecord{}, ErrLiveExecutionRequired
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = record.CreatedAt
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO live_execution_records (
			intent_id,
			client_order_id,
			exchange_order_id,
			credential_id,
			exchange,
			environment,
			symbol,
			side,
			size_usdt,
			validation_only,
			risk_status,
			execution_status,
			intent_json,
			decision_json,
			execution_json,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.IntentID,
		record.ClientOrderID,
		record.ExchangeOrderID,
		record.CredentialID,
		record.Exchange,
		record.Environment,
		record.Symbol,
		record.Side,
		record.SizeUSDT,
		record.ValidationOnly,
		record.RiskStatus,
		record.ExecutionStatus,
		string(record.IntentJSON),
		string(record.DecisionJSON),
		string(record.ExecutionJSON),
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return LiveExecutionRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return LiveExecutionRecord{}, err
	}
	return record, nil
}

func (s *Store) ListLiveExecutions(ctx context.Context, limit int) ([]LiveExecutionRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			intent_id,
			client_order_id,
			exchange_order_id,
			credential_id,
			exchange,
			environment,
			symbol,
			side,
			size_usdt,
			validation_only,
			risk_status,
			execution_status,
			intent_json,
			decision_json,
			execution_json,
			created_at,
			updated_at
		FROM live_execution_records
		ORDER BY id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []LiveExecutionRecord{}
	for rows.Next() {
		record, err := scanLiveExecution(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) GetLiveExecution(ctx context.Context, id int64) (LiveExecutionRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			intent_id,
			client_order_id,
			exchange_order_id,
			credential_id,
			exchange,
			environment,
			symbol,
			side,
			size_usdt,
			validation_only,
			risk_status,
			execution_status,
			intent_json,
			decision_json,
			execution_json,
			created_at,
			updated_at
		FROM live_execution_records
		WHERE id = ?`,
		id,
	)
	return scanLiveExecution(row)
}

func (s *Store) SavePaperExecution(ctx context.Context, record PaperExecutionRecord) (PaperExecutionRecord, error) {
	if len(record.IntentJSON) == 0 || !json.Valid(record.IntentJSON) || len(record.DecisionJSON) == 0 || !json.Valid(record.DecisionJSON) {
		return PaperExecutionRecord{}, ErrPaperExecutionRequired
	}
	if len(record.FillJSON) == 0 {
		record.FillJSON = json.RawMessage(`null`)
	}
	if len(record.EventsJSON) == 0 {
		record.EventsJSON = json.RawMessage(`[]`)
	}
	if !json.Valid(record.FillJSON) || !json.Valid(record.EventsJSON) {
		return PaperExecutionRecord{}, ErrPaperExecutionRequired
	}
	record.Mode = strings.ToLower(defaultString(record.Mode, "shadow"))
	if record.Mode != "paper" {
		record.Mode = "shadow"
	}
	record.Source = defaultString(record.Source, "manual")
	record.RiskStatus = defaultString(record.RiskStatus, "unknown")
	record.FillStatus = defaultString(record.FillStatus, "not_filled")
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO paper_execution_records (
			run_id,
			mode,
			source,
			intent_id,
			exchange,
			symbol,
			side,
			size_usdt,
			intent_price,
			confidence,
			risk_status,
			fill_status,
			fill_price,
			fee_usdt,
			slippage_usdt,
			intent_json,
			decision_json,
			fill_json,
			events_json,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.RunID,
		record.Mode,
		record.Source,
		record.IntentID,
		record.Exchange,
		record.Symbol,
		record.Side,
		record.SizeUSDT,
		record.IntentPrice,
		record.Confidence,
		record.RiskStatus,
		record.FillStatus,
		record.FillPrice,
		record.FeeUSDT,
		record.SlippageUSDT,
		string(record.IntentJSON),
		string(record.DecisionJSON),
		string(record.FillJSON),
		string(record.EventsJSON),
		record.CreatedAt,
	)
	if err != nil {
		return PaperExecutionRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return PaperExecutionRecord{}, err
	}
	return record, nil
}

func (s *Store) ListPaperExecutions(ctx context.Context, limit int) ([]PaperExecutionRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			run_id,
			mode,
			source,
			intent_id,
			exchange,
			symbol,
			side,
			size_usdt,
			intent_price,
			confidence,
			risk_status,
			fill_status,
			fill_price,
			fee_usdt,
			slippage_usdt,
			intent_json,
			decision_json,
			fill_json,
			events_json,
			created_at
		FROM paper_execution_records
		ORDER BY id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []PaperExecutionRecord{}
	for rows.Next() {
		record, err := scanPaperExecution(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) ResetPaperExecutions(ctx context.Context, record audit.Record) (int64, audit.Entry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, audit.Entry{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `DELETE FROM paper_execution_records`)
	if err != nil {
		return 0, audit.Entry{}, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, audit.Entry{}, err
	}
	record.Payload = mergeAuditPayload(record.Payload, map[string]any{
		"deletedRecords": deleted,
	})
	entry, err := appendAuditEntryTx(ctx, tx, record)
	if err != nil {
		return 0, audit.Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return 0, audit.Entry{}, err
	}
	return deleted, entry, nil
}

func (s *Store) SaveLiveReconciliation(ctx context.Context, record LiveReconciliationRecord) (LiveReconciliationRecord, error) {
	if len(record.ReportJSON) == 0 || !json.Valid(record.ReportJSON) {
		return LiveReconciliationRecord{}, ErrLiveReconcileRequired
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO live_reconciliation_records (
			live_execution_id,
			credential_id,
			exchange,
			environment,
			symbol,
			client_order_id,
			exchange_order_id,
			status,
			filled_usdt,
			report_json,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.LiveExecutionID,
		record.CredentialID,
		record.Exchange,
		record.Environment,
		record.Symbol,
		record.ClientOrderID,
		record.ExchangeOrderID,
		record.Status,
		record.FilledUSDT,
		string(record.ReportJSON),
		record.CreatedAt,
	)
	if err != nil {
		return LiveReconciliationRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return LiveReconciliationRecord{}, err
	}
	return record, nil
}

func (s *Store) ListLiveReconciliations(ctx context.Context, liveExecutionID int64, limit int) ([]LiveReconciliationRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			live_execution_id,
			credential_id,
			exchange,
			environment,
			symbol,
			client_order_id,
			exchange_order_id,
			status,
			filled_usdt,
			report_json,
			created_at
		FROM live_reconciliation_records
		WHERE (? <= 0 OR live_execution_id = ?)
		ORDER BY id DESC
		LIMIT ?`,
		liveExecutionID,
		liveExecutionID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []LiveReconciliationRecord{}
	for rows.Next() {
		record, err := scanLiveReconciliation(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func DefaultRiskProfile() RiskProfileRecord {
	return RiskProfileRecord{
		ID:                    1,
		Name:                  "Local Guardrails",
		MinConfidence:         0.65,
		MaxOrderUSDT:          1000,
		MaxSymbolExposureUSDT: 8000,
		MaxTotalExposureUSDT:  12000,
		MaxDailyDrawdownPct:   3,
		MaxConsecutiveLosses:  3,
		MaxSpreadPct:          0.08,
		RequireLiveUnlock:     true,
	}
}

func NormalizeRiskProfile(record RiskProfileRecord) RiskProfileRecord {
	defaults := DefaultRiskProfile()
	normalized := record
	normalized.ID = 1
	normalized.Name = defaultString(record.Name, defaults.Name)
	normalized.MinConfidence = positiveOrDefault(record.MinConfidence, defaults.MinConfidence)
	if normalized.MinConfidence > 1 {
		normalized.MinConfidence = 1
	}
	normalized.MaxOrderUSDT = positiveOrDefault(record.MaxOrderUSDT, defaults.MaxOrderUSDT)
	normalized.MaxSymbolExposureUSDT = positiveOrDefault(record.MaxSymbolExposureUSDT, defaults.MaxSymbolExposureUSDT)
	normalized.MaxTotalExposureUSDT = positiveOrDefault(record.MaxTotalExposureUSDT, defaults.MaxTotalExposureUSDT)
	normalized.MaxDailyDrawdownPct = positiveOrDefault(record.MaxDailyDrawdownPct, defaults.MaxDailyDrawdownPct)
	if normalized.MaxDailyDrawdownPct > 100 {
		normalized.MaxDailyDrawdownPct = 100
	}
	normalized.MaxConsecutiveLosses = record.MaxConsecutiveLosses
	if normalized.MaxConsecutiveLosses <= 0 {
		normalized.MaxConsecutiveLosses = defaults.MaxConsecutiveLosses
	}
	normalized.MaxSpreadPct = positiveOrDefault(record.MaxSpreadPct, defaults.MaxSpreadPct)
	normalized.RequireLiveUnlock = true
	return normalized
}

func (s *Store) RiskProfile(ctx context.Context) (RiskProfileRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			name,
			min_confidence,
			max_order_usdt,
			max_symbol_exposure_usdt,
			max_total_exposure_usdt,
			max_daily_drawdown_pct,
			max_consecutive_losses,
			max_spread_pct,
			require_live_unlock,
			updated_at
		FROM risk_profiles
		WHERE id = 1`,
	)
	record, err := scanRiskProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultRiskProfile(), nil
	}
	if err != nil {
		return RiskProfileRecord{}, err
	}
	return NormalizeRiskProfile(record), nil
}

func (s *Store) SaveRiskProfile(ctx context.Context, record RiskProfileRecord) (RiskProfileRecord, error) {
	record = NormalizeRiskProfile(record)
	if record.UpdatedAt == "" {
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO risk_profiles (
			id,
			name,
			min_confidence,
			max_order_usdt,
			max_symbol_exposure_usdt,
			max_total_exposure_usdt,
			max_daily_drawdown_pct,
			max_consecutive_losses,
			max_spread_pct,
			require_live_unlock,
			updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			min_confidence = excluded.min_confidence,
			max_order_usdt = excluded.max_order_usdt,
			max_symbol_exposure_usdt = excluded.max_symbol_exposure_usdt,
			max_total_exposure_usdt = excluded.max_total_exposure_usdt,
			max_daily_drawdown_pct = excluded.max_daily_drawdown_pct,
			max_consecutive_losses = excluded.max_consecutive_losses,
			max_spread_pct = excluded.max_spread_pct,
			require_live_unlock = excluded.require_live_unlock,
			updated_at = excluded.updated_at`,
		record.Name,
		record.MinConfidence,
		record.MaxOrderUSDT,
		record.MaxSymbolExposureUSDT,
		record.MaxTotalExposureUSDT,
		record.MaxDailyDrawdownPct,
		record.MaxConsecutiveLosses,
		record.MaxSpreadPct,
		record.RequireLiveUnlock,
		record.UpdatedAt,
	)
	if err != nil {
		return RiskProfileRecord{}, err
	}
	return record, nil
}

func DefaultStrategyProfile() StrategyProfileRecord {
	return StrategyProfileRecord{
		ID:              1,
		Name:            "AI Momentum Pro",
		Exchange:        "Binance",
		Symbol:          "BTCUSDT",
		Side:            "buy",
		OrderSizeUSDT:   500,
		IntervalSeconds: 15,
		MaxSteps:        0,
	}
}

func NormalizeStrategyProfile(record StrategyProfileRecord) StrategyProfileRecord {
	defaults := DefaultStrategyProfile()
	normalized := record
	normalized.ID = 1
	normalized.Name = defaultString(record.Name, defaults.Name)
	normalized.Exchange = defaultString(record.Exchange, defaults.Exchange)
	normalized.Symbol = strings.ToUpper(defaultString(record.Symbol, defaults.Symbol))
	normalized.Side = strings.ToLower(defaultString(record.Side, defaults.Side))
	if normalized.Side != "sell" {
		normalized.Side = "buy"
	}
	normalized.OrderSizeUSDT = positiveOrDefault(record.OrderSizeUSDT, defaults.OrderSizeUSDT)
	normalized.IntervalSeconds = record.IntervalSeconds
	if normalized.IntervalSeconds <= 0 {
		normalized.IntervalSeconds = defaults.IntervalSeconds
	}
	if normalized.IntervalSeconds < 5 {
		normalized.IntervalSeconds = 5
	}
	if normalized.IntervalSeconds > 3600 {
		normalized.IntervalSeconds = 3600
	}
	normalized.MaxSteps = record.MaxSteps
	if normalized.MaxSteps < 0 {
		normalized.MaxSteps = 0
	}
	return normalized
}

func (s *Store) StrategyProfile(ctx context.Context) (StrategyProfileRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			name,
			exchange,
			symbol,
			side,
			order_size_usdt,
			interval_seconds,
			max_steps,
			updated_at
		FROM strategy_profiles
		WHERE id = 1`,
	)
	record, err := scanStrategyProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultStrategyProfile(), nil
	}
	if err != nil {
		return StrategyProfileRecord{}, err
	}
	return NormalizeStrategyProfile(record), nil
}

func (s *Store) SaveStrategyProfile(ctx context.Context, record StrategyProfileRecord) (StrategyProfileRecord, error) {
	record = NormalizeStrategyProfile(record)
	if record.UpdatedAt == "" {
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO strategy_profiles (
			id,
			name,
			exchange,
			symbol,
			side,
			order_size_usdt,
			interval_seconds,
			max_steps,
			updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			exchange = excluded.exchange,
			symbol = excluded.symbol,
			side = excluded.side,
			order_size_usdt = excluded.order_size_usdt,
			interval_seconds = excluded.interval_seconds,
			max_steps = excluded.max_steps,
			updated_at = excluded.updated_at`,
		record.Name,
		record.Exchange,
		record.Symbol,
		record.Side,
		record.OrderSizeUSDT,
		record.IntervalSeconds,
		record.MaxSteps,
		record.UpdatedAt,
	)
	if err != nil {
		return StrategyProfileRecord{}, err
	}
	return record, nil
}

func (s *Store) CreateAutopilotRun(ctx context.Context, record AutopilotRunRecord) (AutopilotRunRecord, error) {
	if record.Mode == "" || record.Exchange == "" || record.Symbol == "" || record.Status == "" {
		return AutopilotRunRecord{}, ErrAutopilotRunRequired
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if record.StartedAt == "" {
		record.StartedAt = now
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = record.StartedAt
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO autopilot_runs (
			mode,
			exchange,
			environment,
			symbol,
			operator,
			interval_seconds,
			max_steps,
			credential_id,
			side,
			size_usdt,
			validation_only,
			status,
			completed_steps,
			last_error,
			started_at,
			stopped_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Mode,
		record.Exchange,
		record.Environment,
		record.Symbol,
		record.Operator,
		record.IntervalSeconds,
		record.MaxSteps,
		record.CredentialID,
		record.Side,
		record.SizeUSDT,
		record.ValidationOnly,
		record.Status,
		record.CompletedSteps,
		record.LastError,
		record.StartedAt,
		record.StoppedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return AutopilotRunRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return AutopilotRunRecord{}, err
	}
	return record, nil
}

func (s *Store) UpdateAutopilotRun(ctx context.Context, record AutopilotRunRecord) (AutopilotRunRecord, error) {
	if record.ID <= 0 || record.Mode == "" || record.Exchange == "" || record.Symbol == "" || record.Status == "" {
		return AutopilotRunRecord{}, ErrAutopilotRunRequired
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE autopilot_runs
		SET mode = ?,
			exchange = ?,
			environment = ?,
			symbol = ?,
			operator = ?,
			interval_seconds = ?,
			max_steps = ?,
			credential_id = ?,
			side = ?,
			size_usdt = ?,
			validation_only = ?,
			status = ?,
			completed_steps = ?,
			last_error = ?,
			started_at = ?,
			stopped_at = ?,
			updated_at = ?
		WHERE id = ?`,
		record.Mode,
		record.Exchange,
		record.Environment,
		record.Symbol,
		record.Operator,
		record.IntervalSeconds,
		record.MaxSteps,
		record.CredentialID,
		record.Side,
		record.SizeUSDT,
		record.ValidationOnly,
		record.Status,
		record.CompletedSteps,
		record.LastError,
		record.StartedAt,
		record.StoppedAt,
		record.UpdatedAt,
		record.ID,
	)
	if err != nil {
		return AutopilotRunRecord{}, err
	}
	return record, nil
}

func (s *Store) SaveAutopilotStep(ctx context.Context, record AutopilotStepRecord) (AutopilotStepRecord, error) {
	if record.RunID <= 0 || record.StepNumber <= 0 || record.Status == "" {
		return AutopilotStepRecord{}, ErrAutopilotStepRequired
	}
	if len(record.ResultJSON) == 0 {
		record.ResultJSON = json.RawMessage(`null`)
	}
	if len(record.EventsJSON) == 0 {
		record.EventsJSON = json.RawMessage(`[]`)
	}
	if !json.Valid(record.ResultJSON) || !json.Valid(record.EventsJSON) {
		return AutopilotStepRecord{}, ErrAutopilotStepRequired
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO autopilot_steps (
			run_id,
			step_number,
			status,
			error,
			result_json,
			events_json,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.RunID,
		record.StepNumber,
		record.Status,
		record.Error,
		string(record.ResultJSON),
		string(record.EventsJSON),
		record.CreatedAt,
	)
	if err != nil {
		return AutopilotStepRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return AutopilotStepRecord{}, err
	}
	return record, nil
}

func (s *Store) ListAutopilotRuns(ctx context.Context, limit int) ([]AutopilotRunRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			mode,
			exchange,
			environment,
			symbol,
			operator,
			interval_seconds,
			max_steps,
			credential_id,
			side,
			size_usdt,
			validation_only,
			status,
			completed_steps,
			last_error,
			started_at,
			stopped_at,
			updated_at
		FROM autopilot_runs
		ORDER BY id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []AutopilotRunRecord{}
	for rows.Next() {
		record, err := scanAutopilotRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) ListAutopilotSteps(ctx context.Context, runID int64, limit int) ([]AutopilotStepRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			run_id,
			step_number,
			status,
			error,
			result_json,
			events_json,
			created_at
		FROM autopilot_steps
		WHERE (? <= 0 OR run_id = ?)
		ORDER BY id DESC
		LIMIT ?`,
		runID,
		runID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []AutopilotStepRecord{}
	for rows.Next() {
		record, err := scanAutopilotStep(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) SaveBacktestRun(ctx context.Context, record BacktestRunRecord) (BacktestRunRecord, error) {
	record.StrategyName = strings.TrimSpace(record.StrategyName)
	record.Exchange = strings.TrimSpace(record.Exchange)
	record.Symbol = strings.ToUpper(strings.TrimSpace(record.Symbol))
	record.Interval = defaultString(record.Interval, "15m")
	record.MarketDataSource = defaultString(record.MarketDataSource, "unknown")
	if record.StrategyName == "" || record.Exchange == "" || record.Symbol == "" || !json.Valid(record.ResultJSON) {
		return BacktestRunRecord{}, ErrBacktestRunRequired
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO backtest_runs (
			strategy_name,
			exchange,
			symbol,
			interval,
			market_data_source,
			candle_count,
			trade_count,
			ending_equity_usdt,
			return_pct,
			benchmark_return_pct,
			max_drawdown_pct,
			fees_usdt,
			result_json,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.StrategyName,
		record.Exchange,
		record.Symbol,
		record.Interval,
		record.MarketDataSource,
		record.CandleCount,
		record.TradeCount,
		record.EndingEquityUSDT,
		record.ReturnPct,
		record.BenchmarkReturnPct,
		record.MaxDrawdownPct,
		record.FeesUSDT,
		string(record.ResultJSON),
		record.CreatedAt,
	)
	if err != nil {
		return BacktestRunRecord{}, err
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return BacktestRunRecord{}, err
	}
	return record, nil
}

func (s *Store) ListBacktestRuns(ctx context.Context, limit int) ([]BacktestRunRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			strategy_name,
			exchange,
			symbol,
			interval,
			market_data_source,
			candle_count,
			trade_count,
			ending_equity_usdt,
			return_pct,
			benchmark_return_pct,
			max_drawdown_pct,
			fees_usdt,
			result_json,
			created_at
		FROM backtest_runs
		ORDER BY id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []BacktestRunRecord{}
	for rows.Next() {
		record, err := scanBacktestRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) LocalDataSummary(ctx context.Context) (LocalDataSummary, error) {
	return localDataSummary(ctx, s.db)
}

func (s *Store) PruneLocalData(ctx context.Context, options LocalDataPruneOptions, record audit.Record) (LocalDataPruneReport, audit.Entry, error) {
	options = normalizePruneOptions(options)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	defer tx.Rollback()

	before, err := localDataSummary(ctx, tx)
	if err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	deleted := LocalDataDeleted{}
	if deleted.BacktestRuns, err = pruneTableByLatestID(ctx, tx, "backtest_runs", options.KeepBacktestRuns); err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	if deleted.AutopilotSteps, err = pruneAutopilotSteps(ctx, tx, options.KeepAutopilotRuns); err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	if deleted.AutopilotRuns, err = pruneTableByLatestID(ctx, tx, "autopilot_runs", options.KeepAutopilotRuns); err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	if deleted.PaperExecutions, err = pruneTableByLatestID(ctx, tx, "paper_execution_records", options.KeepPaperExecutions); err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	if deleted.AccountSnapshots, err = pruneTableByLatestID(ctx, tx, "account_snapshots", options.KeepAccountSnapshots); err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	after, err := localDataSummary(ctx, tx)
	if err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	after.AuditEntries++
	report := LocalDataPruneReport{
		Before:    before,
		After:     after,
		Deleted:   deleted,
		Keep:      options,
		Protected: protectedLocalDataSets(),
	}
	record.Payload = mergeAuditPayload(record.Payload, map[string]any{
		"before":    report.Before,
		"after":     report.After,
		"deleted":   report.Deleted,
		"keep":      report.Keep,
		"protected": report.Protected,
	})
	entry, err := appendAuditEntryTx(ctx, tx, record)
	if err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return LocalDataPruneReport{}, audit.Entry{}, err
	}
	report.AuditID = entry.ID
	return report, entry, nil
}

func (s *Store) AppendAudit(ctx context.Context, record audit.Record) (audit.Entry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return audit.Entry{}, err
	}
	defer tx.Rollback()

	entry, err := appendAuditEntryTx(ctx, tx, record)
	if err != nil {
		return audit.Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return audit.Entry{}, err
	}
	return entry, nil
}

func appendAuditEntryTx(ctx context.Context, tx *sql.Tx, record audit.Record) (audit.Entry, error) {
	var previousHash string
	err := tx.QueryRowContext(ctx, `SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&previousHash)
	if err != nil && err != sql.ErrNoRows {
		return audit.Entry{}, err
	}
	entry, err := audit.NewEntry(record, previousHash, time.Now())
	if err != nil {
		return audit.Entry{}, err
	}
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO audit_log (
			created_at,
			actor,
			action,
			entity,
			entity_id,
			status,
			summary,
			payload_json,
			prev_hash,
			hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.CreatedAt,
		entry.Actor,
		entry.Action,
		entry.Entity,
		entry.EntityID,
		entry.Status,
		entry.Summary,
		string(entry.Payload),
		entry.PrevHash,
		entry.Hash,
	)
	if err != nil {
		return audit.Entry{}, err
	}
	entry.ID, err = result.LastInsertId()
	if err != nil {
		return audit.Entry{}, err
	}
	return entry, nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]audit.Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, created_at, actor, action, entity, entity_id, status, summary, payload_json, prev_hash, hash FROM audit_log ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []audit.Entry{}
	for rows.Next() {
		entry, err := scanAuditEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) VerifyAudit(ctx context.Context) (audit.Verification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, actor, action, entity, entity_id, status, summary, payload_json, prev_hash, hash FROM audit_log ORDER BY id`)
	if err != nil {
		return audit.Verification{}, err
	}
	defer rows.Close()

	var previousHash string
	checked := 0
	for rows.Next() {
		entry, err := scanAuditEntry(rows)
		if err != nil {
			return audit.Verification{}, err
		}
		checked++
		if entry.PrevHash != previousHash {
			return audit.Verification{Valid: false, Checked: checked, Error: "previous hash mismatch"}, nil
		}
		if audit.Hash(entry) != entry.Hash {
			return audit.Verification{Valid: false, Checked: checked, Error: "entry hash mismatch"}, nil
		}
		previousHash = entry.Hash
	}
	if err := rows.Err(); err != nil {
		return audit.Verification{}, err
	}
	return audit.Verification{Valid: true, Checked: checked}, nil
}

type auditScanner interface {
	Scan(dest ...any) error
}

func scanAuditEntry(scanner auditScanner) (audit.Entry, error) {
	var entry audit.Entry
	var payload string
	err := scanner.Scan(
		&entry.ID,
		&entry.CreatedAt,
		&entry.Actor,
		&entry.Action,
		&entry.Entity,
		&entry.EntityID,
		&entry.Status,
		&entry.Summary,
		&payload,
		&entry.PrevHash,
		&entry.Hash,
	)
	if err != nil {
		return audit.Entry{}, err
	}
	entry.Payload = json.RawMessage(payload)
	return entry, nil
}

func scanAccountSnapshot(scanner auditScanner) (AccountSnapshotRecord, error) {
	var record AccountSnapshotRecord
	var snapshotJSON string
	err := scanner.Scan(
		&record.ID,
		&record.CredentialID,
		&record.Exchange,
		&record.Environment,
		&record.Symbol,
		&record.BalanceCount,
		&record.OpenOrderCount,
		&snapshotJSON,
		&record.CreatedAt,
	)
	if err != nil {
		return AccountSnapshotRecord{}, err
	}
	record.SnapshotJSON = json.RawMessage(snapshotJSON)
	return record, nil
}

func scanLiveExecution(scanner auditScanner) (LiveExecutionRecord, error) {
	var record LiveExecutionRecord
	var intentJSON string
	var decisionJSON string
	var executionJSON string
	err := scanner.Scan(
		&record.ID,
		&record.IntentID,
		&record.ClientOrderID,
		&record.ExchangeOrderID,
		&record.CredentialID,
		&record.Exchange,
		&record.Environment,
		&record.Symbol,
		&record.Side,
		&record.SizeUSDT,
		&record.ValidationOnly,
		&record.RiskStatus,
		&record.ExecutionStatus,
		&intentJSON,
		&decisionJSON,
		&executionJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return LiveExecutionRecord{}, err
	}
	record.IntentJSON = json.RawMessage(intentJSON)
	record.DecisionJSON = json.RawMessage(decisionJSON)
	if executionJSON != "" {
		record.ExecutionJSON = json.RawMessage(executionJSON)
	}
	return record, nil
}

func scanLiveReconciliation(scanner auditScanner) (LiveReconciliationRecord, error) {
	var record LiveReconciliationRecord
	var reportJSON string
	err := scanner.Scan(
		&record.ID,
		&record.LiveExecutionID,
		&record.CredentialID,
		&record.Exchange,
		&record.Environment,
		&record.Symbol,
		&record.ClientOrderID,
		&record.ExchangeOrderID,
		&record.Status,
		&record.FilledUSDT,
		&reportJSON,
		&record.CreatedAt,
	)
	if err != nil {
		return LiveReconciliationRecord{}, err
	}
	record.ReportJSON = json.RawMessage(reportJSON)
	return record, nil
}

func scanPaperExecution(scanner auditScanner) (PaperExecutionRecord, error) {
	var record PaperExecutionRecord
	var intentJSON string
	var decisionJSON string
	var fillJSON string
	var eventsJSON string
	err := scanner.Scan(
		&record.ID,
		&record.RunID,
		&record.Mode,
		&record.Source,
		&record.IntentID,
		&record.Exchange,
		&record.Symbol,
		&record.Side,
		&record.SizeUSDT,
		&record.IntentPrice,
		&record.Confidence,
		&record.RiskStatus,
		&record.FillStatus,
		&record.FillPrice,
		&record.FeeUSDT,
		&record.SlippageUSDT,
		&intentJSON,
		&decisionJSON,
		&fillJSON,
		&eventsJSON,
		&record.CreatedAt,
	)
	if err != nil {
		return PaperExecutionRecord{}, err
	}
	record.IntentJSON = json.RawMessage(intentJSON)
	record.DecisionJSON = json.RawMessage(decisionJSON)
	record.FillJSON = json.RawMessage(fillJSON)
	record.EventsJSON = json.RawMessage(eventsJSON)
	return record, nil
}

func scanRiskProfile(scanner auditScanner) (RiskProfileRecord, error) {
	var record RiskProfileRecord
	err := scanner.Scan(
		&record.ID,
		&record.Name,
		&record.MinConfidence,
		&record.MaxOrderUSDT,
		&record.MaxSymbolExposureUSDT,
		&record.MaxTotalExposureUSDT,
		&record.MaxDailyDrawdownPct,
		&record.MaxConsecutiveLosses,
		&record.MaxSpreadPct,
		&record.RequireLiveUnlock,
		&record.UpdatedAt,
	)
	if err != nil {
		return RiskProfileRecord{}, err
	}
	return record, nil
}

func scanStrategyProfile(scanner auditScanner) (StrategyProfileRecord, error) {
	var record StrategyProfileRecord
	err := scanner.Scan(
		&record.ID,
		&record.Name,
		&record.Exchange,
		&record.Symbol,
		&record.Side,
		&record.OrderSizeUSDT,
		&record.IntervalSeconds,
		&record.MaxSteps,
		&record.UpdatedAt,
	)
	if err != nil {
		return StrategyProfileRecord{}, err
	}
	return record, nil
}

func scanAutopilotRun(scanner auditScanner) (AutopilotRunRecord, error) {
	var record AutopilotRunRecord
	err := scanner.Scan(
		&record.ID,
		&record.Mode,
		&record.Exchange,
		&record.Environment,
		&record.Symbol,
		&record.Operator,
		&record.IntervalSeconds,
		&record.MaxSteps,
		&record.CredentialID,
		&record.Side,
		&record.SizeUSDT,
		&record.ValidationOnly,
		&record.Status,
		&record.CompletedSteps,
		&record.LastError,
		&record.StartedAt,
		&record.StoppedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return AutopilotRunRecord{}, err
	}
	return record, nil
}

func scanAutopilotStep(scanner auditScanner) (AutopilotStepRecord, error) {
	var record AutopilotStepRecord
	var resultJSON string
	var eventsJSON string
	err := scanner.Scan(
		&record.ID,
		&record.RunID,
		&record.StepNumber,
		&record.Status,
		&record.Error,
		&resultJSON,
		&eventsJSON,
		&record.CreatedAt,
	)
	if err != nil {
		return AutopilotStepRecord{}, err
	}
	record.ResultJSON = json.RawMessage(resultJSON)
	record.EventsJSON = json.RawMessage(eventsJSON)
	return record, nil
}

func scanBacktestRun(scanner auditScanner) (BacktestRunRecord, error) {
	var record BacktestRunRecord
	var resultJSON string
	err := scanner.Scan(
		&record.ID,
		&record.StrategyName,
		&record.Exchange,
		&record.Symbol,
		&record.Interval,
		&record.MarketDataSource,
		&record.CandleCount,
		&record.TradeCount,
		&record.EndingEquityUSDT,
		&record.ReturnPct,
		&record.BenchmarkReturnPct,
		&record.MaxDrawdownPct,
		&record.FeesUSDT,
		&resultJSON,
		&record.CreatedAt,
	)
	if err != nil {
		return BacktestRunRecord{}, err
	}
	record.ResultJSON = json.RawMessage(resultJSON)
	return record, nil
}

func (s *Store) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
		return err
	}
	for _, statement := range schema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM experiment_runs`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return s.seed(ctx)
	}
	return nil
}

func (s *Store) LabState(ctx context.Context) (LabState, error) {
	runs, err := s.runs(ctx)
	if err != nil {
		return LabState{}, err
	}
	candles, err := s.candles(ctx)
	if err != nil {
		return LabState{}, err
	}
	equity, benchmark, err := s.equity(ctx)
	if err != nil {
		return LabState{}, err
	}
	events, err := s.events(ctx)
	if err != nil {
		return LabState{}, err
	}
	return LabState{
		Meta: Meta{
			DataSource:     "Binance",
			Mode:           "Shadow",
			Strategy:       "AI Momentum Pro",
			Model:          "Local AI Policy v0.2.0",
			SimCapital:     100000,
			DailyPnL:       1842.56,
			DailyPnLPct:    1.84,
			DailyDrawdown:  -0.73,
			DataLatencyMS:  38,
			LastUpdated:    "14:32:18",
			SelectedSymbol: "BTCUSDT Perpetual",
			SelectedMarket: "BTCUSDT",
			SlippageModel:  "0.01% + 0.5 tick",
			FeeModel:       "Maker 0.02% / Taker 0.05%",
			FundingModel:   "Real-time",
		},
		Runs:      runs,
		Candles:   candles,
		Equity:    equity,
		Benchmark: benchmark,
		Verdict: Verdict{
			Signal:           "BUY",
			Confidence:       78,
			Uncertainty:      "Medium",
			UncertaintyScore: 0.42,
			Regime:           "Bull Trend",
			RiskOverride:     "None",
			TTL:              "02:46",
			ExpiresAt:        "14:35:04",
			Reasoning:        "Price above EMA50 with rising momentum and positive order book imbalance. Funding neutral. Trend regime bullish; expect continuation.",
		},
		Features: []FeatureImpact{
			{Name: "Spread Quality", Value: 0.82, Impact: "positive"},
			{Name: "Liquidity Depth", Value: 0.61, Impact: "positive"},
			{Name: "Momentum", Value: 0.48, Impact: "positive"},
			{Name: "Trend Alignment", Value: 0.19, Impact: "positive"},
			{Name: "Funding Pressure", Value: -0.23, Impact: "negative"},
		},
		Performance: performanceRows(),
		Positions: []Position{
			{Symbol: "BTCUSDT Perp", Side: "Long", Size: "2.000 BTC", Entry: 64512.3, Mark: 68673.2, PnL: 8321.8, PnLPct: 6.44, Risk: "Low", Age: "1d 04:21"},
			{Symbol: "ETHUSDT Perp", Side: "Long", Size: "10.000 ETH", Entry: 3215.45, Mark: 3514.2, PnL: 2987.5, PnLPct: 9.30, Risk: "Low", Age: "06:11"},
		},
		Orders: []Order{
			{Symbol: "BTCUSDT Perp", Side: "Buy", Type: "Limit", Size: "1.000 BTC", Price: 67800, Status: "Open", Created: "14:21:05"},
			{Symbol: "SOLUSDT Perp", Side: "Sell", Type: "Stop", Size: "120 SOL", Price: 164.8, Status: "Trigger pending", Created: "14:12:44"},
			{Symbol: "ETHUSDT Perp", Side: "Buy", Type: "Limit", Size: "2.500 ETH", Price: 3460, Status: "Working", Created: "14:08:20"},
		},
		Events: events,
	}, nil
}

func (s *Store) runs(ctx context.Context) ([]ExperimentRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, version, run, status, return_7d, max_dd, win_rate, last_run FROM experiment_runs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExperimentRun
	for rows.Next() {
		var run ExperimentRun
		if err := rows.Scan(&run.Name, &run.Version, &run.Run, &run.Status, &run.Return7D, &run.MaxDD, &run.WinRate, &run.LastRun); err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *Store) candles(ctx context.Context) ([]Candle, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, open, high, low, close, volume FROM candles ORDER BY ts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Candle
	for rows.Next() {
		var candle Candle
		if err := rows.Scan(&candle.Time, &candle.Open, &candle.High, &candle.Low, &candle.Close, &candle.Volume); err != nil {
			return nil, err
		}
		result = append(result, candle)
	}
	return result, rows.Err()
}

func (s *Store) equity(ctx context.Context) ([]Point, []Point, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, equity, benchmark FROM equity_points ORDER BY ts`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var equity []Point
	var benchmark []Point
	for rows.Next() {
		var ts int64
		var eq float64
		var bm float64
		if err := rows.Scan(&ts, &eq, &bm); err != nil {
			return nil, nil, err
		}
		equity = append(equity, Point{Time: ts, Value: eq})
		benchmark = append(benchmark, Point{Time: ts, Value: bm})
	}
	return equity, benchmark, rows.Err()
}

func (s *Store) events(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_time, type, symbol, action, price, result, note, level FROM lab_events ORDER BY id DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.Time, &event.Type, &event.Symbol, &event.Action, &event.Price, &event.Result, &event.Note, &event.Level); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *Store) seed(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	runs := []ExperimentRun{
		{Name: "AI Momentum Pro", Version: "v2.3.1", Run: "Run 42", Status: "Running", Return7D: 8.42, MaxDD: -4.62, WinRate: 61.3, LastRun: "14:32:18"},
		{Name: "AI Mean Reversion", Version: "v1.8.0", Run: "Run 15", Status: "Paper", Return7D: 3.27, MaxDD: -2.11, WinRate: 55.7, LastRun: "14:28:05"},
		{Name: "AI Breakout Alpha", Version: "v1.5.2", Run: "Run 27", Status: "Shadow", Return7D: 6.91, MaxDD: -5.08, WinRate: 58.9, LastRun: "14:31:40"},
		{Name: "Funding Arbitrage", Version: "v1.2.4", Run: "Run 9", Status: "Stopped", Return7D: 1.12, MaxDD: -1.35, WinRate: 64.2, LastRun: "12:15:33"},
		{Name: "Multi-Factor Trend", Version: "v2.0.0", Run: "Run 33", Status: "Paper", Return7D: 5.38, MaxDD: -3.21, WinRate: 60.8, LastRun: "14:22:11"},
		{Name: "Volatility Breakout", Version: "v1.6.1", Run: "Run 18", Status: "Shadow", Return7D: 2.48, MaxDD: -2.92, WinRate: 53.1, LastRun: "14:20:07"},
		{Name: "Pairs Trading AI", Version: "v1.3.7", Run: "Run 11", Status: "Stopped", Return7D: -0.31, MaxDD: -1.98, WinRate: 48.6, LastRun: "11:05:22"},
		{Name: "Regime Rotation", Version: "v2.1.0", Run: "Run 21", Status: "Shadow", Return7D: 4.02, MaxDD: -4.11, WinRate: 57.3, LastRun: "14:30:44"},
	}
	for _, run := range runs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO experiment_runs (name, version, run, status, return_7d, max_dd, win_rate, last_run) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, run.Name, run.Version, run.Run, run.Status, run.Return7D, run.MaxDD, run.WinRate, run.LastRun); err != nil {
			return err
		}
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	price := 65080.0
	equity := 100120.0
	benchmark := 100020.0
	for i := 0; i < 96; i++ {
		ts := start.Add(time.Duration(i) * 90 * time.Minute).Unix()
		wave := math.Sin(float64(i)/5.2)*140 + math.Cos(float64(i)/9.5)*70
		trend := float64(i) * 24
		open := price
		close := 65120 + trend + wave + math.Sin(float64(i))*38
		high := math.Max(open, close) + 110 + math.Mod(float64(i*37), 45)
		low := math.Min(open, close) - 100 - math.Mod(float64(i*23), 35)
		volume := 900 + math.Abs(math.Sin(float64(i)/3.1))*1800 + math.Mod(float64(i*101), 420)
		price = close
		if _, err := tx.ExecContext(ctx, `INSERT INTO candles (ts, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?)`, ts, open, high, low, close, volume); err != nil {
			return err
		}

		if i%3 == 0 {
			equity += 82 + math.Sin(float64(i)/4)*130
			benchmark += 42 + math.Cos(float64(i)/5)*90
			if _, err := tx.ExecContext(ctx, `INSERT INTO equity_points (ts, equity, benchmark) VALUES (?, ?, ?)`, ts, equity, benchmark); err != nil {
				return err
			}
		}
	}

	events := []Event{
		{Time: "14:32:18", Type: "AI Decision", Symbol: "BTCUSDT", Action: "BUY", Price: 67214.8, Result: "0.512 BTC", Note: "Conf: 78% TTL: 02:46", Level: "info"},
		{Time: "14:32:18", Type: "Risk Check", Symbol: "BTCUSDT", Action: "-", Price: 0, Result: "Approved", Note: "All gates passed", Level: "success"},
		{Time: "14:32:18", Type: "Sim Fill", Symbol: "BTCUSDT", Action: "BUY", Price: 67214.8, Result: "Simulated", Note: "0.512 BTC", Level: "success"},
		{Time: "14:32:18", Type: "Shadow Fill", Symbol: "BTCUSDT", Action: "BUY", Price: 67219.6, Result: "Live shadow", Note: "0.512 BTC", Level: "info"},
		{Time: "14:30:05", Type: "AI Decision", Symbol: "BTCUSDT", Action: "SELL", Price: 67654.3, Result: "Conf: 72%", Note: "TTL: 01:12", Level: "warn"},
		{Time: "14:29:58", Type: "Rejected", Symbol: "BTCUSDT", Action: "BUY", Price: 67890.1, Result: "Risk", Note: "Max drawdown", Level: "danger"},
		{Time: "14:28:41", Type: "AI Decision", Symbol: "ETHUSDT", Action: "BUY", Price: 3512.9, Result: "Conf: 66%", Note: "TTL: 02:01", Level: "info"},
	}
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO lab_events (event_time, type, symbol, action, price, result, note, level) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, event.Time, event.Type, event.Symbol, event.Action, event.Price, event.Result, event.Note, event.Level); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func performanceRows() []MetricRow {
	return []MetricRow{
		{Metric: "Return", SevenDay: "+8.42%", ThirtyDay: "+27.31%", AllTime: "+54.87%", Benchmark7D: "+2.11%", Benchmark30D: "+9.37%", BenchmarkAll: "+18.62%", Trend: "positive"},
		{Metric: "Annualized Return", SevenDay: "-", ThirtyDay: "+332.18%", AllTime: "+186.24%", Benchmark7D: "-", Benchmark30D: "+113.22%", BenchmarkAll: "+65.47%", Trend: "positive"},
		{Metric: "Max Drawdown", SevenDay: "-4.62%", ThirtyDay: "-6.91%", AllTime: "-12.34%", Benchmark7D: "-6.23%", Benchmark30D: "-8.77%", BenchmarkAll: "-15.42%", Trend: "negative"},
		{Metric: "Sharpe Ratio", SevenDay: "2.31", ThirtyDay: "2.84", AllTime: "2.12", Benchmark7D: "0.72", Benchmark30D: "1.01", BenchmarkAll: "0.85", Trend: "neutral"},
		{Metric: "Sortino Ratio", SevenDay: "3.45", ThirtyDay: "4.21", AllTime: "3.35", Benchmark7D: "1.12", Benchmark30D: "1.53", BenchmarkAll: "1.28", Trend: "neutral"},
		{Metric: "Win Rate", SevenDay: "61.30%", ThirtyDay: "59.84%", AllTime: "58.76%", Benchmark7D: "51.24%", Benchmark30D: "53.67%", BenchmarkAll: "52.11%", Trend: "positive"},
		{Metric: "Profit Factor", SevenDay: "1.89", ThirtyDay: "2.14", AllTime: "1.92", Benchmark7D: "1.23", Benchmark30D: "1.38", BenchmarkAll: "1.21", Trend: "positive"},
		{Metric: "Total Trades", SevenDay: "48", ThirtyDay: "213", AllTime: "1,024", Benchmark7D: "48", Benchmark30D: "213", BenchmarkAll: "1,024", Trend: "neutral"},
	}
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func positiveOrDefault(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

type countQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func localDataSummary(ctx context.Context, queryer countQuerier) (LocalDataSummary, error) {
	var summary LocalDataSummary
	counts := []struct {
		query string
		value *int64
	}{
		{`SELECT COUNT(*) FROM backtest_runs`, &summary.BacktestRuns},
		{`SELECT COUNT(*) FROM autopilot_runs`, &summary.AutopilotRuns},
		{`SELECT COUNT(*) FROM autopilot_steps`, &summary.AutopilotSteps},
		{`SELECT COUNT(*) FROM paper_execution_records`, &summary.PaperExecutions},
		{`SELECT COUNT(*) FROM account_snapshots`, &summary.AccountSnapshots},
		{`SELECT COUNT(*) FROM live_execution_records`, &summary.LiveExecutions},
		{`SELECT COUNT(*) FROM live_reconciliation_records`, &summary.LiveReconciliations},
		{`SELECT COUNT(*) FROM audit_log`, &summary.AuditEntries},
		{`SELECT COUNT(*) FROM api_credentials`, &summary.Credentials},
	}
	for _, count := range counts {
		if err := queryer.QueryRowContext(ctx, count.query).Scan(count.value); err != nil {
			return LocalDataSummary{}, err
		}
	}
	return summary, nil
}

func normalizePruneOptions(options LocalDataPruneOptions) LocalDataPruneOptions {
	if options.KeepBacktestRuns <= 0 {
		options.KeepBacktestRuns = 30
	}
	if options.KeepAutopilotRuns <= 0 {
		options.KeepAutopilotRuns = 20
	}
	if options.KeepPaperExecutions <= 0 {
		options.KeepPaperExecutions = 500
	}
	if options.KeepAccountSnapshots <= 0 {
		options.KeepAccountSnapshots = 50
	}
	return options
}

func protectedLocalDataSets() []string {
	return []string{
		"credentials",
		"audit_log",
		"live_execution_records",
		"live_reconciliation_records",
		"risk_profile",
		"strategy_profile",
	}
}

func pruneTableByLatestID(ctx context.Context, tx *sql.Tx, table string, keep int) (int64, error) {
	var statement string
	switch table {
	case "backtest_runs":
		statement = `DELETE FROM backtest_runs WHERE id NOT IN (SELECT id FROM backtest_runs ORDER BY id DESC LIMIT ?)`
	case "autopilot_runs":
		statement = `DELETE FROM autopilot_runs WHERE id NOT IN (SELECT id FROM autopilot_runs ORDER BY id DESC LIMIT ?)`
	case "paper_execution_records":
		statement = `DELETE FROM paper_execution_records WHERE id NOT IN (SELECT id FROM paper_execution_records ORDER BY id DESC LIMIT ?)`
	case "account_snapshots":
		statement = `DELETE FROM account_snapshots WHERE id NOT IN (SELECT id FROM account_snapshots ORDER BY id DESC LIMIT ?)`
	default:
		return 0, errors.New("unsupported prune table")
	}
	result, err := tx.ExecContext(ctx, statement, keep)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func pruneAutopilotSteps(ctx context.Context, tx *sql.Tx, keepRuns int) (int64, error) {
	result, err := tx.ExecContext(
		ctx,
		`DELETE FROM autopilot_steps
		WHERE run_id NOT IN (SELECT id FROM autopilot_runs ORDER BY id DESC LIMIT ?)`,
		keepRuns,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func mergeAuditPayload(payload any, fields map[string]any) map[string]any {
	merged := map[string]any{}
	if existing, ok := payload.(map[string]any); ok {
		for key, value := range existing {
			merged[key] = value
		}
	} else if payload != nil {
		merged["details"] = payload
	}
	for key, value := range fields {
		merged[key] = value
	}
	return merged
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS experiment_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		run TEXT NOT NULL,
		status TEXT NOT NULL,
		return_7d REAL NOT NULL,
		max_dd REAL NOT NULL,
		win_rate REAL NOT NULL,
		last_run TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS candles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts INTEGER NOT NULL,
		open REAL NOT NULL,
		high REAL NOT NULL,
		low REAL NOT NULL,
		close REAL NOT NULL,
		volume REAL NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_candles_ts ON candles (ts);`,
	`CREATE TABLE IF NOT EXISTS equity_points (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts INTEGER NOT NULL,
		equity REAL NOT NULL,
		benchmark REAL NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_equity_ts ON equity_points (ts);`,
	`CREATE TABLE IF NOT EXISTS lab_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_time TEXT NOT NULL,
		type TEXT NOT NULL,
		symbol TEXT NOT NULL,
		action TEXT NOT NULL,
		price REAL NOT NULL,
		result TEXT NOT NULL,
		note TEXT NOT NULL,
		level TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS api_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		exchange TEXT NOT NULL,
		label TEXT NOT NULL,
		api_key_mask TEXT NOT NULL,
		permissions_json TEXT NOT NULL,
		kdf_name TEXT NOT NULL,
		kdf_iterations INTEGER NOT NULL,
		salt BLOB NOT NULL,
		nonce BLOB NOT NULL,
		ciphertext BLOB NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_api_credentials_exchange ON api_credentials (exchange);`,
	`CREATE TABLE IF NOT EXISTS account_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		credential_id INTEGER NOT NULL,
		exchange TEXT NOT NULL,
		environment TEXT NOT NULL,
		symbol TEXT NOT NULL,
		balance_count INTEGER NOT NULL,
		open_order_count INTEGER NOT NULL,
		snapshot_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_account_snapshots_latest ON account_snapshots (credential_id, exchange, environment, symbol, id);`,
	`CREATE TABLE IF NOT EXISTS live_execution_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		intent_id TEXT NOT NULL,
		client_order_id TEXT NOT NULL,
		exchange_order_id TEXT NOT NULL,
		credential_id INTEGER NOT NULL,
		exchange TEXT NOT NULL,
		environment TEXT NOT NULL,
		symbol TEXT NOT NULL,
		side TEXT NOT NULL,
		size_usdt REAL NOT NULL,
		validation_only INTEGER NOT NULL,
		risk_status TEXT NOT NULL,
		execution_status TEXT NOT NULL,
		intent_json TEXT NOT NULL,
		decision_json TEXT NOT NULL,
		execution_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_live_execution_records_latest ON live_execution_records (id, exchange, environment, symbol);`,
	`CREATE INDEX IF NOT EXISTS idx_live_execution_records_intent ON live_execution_records (intent_id);`,
	`CREATE TABLE IF NOT EXISTS paper_execution_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL,
		mode TEXT NOT NULL,
		source TEXT NOT NULL,
		intent_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
		side TEXT NOT NULL,
		size_usdt REAL NOT NULL,
		intent_price REAL NOT NULL,
		confidence REAL NOT NULL,
		risk_status TEXT NOT NULL,
		fill_status TEXT NOT NULL,
		fill_price REAL NOT NULL,
		fee_usdt REAL NOT NULL,
		slippage_usdt REAL NOT NULL,
		intent_json TEXT NOT NULL,
		decision_json TEXT NOT NULL,
		fill_json TEXT NOT NULL,
		events_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_paper_execution_records_latest ON paper_execution_records (id, mode, symbol);`,
	`CREATE INDEX IF NOT EXISTS idx_paper_execution_records_run ON paper_execution_records (run_id, id);`,
	`CREATE TABLE IF NOT EXISTS live_reconciliation_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		live_execution_id INTEGER NOT NULL,
		credential_id INTEGER NOT NULL,
		exchange TEXT NOT NULL,
		environment TEXT NOT NULL,
		symbol TEXT NOT NULL,
		client_order_id TEXT NOT NULL,
		exchange_order_id TEXT NOT NULL,
		status TEXT NOT NULL,
		filled_usdt REAL NOT NULL,
		report_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_live_reconciliation_records_latest ON live_reconciliation_records (live_execution_id, id);`,
	`CREATE TABLE IF NOT EXISTS risk_profiles (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		name TEXT NOT NULL,
		min_confidence REAL NOT NULL,
		max_order_usdt REAL NOT NULL,
		max_symbol_exposure_usdt REAL NOT NULL,
		max_total_exposure_usdt REAL NOT NULL,
		max_daily_drawdown_pct REAL NOT NULL,
		max_consecutive_losses INTEGER NOT NULL,
		max_spread_pct REAL NOT NULL,
		require_live_unlock INTEGER NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS strategy_profiles (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		name TEXT NOT NULL,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
		side TEXT NOT NULL,
		order_size_usdt REAL NOT NULL,
		interval_seconds INTEGER NOT NULL,
		max_steps INTEGER NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS autopilot_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mode TEXT NOT NULL,
		exchange TEXT NOT NULL,
		environment TEXT NOT NULL,
		symbol TEXT NOT NULL,
		operator TEXT NOT NULL,
		interval_seconds INTEGER NOT NULL,
		max_steps INTEGER NOT NULL,
		credential_id INTEGER NOT NULL,
		side TEXT NOT NULL,
		size_usdt REAL NOT NULL,
		validation_only INTEGER NOT NULL,
		status TEXT NOT NULL,
		completed_steps INTEGER NOT NULL,
		last_error TEXT NOT NULL,
		started_at TEXT NOT NULL,
		stopped_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_autopilot_runs_latest ON autopilot_runs (id, status);`,
	`CREATE TABLE IF NOT EXISTS autopilot_steps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL,
		step_number INTEGER NOT NULL,
		status TEXT NOT NULL,
		error TEXT NOT NULL,
		result_json TEXT NOT NULL,
		events_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_autopilot_steps_run ON autopilot_steps (run_id, id);`,
	`CREATE TABLE IF NOT EXISTS backtest_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		strategy_name TEXT NOT NULL,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
		interval TEXT NOT NULL,
		market_data_source TEXT NOT NULL,
		candle_count INTEGER NOT NULL,
		trade_count INTEGER NOT NULL,
		ending_equity_usdt REAL NOT NULL,
		return_pct REAL NOT NULL,
		benchmark_return_pct REAL NOT NULL,
		max_drawdown_pct REAL NOT NULL,
		fees_usdt REAL NOT NULL,
		result_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_backtest_runs_latest ON backtest_runs (id, exchange, symbol);`,
	`CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TEXT NOT NULL,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		entity TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		status TEXT NOT NULL,
		summary TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		prev_hash TEXT NOT NULL,
		hash TEXT NOT NULL UNIQUE
	);`,
	`CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log (created_at);`,
}
