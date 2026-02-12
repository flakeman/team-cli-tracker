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
