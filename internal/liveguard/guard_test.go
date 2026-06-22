package liveguard

import (
	"errors"
	"testing"
	"time"
)

func TestGuardUnlocksOnlyTestnetWithPhrase(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	guard := New().WithClock(func() time.Time { return now })

	state, err := guard.Unlock(UnlockRequest{
		Operator:    "alice",
		Environment: "testnet",
		Phrase:      UnlockPhrase,
		TTLSeconds:  120,
		Reason:      "qa",
	})
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	if !state.Unlocked || state.Environment != "testnet" || state.Operator != "alice" {
		t.Fatalf("state = %#v", state)
	}
	if state.RemainingSeconds != 120 {
		t.Fatalf("RemainingSeconds = %d, want 120", state.RemainingSeconds)
	}
}

func TestGuardRejectsProduction(t *testing.T) {
	_, err := New().Unlock(UnlockRequest{
		Environment: "production",
		Phrase:      UnlockPhrase,
		TTLSeconds:  120,
	})
	if !errors.Is(err, ErrProductionDisabled) {
		t.Fatalf("error = %v, want ErrProductionDisabled", err)
	}
}

func TestGuardRejectsBadPhrase(t *testing.T) {
	_, err := New().Unlock(UnlockRequest{
		Environment: "testnet",
		Phrase:      "please",
		TTLSeconds:  120,
	})
	if !errors.Is(err, ErrInvalidPhrase) {
		t.Fatalf("error = %v, want ErrInvalidPhrase", err)
	}
}

func TestGuardExpires(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	guard := New().WithClock(func() time.Time { return now })
	if _, err := guard.Unlock(UnlockRequest{Environment: "demo", Phrase: UnlockPhrase, TTLSeconds: 60}); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	now = now.Add(61 * time.Second)
	state := guard.State()
	if state.Unlocked {
		t.Fatalf("state still unlocked: %#v", state)
	}
	if state.Message != "unlock expired" {
		t.Fatalf("Message = %q", state.Message)
	}
}
