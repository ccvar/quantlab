package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/audit"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/liveexec"
	"ccvar.com/web3quant/internal/livesync"
	"ccvar.com/web3quant/internal/simrun"
	"ccvar.com/web3quant/internal/storage"
)

var (
	ErrAlreadyRunning       = errors.New("autopilot is already running")
	ErrNotRunning           = errors.New("autopilot is not running")
	ErrKillSwitchActive     = errors.New("kill switch is active")
	ErrUnsupportedMode      = errors.New("autopilot mode must be shadow, paper, or live")
	ErrLiveCredentialNeeded = errors.New("live autopilot requires credential id and vault passphrase")
)

const (
	ModeShadow = "shadow"
	ModePaper  = "paper"
	ModeLive   = "live"
)

type SimStepFunc func(context.Context, string, string) (simrun.Result, error)
type AccountSyncFunc func(context.Context, livesync.Request) (livesync.Result, error)
type LiveExecuteFunc func(context.Context, liveexec.Request) (liveexec.Result, error)
type LivePlannerFunc func(context.Context, LivePlanRequest) (LivePlan, error)

type Service struct {
	mu          sync.Mutex
	store       *storage.Store
	simStep     SimStepFunc
	accountSync AccountSyncFunc
	liveExecute LiveExecuteFunc
	livePlanner LivePlannerFunc
	halted      func() bool
	now         func() time.Time

	state   State
	runtime *runtimeConfig
	cancel  context.CancelFunc
}

type Request struct {
	Action          string  `json:"action"`
	Operator        string  `json:"operator"`
	Mode            string  `json:"mode"`
	Exchange        string  `json:"exchange"`
	Environment     string  `json:"environment"`
	Symbol          string  `json:"symbol"`
	IntervalSeconds int     `json:"intervalSeconds"`
	MaxSteps        int     `json:"maxSteps"`
	CredentialID    int64   `json:"credentialId"`
	Passphrase      string  `json:"passphrase"`
	Side            string  `json:"side"`
	SizeUSDT        float64 `json:"sizeUsdt"`
	ValidationOnly  bool    `json:"validationOnly"`
	Reason          string  `json:"reason"`
}

type State struct {
	Running         bool            `json:"running"`
	RunID           int64           `json:"runId,omitempty"`
	Mode            string          `json:"mode"`
	Exchange        string          `json:"exchange"`
	Environment     string          `json:"environment,omitempty"`
	Symbol          string          `json:"symbol"`
	IntervalSeconds int             `json:"intervalSeconds"`
	MaxSteps        int             `json:"maxSteps"`
	CompletedSteps  int             `json:"completedSteps"`
	CredentialID    int64           `json:"credentialId,omitempty"`
	Side            string          `json:"side,omitempty"`
	SizeUSDT        float64         `json:"sizeUsdt,omitempty"`
	ValidationOnly  bool            `json:"validationOnly"`
	StartedAt       string          `json:"startedAt,omitempty"`
	StoppedAt       string          `json:"stoppedAt,omitempty"`
	LastRunAt       string          `json:"lastRunAt,omitempty"`
	NextRunAt       string          `json:"nextRunAt,omitempty"`
	LastStatus      string          `json:"lastStatus"`
	LastError       string          `json:"lastError,omitempty"`
	LastEvents      []storage.Event `json:"lastEvents,omitempty"`
	LastResult      json.RawMessage `json:"lastResult,omitempty"`
	Message         string          `json:"message"`
}

type runtimeConfig struct {
	State
	operator   string
	passphrase string
	reason     string
}

type LivePlanRequest struct {
	Operator    string          `json:"operator"`
	Exchange    string          `json:"exchange"`
	Environment string          `json:"environment"`
	Symbol      string          `json:"symbol"`
	Side        string          `json:"side"`
	SizeUSDT    float64         `json:"sizeUsdt"`
	AccountSync livesync.Result `json:"accountSync"`
}

type LivePlan struct {
	Intent core.TradeIntent `json:"-"`
	AI     ai.Trace         `json:"ai"`
	Events []storage.Event  `json:"events,omitempty"`
}

type LivePlanView struct {
	Intent LivePlanIntentView `json:"intent"`
	AI     ai.Trace           `json:"ai"`
}

type LivePlanIntentView struct {
	ID          string  `json:"id"`
	Exchange    string  `json:"exchange"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	OrderType   string  `json:"orderType"`
	Price       float64 `json:"price"`
	SizeUSDT    float64 `json:"sizeUsdt"`
	Confidence  float64 `json:"confidence"`
	TTLSeconds  int64   `json:"ttlSeconds"`
	GeneratedAt string  `json:"generatedAt"`
	Reason      string  `json:"reason"`
}

func New(store *storage.Store, simStep SimStepFunc, accountSync AccountSyncFunc, liveExecute LiveExecuteFunc, halted func() bool) *Service {
	return &Service{
		store:       store,
		simStep:     simStep,
		accountSync: accountSync,
		liveExecute: liveExecute,
		halted:      halted,
		now:         time.Now,
		state: State{
			Mode:            ModeShadow,
			Exchange:        "Binance",
			Environment:     "",
			Symbol:          "BTCUSDT",
			IntervalSeconds: 15,
			LastStatus:      "idle",
			Message:         "autopilot idle",
		},
	}
}

func (service *Service) WithClock(now func() time.Time) *Service {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.now = now
	return service
}

func (service *Service) WithLivePlanner(planner LivePlannerFunc) *Service {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.livePlanner = planner
	return service
}

func (service *Service) State() State {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.state
}

func (service *Service) Start(ctx context.Context, request Request) (State, error) {
	config, err := service.runtimeFromRequest(request)
	if err != nil {
		return service.State(), err
	}
	if service.isHalted() {
		_ = service.audit(ctx, audit.Record{
			Actor:   config.operator,
			Action:  "autopilot.start",
			Entity:  "autopilot",
			Status:  "rejected",
			Summary: ErrKillSwitchActive.Error(),
			Payload: service.safePayload(config.State, ErrKillSwitchActive.Error()),
		})
		return service.State(), ErrKillSwitchActive
	}

	service.mu.Lock()
	if service.state.Running {
		service.mu.Unlock()
		return service.state, ErrAlreadyRunning
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	now := service.clockLocked()
	config.StartedAt = now.Format(time.RFC3339)
	config.StoppedAt = ""
	config.LastRunAt = ""
	config.NextRunAt = ""
	config.LastStatus = "starting"
	config.LastError = ""
	config.Message = "autopilot starting"
	config.CompletedSteps = 0
	config.Running = true
	if service.store != nil {
		run, err := service.store.CreateAutopilotRun(ctx, runRecordFromState(config.State, config.operator))
		if err != nil {
			cancel()
			service.mu.Unlock()
			return service.state, err
		}
		config.RunID = run.ID
	}
	service.runtime = config
	service.cancel = cancel
	service.state = config.State
	service.mu.Unlock()

	_ = service.audit(ctx, audit.Record{
		Actor:    config.operator,
		Action:   "autopilot.start",
		Entity:   "autopilot",
		EntityID: fmt.Sprint(config.RunID),
		Status:   "approved",
		Summary:  fmt.Sprintf("%s autopilot started", config.Mode),
		Payload:  service.safePayload(config.State, ""),
	})

	state, stepErr := service.Step(ctx)
	if stepErr != nil && service.State().Running {
		return state, stepErr
	}
	if service.State().Running {
		go service.loop(loopCtx)
	}
	return service.State(), stepErr
}

func (service *Service) Stop(ctx context.Context, operator, reason string) State {
	return service.stop(ctx, operator, reason, "stopped", "autopilot stopped")
}

func (service *Service) Step(ctx context.Context) (State, error) {
	service.mu.Lock()
	config := service.runtime
	if config == nil || !service.state.Running {
		state := service.state
		service.mu.Unlock()
		return state, ErrNotRunning
	}
	service.mu.Unlock()

	if service.isHalted() {
		return service.stop(ctx, config.operator, "kill switch active", "halted", ErrKillSwitchActive.Error()), ErrKillSwitchActive
	}

	now := service.clock()
	result, events, err := service.runOnce(ctx, config)
	payload, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		payload = nil
		if err == nil {
			err = marshalErr
		}
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.runtime != config || !service.state.Running {
		return service.state, err
	}
	service.state.CompletedSteps++
	service.state.LastRunAt = now.Format(time.RFC3339)
	service.state.NextRunAt = now.Add(time.Duration(config.IntervalSeconds) * time.Second).Format(time.RFC3339)
	service.state.LastEvents = events
	service.state.LastResult = payload
	service.state.LastError = ""
	service.state.LastStatus = "ok"
	service.state.Message = "autopilot step complete"
	if err != nil {
		service.state.LastStatus = "failed"
		service.state.LastError = err.Error()
		service.state.Message = err.Error()
	}
	if persistErr := service.persistStepLocked(ctx, payload, events); persistErr != nil && err == nil {
		err = persistErr
		service.state.LastStatus = "failed"
		service.state.LastError = persistErr.Error()
		service.state.Message = persistErr.Error()
	}
	status := "approved"
	if err != nil {
		status = "failed"
	}
	_ = service.audit(ctx, audit.Record{
		Actor:    config.operator,
		Action:   "autopilot.step",
		Entity:   "autopilot",
		EntityID: fmt.Sprint(service.state.RunID),
		Status:   status,
		Summary:  service.state.Message,
		Payload:  service.safePayload(service.state, service.state.LastError),
	})
	if err == nil && service.state.MaxSteps > 0 && service.state.CompletedSteps >= service.state.MaxSteps {
		service.stopLocked(now, "max steps reached", "completed", "autopilot completed max steps")
	}
	_ = service.persistRunLocked(ctx, config.operator)
	return service.state, err
}

func (service *Service) runOnce(ctx context.Context, config *runtimeConfig) (any, []storage.Event, error) {
	switch config.Mode {
	case ModeShadow, ModePaper:
		if service.simStep == nil {
			return nil, nil, errors.New("simulation runner is not configured")
		}
		result, err := service.simStep(ctx, config.Exchange, config.Symbol)
		if err != nil {
			return result, result.Events, err
		}
		if service.store != nil {
			record, err := simrun.PaperExecutionRecordFromResult(result, config.Mode, "autopilot", config.RunID, service.clock())
			if err != nil {
				return result, result.Events, err
			}
			if _, err := service.store.SavePaperExecution(ctx, record); err != nil {
				return result, result.Events, err
			}
		}
		return result, result.Events, err
	case ModeLive:
		if service.accountSync == nil || service.liveExecute == nil {
			return nil, nil, errors.New("live executor is not configured")
		}
		syncResult, err := service.accountSync(ctx, livesync.Request{
			Operator:     config.operator,
			CredentialID: config.CredentialID,
			Passphrase:   config.passphrase,
			Exchange:     config.Exchange,
			Environment:  config.Environment,
			Symbol:       config.Symbol,
		})
		if err != nil {
			return map[string]any{"accountSync": syncResult}, nil, err
		}
		executeRequest := liveexec.Request{
			Operator:       config.operator,
			CredentialID:   config.CredentialID,
			Passphrase:     config.passphrase,
			Exchange:       config.Exchange,
			Symbol:         config.Symbol,
			Side:           config.Side,
			SizeUSDT:       config.SizeUSDT,
			ValidationOnly: config.ValidationOnly,
		}
		var plan LivePlan
		var planView *LivePlanView
		if service.livePlanner != nil {
			var planErr error
			plan, planErr = service.livePlanner(ctx, LivePlanRequest{
				Operator:    config.operator,
				Exchange:    config.Exchange,
				Environment: config.Environment,
				Symbol:      config.Symbol,
				Side:        config.Side,
				SizeUSDT:    config.SizeUSDT,
				AccountSync: syncResult,
			})
			if planErr != nil {
				return map[string]any{"accountSync": syncResult, "aiPlan": safeLivePlan(plan)}, plan.Events, planErr
			}
			planViewValue := safeLivePlan(plan)
			planView = &planViewValue
			executeRequest.Side = string(plan.Intent.Side)
			executeRequest.SizeUSDT = plan.Intent.SizeUSDT
			executeRequest.PlannedIntent = &plan.Intent
			executeRequest.AITrace = &plan.AI
		}
		result, err := service.liveExecute(ctx, executeRequest)
		events := append([]storage.Event{}, plan.Events...)
		events = append(events, result.Events...)
		payload := map[string]any{"accountSync": syncResult, "execution": result}
		if planView != nil {
			payload["aiPlan"] = *planView
		}
		return payload, events, err
	default:
		return nil, nil, ErrUnsupportedMode
	}
}

func (service *Service) loop(ctx context.Context) {
	for {
		service.mu.Lock()
		if service.runtime == nil || !service.state.Running {
			service.mu.Unlock()
			return
		}
		interval := time.Duration(service.runtime.IntervalSeconds) * time.Second
		service.mu.Unlock()

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			_, _ = service.Step(context.Background())
		}
	}
}

func (service *Service) stop(ctx context.Context, operator, reason, status, message string) State {
	service.mu.Lock()
	now := service.clockLocked()
	if service.runtime != nil && strings.TrimSpace(operator) == "" {
		operator = service.runtime.operator
	}
	state := service.stopLocked(now, reason, status, message)
	_ = service.persistRunLocked(ctx, defaultString(operator, "local"))
	service.mu.Unlock()
	_ = service.audit(ctx, audit.Record{
		Actor:    defaultString(operator, "local"),
		Action:   "autopilot.stop",
		Entity:   "autopilot",
		EntityID: fmt.Sprint(state.RunID),
		Status:   status,
		Summary:  message,
		Payload:  service.safePayload(state, reason),
	})
	return state
}

func (service *Service) stopLocked(now time.Time, reason, status, message string) State {
	if service.cancel != nil {
		service.cancel()
	}
	service.cancel = nil
	service.runtime = nil
	service.state.Running = false
	service.state.StoppedAt = now.UTC().Format(time.RFC3339)
	service.state.NextRunAt = ""
	service.state.LastStatus = status
	service.state.LastError = ""
	if status == "halted" || status == "failed" {
		service.state.LastError = reason
	}
	service.state.Message = message
	return service.state
}

func (service *Service) History(ctx context.Context, limit int) ([]storage.AutopilotRunRecord, error) {
	if service.store == nil {
		return []storage.AutopilotRunRecord{}, nil
	}
	return service.store.ListAutopilotRuns(ctx, limit)
}

func (service *Service) Steps(ctx context.Context, runID int64, limit int) ([]storage.AutopilotStepRecord, error) {
	if service.store == nil {
		return []storage.AutopilotStepRecord{}, nil
	}
	return service.store.ListAutopilotSteps(ctx, runID, limit)
}

func (service *Service) persistStepLocked(ctx context.Context, resultJSON json.RawMessage, events []storage.Event) error {
	if service.store == nil || service.state.RunID <= 0 {
		return nil
	}
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return err
	}
	returnStatus := service.state.LastStatus
	if returnStatus == "" {
		returnStatus = "ok"
	}
	_, err = service.store.SaveAutopilotStep(ctx, storage.AutopilotStepRecord{
		RunID:      service.state.RunID,
		StepNumber: service.state.CompletedSteps,
		Status:     returnStatus,
		Error:      service.state.LastError,
		ResultJSON: resultJSON,
		EventsJSON: eventsJSON,
		CreatedAt:  service.state.LastRunAt,
	})
	return err
}

func (service *Service) persistRunLocked(ctx context.Context, operator string) error {
	if service.store == nil || service.state.RunID <= 0 {
		return nil
	}
	_, err := service.store.UpdateAutopilotRun(ctx, runRecordFromState(service.state, operator))
	return err
}

func (service *Service) runtimeFromRequest(request Request) (*runtimeConfig, error) {
	mode := normalizeMode(defaultString(request.Mode, ModeShadow))
	if mode == "" {
		return nil, ErrUnsupportedMode
	}
	interval := request.IntervalSeconds
	if interval == 0 {
		interval = 15
	}
	if interval < 5 {
		interval = 5
	}
	if interval > 3600 {
		interval = 3600
	}
	exchangeName := canonicalExchange(defaultString(request.Exchange, "Binance"))
	environment := strings.ToLower(strings.TrimSpace(request.Environment))
	symbol := strings.ToUpper(defaultString(request.Symbol, "BTCUSDT"))
	side := strings.ToLower(defaultString(request.Side, "buy"))
	if side != "sell" {
		side = "buy"
	}
	size := request.SizeUSDT
	if size <= 0 {
		size = 100
	}
	config := &runtimeConfig{
		State: State{
			Mode:            mode,
			Exchange:        exchangeName,
			Environment:     environment,
			Symbol:          symbol,
			IntervalSeconds: interval,
			MaxSteps:        request.MaxSteps,
			CredentialID:    request.CredentialID,
			Side:            side,
			SizeUSDT:        size,
			ValidationOnly:  true,
		},
		operator:   defaultString(request.Operator, "local"),
		passphrase: strings.TrimSpace(request.Passphrase),
		reason:     strings.TrimSpace(request.Reason),
	}
	if mode == ModeLive {
		if config.Environment == "" {
			config.Environment = "testnet"
		}
		config.ValidationOnly = request.ValidationOnly
		if config.CredentialID <= 0 || config.passphrase == "" {
			return nil, ErrLiveCredentialNeeded
		}
	}
	return config, nil
}

func (service *Service) safePayload(state State, errorText string) map[string]any {
	return map[string]any{
		"runId":           state.RunID,
		"mode":            state.Mode,
		"exchange":        state.Exchange,
		"environment":     state.Environment,
		"symbol":          state.Symbol,
		"intervalSeconds": state.IntervalSeconds,
		"maxSteps":        state.MaxSteps,
		"completedSteps":  state.CompletedSteps,
		"credentialId":    state.CredentialID,
		"side":            state.Side,
		"sizeUsdt":        state.SizeUSDT,
		"validationOnly":  state.ValidationOnly,
		"running":         state.Running,
		"lastStatus":      state.LastStatus,
		"error":           errorText,
	}
}

func safeLivePlan(plan LivePlan) LivePlanView {
	return LivePlanView{
		Intent: LivePlanIntentView{
			ID:          plan.Intent.ID,
			Exchange:    plan.Intent.Exchange,
			Symbol:      plan.Intent.Symbol,
			Side:        string(plan.Intent.Side),
			OrderType:   string(plan.Intent.OrderType),
			Price:       plan.Intent.Price,
			SizeUSDT:    plan.Intent.SizeUSDT,
			Confidence:  plan.Intent.Confidence,
			TTLSeconds:  int64(plan.Intent.TTL.Seconds()),
			GeneratedAt: plan.Intent.GeneratedAt.Format(time.RFC3339),
			Reason:      plan.Intent.Reason,
		},
		AI: plan.AI,
	}
}

func runRecordFromState(state State, operator string) storage.AutopilotRunRecord {
	status := defaultString(state.LastStatus, "idle")
	if state.Running {
		switch status {
		case "idle", "starting", "ok":
			status = "running"
		}
	}
	return storage.AutopilotRunRecord{
		ID:              state.RunID,
		Mode:            state.Mode,
		Exchange:        state.Exchange,
		Environment:     state.Environment,
		Symbol:          state.Symbol,
		Operator:        defaultString(operator, "local"),
		IntervalSeconds: state.IntervalSeconds,
		MaxSteps:        state.MaxSteps,
		CredentialID:    state.CredentialID,
		Side:            state.Side,
		SizeUSDT:        state.SizeUSDT,
		ValidationOnly:  state.ValidationOnly,
		Status:          status,
		CompletedSteps:  state.CompletedSteps,
		LastError:       state.LastError,
		StartedAt:       state.StartedAt,
		StoppedAt:       state.StoppedAt,
		UpdatedAt:       defaultString(state.LastRunAt, state.StartedAt),
	}
}

func (service *Service) audit(ctx context.Context, record audit.Record) error {
	if service.store == nil {
		return nil
	}
	_, err := service.store.AppendAudit(ctx, record)
	return err
}

func (service *Service) isHalted() bool {
	if service.halted == nil {
		return false
	}
	return service.halted()
}

func (service *Service) clock() time.Time {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.clockLocked()
}

func (service *Service) clockLocked() time.Time {
	if service.now == nil {
		return time.Now().UTC()
	}
	return service.now().UTC()
}

func normalizeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModeShadow, "sim", "simulation":
		return ModeShadow
	case ModePaper:
		return ModePaper
	case ModeLive:
		return ModeLive
	default:
		return ""
	}
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
