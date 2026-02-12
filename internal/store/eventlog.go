package store

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/vladimir/team-cli-tracker/internal/events"
)

type EventLog struct {
	mu          sync.Mutex
	dataDir     string
	path        string
	metaPath    string
	keyRingPath string
	keyRing     *keyRing
	keyProvider externalKeyProvider
	replayIx    map[string]struct{}
	lastSeq     map[string]uint64
}

type persistedEvent struct {
	events.SignedEvent
	PayloadB64       string `json:"payload_b64"`
	PayloadEnc       bool   `json:"payload_enc"`
	PayloadKeyID     string `json:"payload_key_id,omitempty"`
	PayloadNonceB64  string `json:"payload_nonce_b64,omitempty"`
	PayloadCipherB64 string `json:"payload_cipher_b64,omitempty"`
	SignerPubB64     string `json:"signer_pub_b64"`
	SignatureB64     string `json:"signature_b64"`
}

type storeMeta struct {
	SchemaVersion int `json:"schema_version"`
}

const currentSchemaVersion = 2

func Open(dataDir string) (*EventLog, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	_ = f.Close()

	l := &EventLog{
		dataDir:  dataDir,
		path:     path,
		metaPath: filepath.Join(dataDir, "events.meta.json"),
		replayIx: make(map[string]struct{}),
		lastSeq:  make(map[string]uint64),
	}
	kr, krPath, err := loadKeyRing(dataDir)
	if err != nil {
		return nil, err
	}
	l.keyRing = kr
	l.keyRingPath = krPath
	provider, err := initExternalKeyProviderFromEnv()
	if err != nil {
		return nil, err
	}
	l.keyProvider = provider
	if err := l.ensureMetaAndMigrate(); err != nil {
		return nil, err
	}
	if err := l.loadIndexes(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *EventLog) NextSeq(projectID, signerID string) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastSeq[seqKey(projectID, signerID)] + 1
}

func (l *EventLog) Append(e events.SignedEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	replayKey := replayKey(e.ProjectID, e.SignerID, e.Seq)
	if _, exists := l.replayIx[replayKey]; exists {
		return fmt.Errorf("replay detected for project=%s signer=%s seq=%d", e.ProjectID, e.SignerID, e.Seq)
	}

	current := l.lastSeq[seqKey(e.ProjectID, e.SignerID)]
	if e.Seq != current+1 {
		return fmt.Errorf("invalid sequence: got=%d want=%d", e.Seq, current+1)
	}

	row, err := l.toPersistedEvent(e)
	if err != nil {
		return err
	}
	row.SignedEvent = e
	row.PayloadB64 = row.PayloadB64
	row.PayloadEnc = row.PayloadEnc
	row.PayloadKeyID = row.PayloadKeyID
	row.PayloadNonceB64 = row.PayloadNonceB64
	row.PayloadCipherB64 = row.PayloadCipherB64
	row.SignerPubB64 = base64.StdEncoding.EncodeToString(e.SignerPub)
	row.SignatureB64 = base64.StdEncoding.EncodeToString(e.Signature)
	row.Payload = nil
	row.SignerPub = nil
	row.Signature = nil

	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}

	l.replayIx[replayKey] = struct{}{}
	l.lastSeq[seqKey(e.ProjectID, e.SignerID)] = e.Seq
	return nil
}

func (l *EventLog) ReadAll() ([]events.SignedEvent, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.readAllUnlocked()
}

func (l *EventLog) readAllUnlocked() ([]events.SignedEvent, error) {

	f, err := os.Open(l.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make([]events.SignedEvent, 0)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var row persistedEvent
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, err
		}
		payload, err := l.decodePayload(row)
		if err != nil {
			return nil, err
		}
		pub, err := base64.StdEncoding.DecodeString(row.SignerPubB64)
		if err != nil {
			return nil, err
		}
		sig, err := base64.StdEncoding.DecodeString(row.SignatureB64)
		if err != nil {
			return nil, err
		}
		row.Payload = payload
		row.SignerPub = pub
		row.Signature = sig
		out = append(out, row.SignedEvent)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (l *EventLog) Clock(projectID string) (map[string]uint64, error) {
	eventsAll, err := l.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make(map[string]uint64)
	for _, e := range eventsAll {
		if e.ProjectID != projectID {
			continue
		}
		if e.Seq > out[e.SignerID] {
			out[e.SignerID] = e.Seq
		}
	}
	return out, nil
}

func (l *EventLog) EventsAfter(projectID, signerID string, afterSeq uint64) ([]events.SignedEvent, error) {
	eventsAll, err := l.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]events.SignedEvent, 0)
	for _, e := range eventsAll {
		if e.ProjectID != projectID || e.SignerID != signerID || e.Seq <= afterSeq {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (l *EventLog) loadIndexes() error {
	eventsAll, err := l.ReadAll()
	if err != nil {
		return err
	}
	for _, e := range eventsAll {
		l.replayIx[replayKey(e.ProjectID, e.SignerID, e.Seq)] = struct{}{}
		k := seqKey(e.ProjectID, e.SignerID)
		if e.Seq > l.lastSeq[k] {
			l.lastSeq[k] = e.Seq
		}
	}
	return nil
}

func (l *EventLog) ensureMetaAndMigrate() error {
	meta, err := l.loadMeta()
	if err != nil {
		return err
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = 1
	}
	if meta.SchemaVersion < currentSchemaVersion {
		if err := l.runMigrations(meta.SchemaVersion, currentSchemaVersion); err != nil {
			return err
		}
		meta.SchemaVersion = currentSchemaVersion
	}
	return l.saveMeta(meta)
}

func (l *EventLog) loadMeta() (storeMeta, error) {
	if _, err := os.Stat(l.metaPath); os.IsNotExist(err) {
		return storeMeta{SchemaVersion: 1}, nil
	}
	raw, err := os.ReadFile(l.metaPath)
	if err != nil {
		return storeMeta{}, err
	}
	var m storeMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return storeMeta{}, err
	}
	return m, nil
}

func (l *EventLog) saveMeta(m storeMeta) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.metaPath, raw, 0o644)
}

func (l *EventLog) runMigrations(fromVersion, toVersion int) error {
	if fromVersion >= toVersion {
		return nil
	}
	// v1 -> v2: ensure explicit event version field in jsonl rows.
	if fromVersion < 2 && toVersion >= 2 {
		if err := l.migrateSetDefaultEventVersion(); err != nil {
			return err
		}
	}
	return nil
}

func (l *EventLog) migrateSetDefaultEventVersion() error {
	eventsAll, err := l.ReadAll()
	if err != nil {
		return err
	}
	for i := range eventsAll {
		if eventsAll[i].Version == 0 {
			eventsAll[i].Version = 1
		}
	}
	return l.rewriteAll(eventsAll)
}

func (l *EventLog) rewriteAll(all []events.SignedEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rewriteAllUnlocked(all)
}

func (l *EventLog) rewriteAllUnlocked(all []events.SignedEvent) error {
	f, err := os.OpenFile(l.path, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range all {
		row, err := l.toPersistedEvent(e)
		if err != nil {
			return err
		}
		row.SignedEvent = e
		row.SignerPubB64 = base64.StdEncoding.EncodeToString(e.SignerPub)
		row.SignatureB64 = base64.StdEncoding.EncodeToString(e.Signature)
		row.Payload = nil
		row.SignerPub = nil
		row.Signature = nil
		raw, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}

func replayKey(projectID, signerID string, seq uint64) string {
	return fmt.Sprintf("%s|%s|%d", projectID, signerID, seq)
}

func seqKey(projectID, signerID string) string {
	return projectID + "|" + signerID
}

func (l *EventLog) toPersistedEvent(e events.SignedEvent) (persistedEvent, error) {
	keyID, key, err := l.activeKey()
	if err != nil {
		return persistedEvent{}, err
	}
	if len(key) == 0 {
		return persistedEvent{PayloadB64: base64.StdEncoding.EncodeToString(e.Payload)}, nil
	}
	nonce, cipherText, err := encryptAESGCM(key, e.Payload)
	if err != nil {
		return persistedEvent{}, err
	}
	return persistedEvent{
		PayloadEnc:       true,
		PayloadKeyID:     keyID,
		PayloadNonceB64:  base64.StdEncoding.EncodeToString(nonce),
		PayloadCipherB64: base64.StdEncoding.EncodeToString(cipherText),
	}, nil
}

func (l *EventLog) decodePayload(row persistedEvent) ([]byte, error) {
	if !row.PayloadEnc {
		return base64.StdEncoding.DecodeString(row.PayloadB64)
	}
	key, err := l.keyByID(row.PayloadKeyID)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(row.PayloadNonceB64)
	if err != nil {
		return nil, err
	}
	cipherText, err := base64.StdEncoding.DecodeString(row.PayloadCipherB64)
	if err != nil {
		return nil, err
	}
	return decryptAESGCM(key, nonce, cipherText)
}
