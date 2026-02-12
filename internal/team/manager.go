package team

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Manager struct {
	mu   sync.Mutex
	path string
	doc  document
}

type document struct {
	Members map[string]Member `json:"members"`
}

type Member struct {
	UserID string   `json:"user_id"`
	Role   string   `json:"role"`
	Active bool     `json:"active"`
	Duty   bool     `json:"duty"`
	Tokens []string `json:"tokens,omitempty"`
}

func Open(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		path: filepath.Join(dataDir, "team.json"),
		doc: document{
			Members: map[string]Member{},
		},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Onboard(in Member) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if in.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if in.Role == "" {
		in.Role = "viewer"
	}
	in.Active = true
	m.doc.Members[in.UserID] = in
	return m.save()
}

func (m *Manager) ChangeRole(userID, role string, duty bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mem, ok := m.doc.Members[userID]
	if !ok {
		return fmt.Errorf("member not found: %s", userID)
	}
	mem.Role = role
	mem.Duty = duty
	m.doc.Members[userID] = mem
	return m.save()
}

func (m *Manager) Offboard(userID string) (Member, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mem, ok := m.doc.Members[userID]
	if !ok {
		return Member{}, fmt.Errorf("member not found: %s", userID)
	}
	mem.Active = false
	mem.Tokens = nil
	m.doc.Members[userID] = mem
	if err := m.save(); err != nil {
		return Member{}, err
	}
	return mem, nil
}

func (m *Manager) List() []Member {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Member, 0, len(m.doc.Members))
	for _, v := range m.doc.Members {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out
}

func (m *Manager) ActiveLead() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.doc.Members {
		if v.Active && v.Role == "lead" {
			return v.UserID
		}
	}
	return ""
}

func (m *Manager) ActiveDuty() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.doc.Members {
		if v.Active && v.Duty {
			return v.UserID
		}
	}
	return ""
}

func (m *Manager) load() error {
	raw, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return m.save()
	}
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &m.doc); err != nil {
		return err
	}
	if m.doc.Members == nil {
		m.doc.Members = map[string]Member{}
	}
	return nil
}

func (m *Manager) save() error {
	raw, err := json.MarshalIndent(m.doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, raw, 0o644)
}
