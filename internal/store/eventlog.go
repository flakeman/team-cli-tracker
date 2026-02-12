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
	mu       sync.Mutex
	path     string
	replayIx map[string]struct{}
	lastSeq  map[string]uint64
}

type persistedEvent struct {
	events.SignedEvent
	PayloadB64   string `json:"payload_b64"`
	SignerPubB64 string `json:"signer_pub_b64"`
	SignatureB64 string `json:"signature_b64"`
}

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
		path:     path,
		replayIx: make(map[string]struct{}),
		lastSeq:  make(map[string]uint64),
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

	row := persistedEvent{
		SignedEvent:  e,
		PayloadB64:   base64.StdEncoding.EncodeToString(e.Payload),
		SignerPubB64: base64.StdEncoding.EncodeToString(e.SignerPub),
		SignatureB64: base64.StdEncoding.EncodeToString(e.Signature),
	}
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
		payload, err := base64.StdEncoding.DecodeString(row.PayloadB64)
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

func replayKey(projectID, signerID string, seq uint64) string {
	return fmt.Sprintf("%s|%s|%d", projectID, signerID, seq)
}

func seqKey(projectID, signerID string) string {
	return projectID + "|" + signerID
}
