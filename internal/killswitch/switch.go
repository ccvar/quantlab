package killswitch

import (
	"strings"
	"sync"
	"time"
)

type Request struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type State struct {
	Active      bool   `json:"active"`
	Operator    string `json:"operator"`
	Reason      string `json:"reason"`
	ActivatedAt string `json:"activatedAt,omitempty"`
	ResumedAt   string `json:"resumedAt,omitempty"`
	Message     string `json:"message"`
}

type Switch struct {
	mu    sync.Mutex
	state State
	now   func() time.Time
}

func New() *Switch {
	return &Switch{now: time.Now}
}

func (killSwitch *Switch) WithClock(now func() time.Time) *Switch {
	killSwitch.mu.Lock()
	defer killSwitch.mu.Unlock()
	killSwitch.now = now
	return killSwitch
}

func (killSwitch *Switch) State() State {
	killSwitch.mu.Lock()
	defer killSwitch.mu.Unlock()
	return killSwitch.stateWithMessageLocked()
}

func (killSwitch *Switch) Active() bool {
	killSwitch.mu.Lock()
	defer killSwitch.mu.Unlock()
	return killSwitch.state.Active
}

func (killSwitch *Switch) Activate(request Request) State {
	killSwitch.mu.Lock()
	defer killSwitch.mu.Unlock()
	killSwitch.state = State{
		Active:      true,
		Operator:    defaultString(request.Operator, "local"),
		Reason:      strings.TrimSpace(request.Reason),
		ActivatedAt: killSwitch.clock().UTC().Format(time.RFC3339),
		Message:     "kill switch active: all AI execution is halted",
	}
	return killSwitch.state
}

func (killSwitch *Switch) Resume(request Request) State {
	killSwitch.mu.Lock()
	defer killSwitch.mu.Unlock()
	killSwitch.state = State{
		Active:    false,
		Operator:  defaultString(request.Operator, "local"),
		Reason:    strings.TrimSpace(request.Reason),
		ResumedAt: killSwitch.clock().UTC().Format(time.RFC3339),
		Message:   "kill switch clear",
	}
	return killSwitch.state
}

func (killSwitch *Switch) stateWithMessageLocked() State {
	state := killSwitch.state
	if state.Message == "" {
		if state.Active {
			state.Message = "kill switch active: all AI execution is halted"
		} else {
			state.Message = "kill switch clear"
		}
	}
	return state
}

func (killSwitch *Switch) clock() time.Time {
	if killSwitch.now == nil {
		return time.Now().UTC()
	}
	return killSwitch.now().UTC()
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
