package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIssueValidateRevoke(t *testing.T) {
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	rec, err := m.Issue("u1", "admin", time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, ok := m.Validate(rec.Token); !ok {
		t.Fatalf("token should validate")
	}
	if err := m.Revoke(rec.Token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, ok := m.Validate(rec.Token); ok {
		t.Fatalf("revoked token must not validate")
	}
}

func TestIssueUsesRoleBinding(t *testing.T) {
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := m.SetRoleBinding("u2", "lead"); err != nil {
		t.Fatalf("set role binding: %v", err)
	}
	rec, err := m.Issue("u2", "", time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if rec.Role != "lead" {
		t.Fatalf("role=%s want=lead", rec.Role)
	}
}

func TestSeedStaticMigrationCompatibility(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = m.SeedStatic(map[string]Principal{
		"legacy-token": {UserID: "legacy", Role: "dev", Active: true},
	})
	if err != nil {
		t.Fatalf("seed static: %v", err)
	}
	if p, ok := m.Validate("legacy-token"); !ok || p.UserID != "legacy" {
		t.Fatalf("legacy token must remain valid: ok=%v p=%+v", ok, p)
	}
}
