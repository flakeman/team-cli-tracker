package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendRedactsSensitiveFields(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m.Append("auth.issue", "u1", "ok", map[string]any{
		"token":       "abc",
		"password":    "p",
		"safe":        "v",
		"nested_data": map[string]any{"client_secret": "zzz"},
	})
	evs, err := m.ReadAll()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("events=%d want=1", len(evs))
	}
	if evs[0].Details["token"] != "[redacted]" {
		t.Fatalf("token not redacted: %#v", evs[0].Details["token"])
	}
	if evs[0].Details["password"] != "[redacted]" {
		t.Fatalf("password not redacted: %#v", evs[0].Details["password"])
	}
	nested, _ := evs[0].Details["nested_data"].(map[string]any)
	if nested["client_secret"] != "[redacted]" {
		t.Fatalf("nested secret not redacted: %#v", nested["client_secret"])
	}
}

func TestVerifyIntegrityDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m.Append("team.onboard", "u1", "ok", map[string]any{"user_id": "u2"})
	m.Append("team.role_change", "u1", "ok", map[string]any{"role": "lead"})

	if _, err := m.VerifyIntegrity(); err != nil {
		t.Fatalf("verify before tamper: %v", err)
	}

	p := filepath.Join(dir, "security_audit.jsonl")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	tampered := strings.Replace(string(raw), "team.role_change", "team.role_hack", 1)
	if err := os.WriteFile(p, []byte(tampered), 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}
	if _, err := m.VerifyIntegrity(); err == nil {
		t.Fatalf("expected integrity failure after tamper")
	}
}
