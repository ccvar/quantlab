package killswitch

import (
	"testing"
	"time"
)

func TestSwitchActivateAndResume(t *testing.T) {
	now := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	killSwitch := New().WithClock(func() time.Time { return now })

	if state := killSwitch.State(); state.Active || state.Message != "kill switch clear" {
		t.Fatalf("initial state = %#v", state)
	}

	active := killSwitch.Activate(Request{Operator: "qa", Reason: "stop all"})
	if !active.Active || active.Operator != "qa" || active.ActivatedAt != "2026-06-22T09:00:00Z" {
		t.Fatalf("active state = %#v", active)
	}
	if !killSwitch.Active() {
		t.Fatal("Active() = false, want true")
	}

	now = now.Add(time.Minute)
	clear := killSwitch.Resume(Request{Operator: "qa", Reason: "manual resume"})
	if clear.Active || clear.ResumedAt != "2026-06-22T09:01:00Z" || clear.Message != "kill switch clear" {
		t.Fatalf("clear state = %#v", clear)
	}
}
