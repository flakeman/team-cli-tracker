package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	mu       sync.Mutex
	path     string
	lastHash string
	seq      int64
}

type Event struct {
	EventID string         `json:"event_id,omitempty"`
	Time    string         `json:"time"`
	Type    string         `json:"type"`
	Actor   string         `json:"actor,omitempty"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details,omitempty"`
	PrevHash string        `json:"prev_hash,omitempty"`
	Hash    string         `json:"hash,omitempty"`
}

func Open(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "security_audit.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	m := &Manager{path: path}
	existing, err := m.ReadAll()
	if err != nil {
		return nil, err
	}
	m.seq = int64(len(existing))
	for i := len(existing) - 1; i >= 0; i-- {
		if strings.TrimSpace(existing[i].Hash) != "" {
			m.lastHash = strings.TrimSpace(existing[i].Hash)
			break
		}
	}
	return m, nil
}

func (m *Manager) Append(eventType, actor, status string, details map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	now := time.Now().UTC().Format(time.RFC3339)
	rd := redactDetails(details)
	e := Event{
		EventID: fmt.Sprintf("ae-%d", m.seq),
		Time:    now,
		Type:    eventType,
		Actor:   actor,
		Status:  status,
		Details: rd,
		PrevHash: m.lastHash,
	}
	e.Hash = computeEventHash(e)
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return
	}
	m.lastHash = e.Hash
}

func (m *Manager) ReadTail(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	raw, err := os.ReadFile(m.path)
	if err != nil {
		return nil, err
	}
	lines := splitLines(string(raw))
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	out := make([]Event, 0, len(lines))
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (m *Manager) ReadAll() ([]Event, error) {
	raw, err := os.ReadFile(m.path)
	if err != nil {
		return nil, err
	}
	lines := splitLines(string(raw))
	out := make([]Event, 0, len(lines))
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (m *Manager) VerifyIntegrity() (int, error) {
	events, err := m.ReadAll()
	if err != nil {
		return 0, err
	}
	prev := ""
	checked := 0
	for _, e := range events {
		if strings.TrimSpace(e.Hash) == "" {
			continue
		}
		if strings.TrimSpace(e.PrevHash) != prev {
			return checked, fmt.Errorf("broken prev_hash chain at event_id=%s", e.EventID)
		}
		expected := computeEventHash(e)
		if strings.TrimSpace(e.Hash) != expected {
			return checked, fmt.Errorf("hash mismatch at event_id=%s", e.EventID)
		}
		prev = strings.TrimSpace(e.Hash)
		checked++
	}
	return checked, nil
}

func computeEventHash(e Event) string {
	details := "{}"
	if e.Details != nil {
		if raw, err := json.Marshal(e.Details); err == nil {
			details = string(raw)
		}
	}
	base := strings.Join([]string{
		strings.TrimSpace(e.EventID),
		strings.TrimSpace(e.Time),
		strings.TrimSpace(e.Type),
		strings.TrimSpace(e.Actor),
		strings.TrimSpace(e.Status),
		strings.TrimSpace(e.PrevHash),
		details,
	}, "|")
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func redactDetails(details map[string]any) map[string]any {
	if details == nil {
		return nil
	}
	out := make(map[string]any, len(details))
	for k, v := range details {
		lk := strings.ToLower(strings.TrimSpace(k))
		if isSensitiveKey(lk) {
			out[k] = "[redacted]"
			continue
		}
		switch vv := v.(type) {
		case map[string]any:
			out[k] = redactDetails(vv)
		default:
			out[k] = vv
		}
	}
	return out
}

func isSensitiveKey(k string) bool {
	if k == "" {
		return false
	}
	return strings.Contains(k, "token") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "password") ||
		strings.Contains(k, "authorization") ||
		strings.Contains(k, "private_key") ||
		strings.Contains(k, "key_material")
}

func splitLines(s string) []string {
	out := make([]string, 0)
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		if r != '\r' {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
