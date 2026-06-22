package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidPayload = errors.New("audit payload must be valid json")

type Record struct {
	Actor    string `json:"actor"`
	Action   string `json:"action"`
	Entity   string `json:"entity"`
	EntityID string `json:"entityId"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	Payload  any    `json:"payload,omitempty"`
}

type Entry struct {
	ID        int64           `json:"id"`
	CreatedAt string          `json:"createdAt"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  string          `json:"entityId"`
	Status    string          `json:"status"`
	Summary   string          `json:"summary"`
	Payload   json.RawMessage `json:"payload"`
	PrevHash  string          `json:"prevHash"`
	Hash      string          `json:"hash"`
}

type Verification struct {
	Valid   bool   `json:"valid"`
	Checked int    `json:"checked"`
	Error   string `json:"error,omitempty"`
}

func NewEntry(record Record, previousHash string, now time.Time) (Entry, error) {
	if now.IsZero() {
		now = time.Now()
	}
	payload, err := normalizePayload(record.Payload)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{
		CreatedAt: now.UTC().Format(time.RFC3339Nano),
		Actor:     defaultString(record.Actor, "system"),
		Action:    strings.TrimSpace(record.Action),
		Entity:    strings.TrimSpace(record.Entity),
		EntityID:  strings.TrimSpace(record.EntityID),
		Status:    strings.TrimSpace(record.Status),
		Summary:   strings.TrimSpace(record.Summary),
		Payload:   payload,
		PrevHash:  strings.TrimSpace(previousHash),
	}
	entry.Hash = Hash(entry)
	return entry, nil
}

func Hash(entry Entry) string {
	hash := sha256.New()
	writePart(hash, entry.CreatedAt)
	writePart(hash, entry.Actor)
	writePart(hash, entry.Action)
	writePart(hash, entry.Entity)
	writePart(hash, entry.EntityID)
	writePart(hash, entry.Status)
	writePart(hash, entry.Summary)
	writePart(hash, string(entry.Payload))
	writePart(hash, entry.PrevHash)
	return hex.EncodeToString(hash.Sum(nil))
}

func normalizePayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return json.RawMessage(`{}`), nil
	}
	switch value := payload.(type) {
	case json.RawMessage:
		if len(value) == 0 {
			return json.RawMessage(`{}`), nil
		}
		if !json.Valid(value) {
			return nil, ErrInvalidPayload
		}
		return append(json.RawMessage(nil), value...), nil
	case []byte:
		if len(value) == 0 {
			return json.RawMessage(`{}`), nil
		}
		if !json.Valid(value) {
			return nil, ErrInvalidPayload
		}
		return append(json.RawMessage(nil), value...), nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return encoded, nil
	}
}

func writePart(hash hashWriter, value string) {
	hash.Write([]byte(value))
	hash.Write([]byte{0})
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

type hashWriter interface {
	Write([]byte) (int, error)
}
