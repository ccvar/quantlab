package liveguard

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const UnlockPhrase = "ENABLE TESTNET LIVE"

var (
	ErrInvalidPhrase      = errors.New("unlock phrase must be ENABLE TESTNET LIVE")
	ErrProductionDisabled = errors.New("production live trading is disabled")
	ErrInvalidTTL         = errors.New("unlock ttl must be between 60 and 900 seconds")
)

type Guard struct {
	mu    sync.Mutex
	state State
	now   func() time.Time
}

type UnlockRequest struct {
	Operator     string  `json:"operator"`
	Environment  string  `json:"environment"`
	Phrase       string  `json:"phrase"`
	TTLSeconds   int     `json:"ttlSeconds"`
	MaxOrderUSDT float64 `json:"maxOrderUsdt"`
	Reason       string  `json:"reason"`
}

type State struct {
	Unlocked         bool    `json:"unlocked"`
	Environment      string  `json:"environment"`
	SessionID        string  `json:"sessionId"`
	Operator         string  `json:"operator"`
	Reason           string  `json:"reason"`
	ExpiresAt        string  `json:"expiresAt"`
	RemainingSeconds int64   `json:"remainingSeconds"`
	MaxOrderUSDT     float64 `json:"maxOrderUsdt"`
	Message          string  `json:"message"`
}

func New() *Guard {
	return &Guard{
		now: time.Now,
	}
}

func (guard *Guard) WithClock(now func() time.Time) *Guard {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.now = now
	return guard
}

func (guard *Guard) State() State {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	return guard.currentStateLocked()
}

func (guard *Guard) Unlock(request UnlockRequest) (State, error) {
	guard.mu.Lock()
	defer guard.mu.Unlock()

	environment := normalizeEnvironment(request.Environment)
	if environment != "testnet" && environment != "demo" {
		return guard.currentStateLocked(), ErrProductionDisabled
	}
	if strings.TrimSpace(request.Phrase) != UnlockPhrase {
		return guard.currentStateLocked(), ErrInvalidPhrase
	}
	ttl := request.TTLSeconds
	if ttl == 0 {
		ttl = 600
	}
	if ttl < 60 || ttl > 900 {
		return guard.currentStateLocked(), ErrInvalidTTL
	}
	sessionID, err := newSessionID()
	if err != nil {
		return guard.currentStateLocked(), err
	}
	now := guard.clock()
	guard.state = State{
		Unlocked:     true,
		Environment:  environment,
		SessionID:    sessionID,
		Operator:     defaultString(request.Operator, "local"),
		Reason:       strings.TrimSpace(request.Reason),
		ExpiresAt:    now.Add(time.Duration(ttl) * time.Second).UTC().Format(time.RFC3339),
		MaxOrderUSDT: positiveOrDefault(request.MaxOrderUSDT, 1000),
		Message:      "testnet live unlock active",
	}
	return guard.currentStateLocked(), nil
}

func (guard *Guard) Lock() State {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.state = State{Message: "live trading locked"}
	return guard.state
}

func (guard *Guard) currentStateLocked() State {
	state := guard.state
	if !state.Unlocked {
		if state.Message == "" {
			state.Message = "live trading locked"
		}
		return state
	}
	expiresAt, err := time.Parse(time.RFC3339, state.ExpiresAt)
	if err != nil || !guard.clock().Before(expiresAt) {
		guard.state = State{Message: "unlock expired"}
		return guard.state
	}
	state.RemainingSeconds = int64(time.Until(expiresAt).Seconds())
	if guard.now != nil {
		state.RemainingSeconds = int64(expiresAt.Sub(guard.clock()).Seconds())
	}
	if state.RemainingSeconds < 0 {
		state.RemainingSeconds = 0
	}
	return state
}

func (guard *Guard) clock() time.Time {
	if guard.now == nil {
		return time.Now().UTC()
	}
	return guard.now().UTC()
}

func normalizeEnvironment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "test", "testnet", "binance-testnet":
		return "testnet"
	case "sim", "demo", "paper", "okx-demo":
		return "demo"
	default:
		return value
	}
}

func positiveOrDefault(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func newSessionID() (string, error) {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
