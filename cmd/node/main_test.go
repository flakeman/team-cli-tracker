package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladimir/team-cli-tracker/internal/events"
	"github.com/vladimir/team-cli-tracker/internal/node"
	"github.com/vladimir/team-cli-tracker/internal/store"
)

func TestThreeNodeConvergeAfterReconnect(t *testing.T) {
	base := t.TempDir()
	logA, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	// Configure peers after URLs are known.
	srvA.syncServer.peers = []string{srvB.url, srvC.url}
	srvB.syncServer.peers = []string{srvA.url, srvC.url}
	srvC.syncServer.peers = []string{srvA.url, srvB.url}

	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-201", "issue.create", map[string]string{
		"status":  "todo",
		"summary": "Reconnect convergence",
	}); err != nil {
		t.Fatalf("append on A: %v", err)
	}

	// Simulate partial sync during split: C pulls from A, B is "offline".
	srvC.syncServer.pullFromPeer(srvA.url)

	// Reconnect: B catches up from A and C.
	srvB.syncServer.pullFromPeer(srvA.url)
	srvB.syncServer.pullFromPeer(srvC.url)

	evts, err := srvB.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all from B: %v", err)
	}
	board := projectBoardFromEvents("OPS", evts)
	items := board["todo"]
	if len(items) != 1 || items[0].ID != "OPS-201" {
		t.Fatalf("unexpected board after reconnect: %+v", board)
	}

	// Ensure source has the event too (sanity).
	src, err := logA.ReadAll()
	if err != nil || len(src) == 0 {
		t.Fatalf("source log empty err=%v", err)
	}
}

func TestSyncIngestRejectsInvalidSignatureAndReplay(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "s"), "node-s", "OPS")
	defer cleanup()

	id, err := node.LoadOrCreate(filepath.Join(base, "x"), "node-x")
	if err != nil {
		t.Fatalf("load identity: %v", err)
	}
	log, err := store.Open(filepath.Join(base, "x"))
	if err != nil {
		t.Fatalf("open log: %v", err)
	}

	ev := events.SignedEvent{
		Version:   1,
		ProjectID: "OPS",
		EntityID:  "OPS-301",
		Type:      "issue.create",
		Payload:   []byte(`{"status":"todo","summary":"sig test"}`),
		SignerID:  "node-x",
		SignerPub: id.Pub,
		Seq:       log.NextSeq("OPS", "node-x"),
		Timestamp: time.Now().UTC(),
	}
	sig, err := events.Sign(id.Priv, ev)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ev.Signature = sig

	invalid := ev
	invalid.Signature = []byte("bad-signature")

	if code := postIngest(t, srv.url, invalid); code != http.StatusBadRequest {
		t.Fatalf("invalid signature code=%d want=%d", code, http.StatusBadRequest)
	}
	if code := postIngest(t, srv.url, ev); code != http.StatusCreated {
		t.Fatalf("valid ingest code=%d want=%d", code, http.StatusCreated)
	}
	if code := postIngest(t, srv.url, ev); code != http.StatusConflict {
		t.Fatalf("replay ingest code=%d want=%d", code, http.StatusConflict)
	}
}

type testNodeServer struct {
	syncServer *syncServer
	server     *httptest.Server
	url        string
}

func newSyncServerForTest(t *testing.T, dataDir, nodeID, projectID string) (*store.EventLog, *testNodeServer, func()) {
	t.Helper()
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	s := &syncServer{
		projectID: projectID,
		nodeID:    nodeID,
		log:       logDB,
		peers:     nil,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sync/clock", s.syncClock)
	mux.HandleFunc("/sync/events", s.syncEvents)
	mux.HandleFunc("/sync/ingest", s.syncIngest)
	mux.HandleFunc("/raft/vote-transition", s.raftVoteTransition)
	mux.HandleFunc("/raft/validate-transition", s.raftValidateTransition)
	mux.HandleFunc("/metrics", s.metrics)
	ts := httptest.NewServer(mux)
	out := &testNodeServer{syncServer: s, server: ts, url: ts.URL}
	return logDB, out, func() { ts.Close() }
}

func postIngest(t *testing.T, baseURL string, e events.SignedEvent) int {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	res, err := http.Post(baseURL+"/sync/ingest", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post ingest: %v", err)
	}
	defer res.Body.Close()
	return res.StatusCode
}
