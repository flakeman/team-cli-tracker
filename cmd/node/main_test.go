package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladimir/team-cli-tracker/internal/audit"
	"github.com/vladimir/team-cli-tracker/internal/events"
	"github.com/vladimir/team-cli-tracker/internal/governance"
	"github.com/vladimir/team-cli-tracker/internal/node"
	"github.com/vladimir/team-cli-tracker/internal/store"
	"github.com/vladimir/team-cli-tracker/internal/team"
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

func TestOffboardingReassignsIssues(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "node")
	id, err := node.LoadOrCreate(dataDir, "node-1")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("log open: %v", err)
	}
	tm, err := team.Open(dataDir)
	if err != nil {
		t.Fatalf("team open: %v", err)
	}
	_ = tm.Onboard(team.Member{UserID: "lead1", Role: "lead", Active: true})
	_ = tm.Onboard(team.Member{UserID: "dev1", Role: "dev", Active: true})

	createPayload := map[string]string{"status": "todo", "summary": "Task", "assignee": "dev1"}
	raw, _ := json.Marshal(createPayload)
	e := events.SignedEvent{
		Version:   1,
		ProjectID: "OPS",
		EntityID:  "OPS-500",
		Type:      "issue.create",
		Payload:   raw,
		SignerID:  id.NodeID,
		SignerPub: id.Pub,
		Seq:       logDB.NextSeq("OPS", id.NodeID),
		Timestamp: time.Now().UTC(),
	}
	sig, _ := events.Sign(id.Priv, e)
	e.Signature = sig
	if err := logDB.Append(e); err != nil {
		t.Fatalf("append create: %v", err)
	}
	if _, err := tm.Offboard("dev1"); err != nil {
		t.Fatalf("offboard: %v", err)
	}
	reassigned, err := reassignOffboardedUser(dataDir, "node-1", "OPS", "dev1", tm)
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if reassigned != 1 {
		t.Fatalf("reassigned=%d want=1", reassigned)
	}
	all, _ := logDB.ReadAll()
	board := projectBoardFromEvents("OPS", all)
	if len(board["todo"]) != 1 || board["todo"][0].Assignee != "lead1" {
		t.Fatalf("unexpected assignee after reassign: %+v", board["todo"])
	}
}

func TestAuthzDenialForProtectedEndpoint(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "s"), "node-s", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"dev-token": {UserID: "dev1", Role: "dev", Active: true},
	}

	body := []byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer dev-token")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusForbidden)
	}
}

func TestMTLSConfigRequiresCA(t *testing.T) {
	_, err := buildServerTLSConfig("", true)
	if err == nil {
		t.Fatalf("expected error when mtls is enabled without CA file")
	}
}

func TestGovernanceReconfigureKeepsOddVotingSet(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "g"), "node-1", "OPS")
	defer cleanup()
	gm, err := governance.Open(filepath.Join(base, "g"))
	if err != nil {
		t.Fatalf("governance open: %v", err)
	}
	if err := gm.SetNodeRole("node-1", "voting", true); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if err := gm.SetNodeRole("node-2", "voting", true); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if err := gm.SetNodeRole("node-3", "voting", true); err != nil {
		t.Fatalf("set role: %v", err)
	}
	srv.syncServer.govManager = gm

	body := []byte(`{"voting_nodes":["node-1","node-3","node-4"]}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/governance/reconfigure", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// auth disabled in test server by default, so direct call is enough.
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusOK)
	}
	if got := gm.VotingCount(); got != 3 {
		t.Fatalf("voting count=%d want=3", got)
	}
	if gm.IsVoting("node-2") {
		t.Fatalf("node-2 should be non-voting after reconfigure")
	}
}

func TestGovernanceReconfigureRejectsEvenVotingSet(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "g"), "node-1", "OPS")
	defer cleanup()

	body := []byte(`{"voting_nodes":["node-1","node-2"]}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/governance/reconfigure", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusBadRequest)
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
	gm, err := governance.Open(dataDir)
	if err != nil {
		t.Fatalf("open governance: %v", err)
	}
	am, err := audit.Open(dataDir)
	if err != nil {
		t.Fatalf("open audit: %v", err)
	}
	s := &syncServer{
		projectID:    projectID,
		nodeID:       nodeID,
		log:          logDB,
		peers:        nil,
		govManager:   gm,
		auditManager: am,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sync/clock", s.syncClock)
	mux.HandleFunc("/sync/events", s.syncEvents)
	mux.HandleFunc("/sync/ingest", s.syncIngest)
	mux.HandleFunc("/raft/vote-transition", s.withAuthRoles(s.raftVoteTransition, "admin", "lead"))
	mux.HandleFunc("/raft/validate-transition", s.withAuthRoles(s.raftValidateTransition, "admin", "lead"))
	mux.HandleFunc("/raft/vote-governance-reconfigure", s.withAuthRoles(s.raftVoteGovernanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/raft/validate-governance-reconfigure", s.withAuthRoles(s.raftValidateGovernanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/governance/reconfigure", s.withAuthRoles(s.governanceReconfigure, "admin", "lead"))
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
