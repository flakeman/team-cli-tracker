package trust

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Manager struct {
	mu   sync.Mutex
	path string
	doc  document
}

type document struct {
	TrustedNodes map[string]bool   `json:"trusted_nodes"`
	Invites      map[string]invite `json:"invites"`
}

type invite struct {
	NodeID    string `json:"node_id"`
	ExpiresAt string `json:"expires_at"`
}

func Open(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		path: filepath.Join(dataDir, "trust.json"),
		doc: document{
			TrustedNodes: map[string]bool{},
			Invites:      map[string]invite{},
		},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) TrustNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doc.TrustedNodes[nodeID] = true
	return m.save()
}

func (m *Manager) RevokeNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.doc.TrustedNodes, nodeID)
	for token, inv := range m.doc.Invites {
		if inv.NodeID == nodeID {
			delete(m.doc.Invites, token)
		}
	}
	return m.save()
}

func (m *Manager) IsTrusted(nodeID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc.TrustedNodes[nodeID]
}

func (m *Manager) ListTrusted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.doc.TrustedNodes))
	for id, ok := range m.doc.TrustedNodes {
		if ok {
			out = append(out, id)
		}
	}
	return out
}

func (m *Manager) CreateInvite(nodeID string, ttl time.Duration) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ttl <= 0 {
		ttl = time.Hour
	}
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().UTC().Add(ttl)
	m.doc.Invites[token] = invite{
		NodeID:    nodeID,
		ExpiresAt: exp.Format(time.RFC3339),
	}
	if err := m.save(); err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

func (m *Manager) UseInvite(token, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.doc.Invites[token]
	if !ok {
		return fmt.Errorf("invite token not found")
	}
	exp, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return err
	}
	if time.Now().UTC().After(exp) {
		delete(m.doc.Invites, token)
		_ = m.save()
		return fmt.Errorf("invite token expired")
	}
	if inv.NodeID != nodeID {
		return fmt.Errorf("invite token is bound to different node_id")
	}
	delete(m.doc.Invites, token)
	m.doc.TrustedNodes[nodeID] = true
	return m.save()
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
	if m.doc.TrustedNodes == nil {
		m.doc.TrustedNodes = map[string]bool{}
	}
	if m.doc.Invites == nil {
		m.doc.Invites = map[string]invite{}
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

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
