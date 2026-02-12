package store

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladimir/team-cli-tracker/internal/events"
)

func TestOpenCreatesMetaWithCurrentSchema(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "events.meta.json"))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var m map[string]int
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if got := m["schema_version"]; got != currentSchemaVersion {
		t.Fatalf("schema_version=%d want=%d", got, currentSchemaVersion)
	}
}

func TestEncryptionEnableRotateAndVerifyIntegrity(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	e := events.SignedEvent{
		Version:   1,
		ProjectID: "OPS",
		EntityID:  "OPS-9",
		Type:      "issue.create",
		Payload:   []byte(`{"status":"todo","summary":"enc"}`),
		SignerID:  "node-1",
		SignerPub: []byte("pub"),
		Signature: []byte("sig"),
		Seq:       1,
		Timestamp: time.Now().UTC(),
	}
	if err := log.Append(e); err != nil {
		t.Fatalf("append: %v", err)
	}
	k1, err := log.EnableEncryption()
	if err != nil {
		t.Fatalf("enable encryption: %v", err)
	}
	if k1 == "" {
		t.Fatalf("empty key id")
	}
	k2, err := log.RotateEncryptionKey()
	if err != nil {
		t.Fatalf("rotate key: %v", err)
	}
	if k2 == "" || k2 == k1 {
		t.Fatalf("unexpected rotated key id: k1=%s k2=%s", k1, k2)
	}
	if err := log.VerifyIntegrity(); err == nil {
		t.Fatalf("verify integrity expected to fail for fake signature")
	}
}

func TestMigrationSetsDefaultEventVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	row := persistedEvent{
		SignedEvent: events.SignedEvent{
			Version:   0,
			ProjectID: "OPS",
			EntityID:  "OPS-1",
			Type:      "issue.create",
			SignerID:  "node-1",
			Seq:       1,
			Timestamp: time.Now().UTC(),
		},
		PayloadB64:   base64.StdEncoding.EncodeToString([]byte(`{"status":"todo","summary":"legacy"}`)),
		SignerPubB64: "",
		SignatureB64: base64.StdEncoding.EncodeToString([]byte("sig")),
	}
	raw, _ := json.Marshal(row)
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy row: %v", err)
	}
	// Force old schema marker.
	if err := os.WriteFile(filepath.Join(dir, "events.meta.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	log, err := Open(dir)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	all, err := log.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("events len=%d want=1", len(all))
	}
	if all[0].Version == 0 {
		t.Fatalf("expected migrated event version > 0")
	}
}
