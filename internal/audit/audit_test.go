package audit

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEntryBuildsHashChain(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	first, err := NewEntry(Record{
		Actor:   "local",
		Action:  "live.unlock",
		Entity:  "live_guard",
		Status:  "approved",
		Summary: "unlock testnet",
		Payload: map[string]any{"environment": "testnet"},
	}, "", now)
	if err != nil {
		t.Fatalf("NewEntry() error = %v", err)
	}
	second, err := NewEntry(Record{
		Action:  "live.lock",
		Entity:  "live_guard",
		Status:  "approved",
		Summary: "manual lock",
	}, first.Hash, now.Add(time.Second))
	if err != nil {
		t.Fatalf("NewEntry() second error = %v", err)
	}
	if second.PrevHash != first.Hash {
		t.Fatalf("PrevHash = %q, want %q", second.PrevHash, first.Hash)
	}
	if first.Hash == second.Hash {
		t.Fatal("hash did not change across entries")
	}
	if Hash(first) != first.Hash {
		t.Fatal("hash is not reproducible")
	}
}

func TestNewEntryAcceptsRawJSONPayload(t *testing.T) {
	entry, err := NewEntry(Record{
		Action:  "order.test",
		Entity:  "order",
		Status:  "approved",
		Payload: json.RawMessage(`{"id":"abc"}`),
	}, "", time.Time{})
	if err != nil {
		t.Fatalf("NewEntry() error = %v", err)
	}
	if string(entry.Payload) != `{"id":"abc"}` {
		t.Fatalf("payload = %s", entry.Payload)
	}
}
