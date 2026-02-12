package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Principal struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

type TokenRecord struct {
	Principal
	Token     string `json:"token"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
	Source    string `json:"source,omitempty"`
}

type RoleBinding struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type document struct {
	Tokens       map[string]TokenRecord `json:"tokens"`
	RoleBindings map[string]string      `json:"role_bindings"`
}

type Manager struct {
	mu   sync.Mutex
	path string
	doc  document
}

func Open(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		path: filepath.Join(dataDir, "auth.tokens.json"),
		doc: document{
			Tokens:       map[string]TokenRecord{},
			RoleBindings: map[string]string{},
		},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Issue(userID, role string, ttl time.Duration) (TokenRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	userID = strings.TrimSpace(userID)
	role = strings.ToLower(strings.TrimSpace(role))
	if userID == "" {
		return TokenRecord{}, os.ErrInvalid
	}
	if role == "" {
		if bound := strings.TrimSpace(m.doc.RoleBindings[userID]); bound != "" {
			role = strings.ToLower(bound)
		} else {
			role = "viewer"
		}
	}
	token, err := randomToken()
	if err != nil {
		return TokenRecord{}, err
	}
	now := time.Now().UTC()
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	rec := TokenRecord{
		Principal: Principal{
			UserID: userID,
			Role:   role,
			Active: true,
		},
		Token:     token,
		IssuedAt:  now.Format(time.RFC3339),
		ExpiresAt: now.Add(ttl).Format(time.RFC3339),
		Source:    "issuer",
	}
	m.doc.Tokens[token] = rec
	if _, ok := m.doc.RoleBindings[userID]; !ok {
		m.doc.RoleBindings[userID] = role
	}
	if err := m.save(); err != nil {
		return TokenRecord{}, err
	}
	return rec, nil
}

func (m *Manager) Revoke(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	token = strings.TrimSpace(token)
	rec, ok := m.doc.Tokens[token]
	if !ok {
		return os.ErrNotExist
	}
	rec.Active = false
	m.doc.Tokens[token] = rec
	return m.save()
}

func (m *Manager) Validate(token string) (Principal, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.doc.Tokens[strings.TrimSpace(token)]
	if !ok || !rec.Active {
		return Principal{}, false
	}
	if exp, err := time.Parse(time.RFC3339, strings.TrimSpace(rec.ExpiresAt)); err == nil {
		if time.Now().UTC().After(exp) {
			return Principal{}, false
		}
	}
	return rec.Principal, true
}

func (m *Manager) SeedStatic(tokens map[string]Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	exp := time.Now().UTC().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	for token, p := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, exists := m.doc.Tokens[token]; exists {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(p.Role))
		m.doc.Tokens[token] = TokenRecord{
			Principal: Principal{
				UserID: strings.TrimSpace(p.UserID),
				Role:   role,
				Active: p.Active,
			},
			Token:     token,
			IssuedAt:  now,
			ExpiresAt: exp,
			Source:    "static",
		}
		if uid := strings.TrimSpace(p.UserID); uid != "" && role != "" {
			if _, ok := m.doc.RoleBindings[uid]; !ok {
				m.doc.RoleBindings[uid] = role
			}
		}
	}
	return m.save()
}

func (m *Manager) SetRoleBinding(userID, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	userID = strings.TrimSpace(userID)
	role = strings.ToLower(strings.TrimSpace(role))
	if userID == "" || role == "" {
		return os.ErrInvalid
	}
	m.doc.RoleBindings[userID] = role
	return m.save()
}

func (m *Manager) ListRoleBindings() []RoleBinding {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RoleBinding, 0, len(m.doc.RoleBindings))
	for uid, role := range m.doc.RoleBindings {
		out = append(out, RoleBinding{UserID: uid, Role: role})
	}
	return out
}

func (m *Manager) ListTokens() []TokenRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TokenRecord, 0, len(m.doc.Tokens))
	for _, rec := range m.doc.Tokens {
		out = append(out, rec)
	}
	return out
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
	if m.doc.Tokens == nil {
		m.doc.Tokens = map[string]TokenRecord{}
	}
	if m.doc.RoleBindings == nil {
		m.doc.RoleBindings = map[string]string{}
	}
	return nil
}

func (m *Manager) save() error {
	raw, err := json.MarshalIndent(m.doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, raw, 0o600)
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
