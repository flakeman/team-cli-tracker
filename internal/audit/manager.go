package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Manager struct {
	mu   sync.Mutex
	path string
}

type Event struct {
	Time    string         `json:"time"`
	Type    string         `json:"type"`
	Actor   string         `json:"actor,omitempty"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details,omitempty"`
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
	return &Manager{path: path}, nil
}

func (m *Manager) Append(eventType, actor, status string, details map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := Event{
		Time:    time.Now().UTC().Format(time.RFC3339),
		Type:    eventType,
		Actor:   actor,
		Status:  status,
		Details: details,
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
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
