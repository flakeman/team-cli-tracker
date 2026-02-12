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
	"github.com/vladimir/team-cli-tracker/internal/trust"
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
	if err := srv.syncServer.trustManager.TrustNode("node-x"); err != nil {
		t.Fatalf("trust node-x: %v", err)
	}

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

func TestTeamOffboardRejectedWithoutQuorum(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "o"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.peers = []string{"http://127.0.0.1:1"}
	if err := srv.syncServer.teamManager.Onboard(team.Member{UserID: "dev1", Role: "dev", Active: true}); err != nil {
		t.Fatalf("onboard: %v", err)
	}

	body := []byte(`{"project_id":"OPS","user_id":"dev1"}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/team/offboard", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusOK)
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Status != "rejected" {
		t.Fatalf("status=%q want=%q", out.Status, "rejected")
	}
	members := srv.syncServer.teamManager.List()
	if len(members) != 1 || !members[0].Active {
		t.Fatalf("member must remain active when quorum is not reached: %+v", members)
	}
}

func TestTrustInviteRejectedWithoutQuorum(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "ti"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.peers = []string{"http://127.0.0.1:1"}

	body := []byte(`{"project_id":"OPS","node_id":"node-x","ttl_sec":300}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/trust/invite", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusOK)
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Status != "rejected" {
		t.Fatalf("status=%q want=%q", out.Status, "rejected")
	}
	if srv.syncServer.trustManager.IsTrusted("node-x") {
		t.Fatalf("node-x must not become trusted when quorum is not reached")
	}
}

func TestTrustRevokeRejectedWithoutQuorum(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "tr"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.peers = []string{"http://127.0.0.1:1"}
	if err := srv.syncServer.trustManager.TrustNode("node-z"); err != nil {
		t.Fatalf("trust node-z: %v", err)
	}

	body := []byte(`{"project_id":"OPS","node_id":"node-z"}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/trust/revoke", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusOK)
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Status != "rejected" {
		t.Fatalf("status=%q want=%q", out.Status, "rejected")
	}
	if !srv.syncServer.trustManager.IsTrusted("node-z") {
		t.Fatalf("node-z must remain trusted when quorum is not reached")
	}
}

func TestGovernanceReconfigurePartitionThenRejoin(t *testing.T) {
	base := t.TempDir()
	_, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	// Split A from the cluster: quorum for 2 nodes requires both votes, second vote is unreachable.
	srvA.syncServer.peers = []string{"http://127.0.0.1:1"}

	reconfigureBody := []byte(`{"project_id":"OPS","voting_nodes":["node-a","node-b","node-c"]}`)
	req, err := http.NewRequest(http.MethodPost, srvA.url+"/governance/reconfigure", bytes.NewReader(reconfigureBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	var rejected struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rejected); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("status=%q want=%q", rejected.Status, "rejected")
	}

	// Rejoin: A can reach B and C, quorum should pass.
	srvA.syncServer.peers = []string{srvB.url, srvC.url}
	req2, err := http.NewRequest(http.MethodPost, srvA.url+"/governance/reconfigure", bytes.NewReader(reconfigureBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res2.Body.Close()
	var accepted struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if accepted.Status != "reconfigured" {
		t.Fatalf("status=%q want=%q", accepted.Status, "reconfigured")
	}
}

func TestTeamOffboardPartitionThenRejoinConverges(t *testing.T) {
	base := t.TempDir()
	dataA := filepath.Join(base, "a")
	_, srvA, cleanupA := newSyncServerForTest(t, dataA, "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	if err := srvA.syncServer.teamManager.Onboard(team.Member{UserID: "lead1", Role: "lead", Active: true}); err != nil {
		t.Fatalf("onboard lead: %v", err)
	}
	if err := srvA.syncServer.teamManager.Onboard(team.Member{UserID: "dev1", Role: "dev", Active: true}); err != nil {
		t.Fatalf("onboard dev: %v", err)
	}
	createPayload, _ := json.Marshal(map[string]string{
		"status":   "todo",
		"summary":  "critical mutation partition test",
		"assignee": "dev1",
	})
	createEvent := events.SignedEvent{
		Version:   1,
		ProjectID: "OPS",
		EntityID:  "OPS-901",
		Type:      "issue.create",
		Payload:   createPayload,
		SignerID:  srvA.syncServer.identity.NodeID,
		SignerPub: srvA.syncServer.identity.Pub,
		Seq:       srvA.syncServer.log.NextSeq("OPS", srvA.syncServer.identity.NodeID),
		Timestamp: time.Now().UTC(),
	}
	createSig, err := events.Sign(srvA.syncServer.identity.Priv, createEvent)
	if err != nil {
		t.Fatalf("sign create event: %v", err)
	}
	createEvent.Signature = createSig
	if err := srvA.syncServer.log.Append(createEvent); err != nil {
		t.Fatalf("append issue: %v", err)
	}

	// Partition A from the rest: critical mutation must be rejected.
	srvA.syncServer.peers = []string{"http://127.0.0.1:1"}
	offboardBody := []byte(`{"project_id":"OPS","user_id":"dev1"}`)
	req, err := http.NewRequest(http.MethodPost, srvA.url+"/team/offboard", bytes.NewReader(offboardBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	var rejected struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rejected); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("status=%q want=%q", rejected.Status, "rejected")
	}

	// Rejoin and retry: mutation should apply and reassign event should converge to peers.
	srvA.syncServer.peers = []string{srvB.url, srvC.url}
	req2, err := http.NewRequest(http.MethodPost, srvA.url+"/team/offboard", bytes.NewReader(offboardBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res2.Body.Close()
	var accepted struct {
		Status           string `json:"status"`
		ReassignedIssues int    `json:"reassigned_issues"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if accepted.Status != "offboarded" {
		t.Fatalf("status=%q want=%q", accepted.Status, "offboarded")
	}
	if accepted.ReassignedIssues != 1 {
		t.Fatalf("reassigned_issues=%d want=1", accepted.ReassignedIssues)
	}

	evtsA, err := srvA.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all from A: %v", err)
	}
	boardA := projectBoardFromEvents("OPS", evtsA)
	if len(boardA["todo"]) != 1 || boardA["todo"][0].Assignee != "lead1" {
		t.Fatalf("unexpected board on A after offboard: %+v", boardA)
	}

	srvB.syncServer.pullFromPeer(srvA.url)
	srvC.syncServer.pullFromPeer(srvA.url)
	evtsB, err := srvB.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all from B: %v", err)
	}
	boardB := projectBoardFromEvents("OPS", evtsB)
	if len(boardB["todo"]) != 1 || boardB["todo"][0].Assignee != "lead1" {
		t.Fatalf("unexpected board on B after rejoin: %+v", boardB)
	}
}

func TestCriticalMutationsAuditContainQuorumMetadata(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "audit"), "node-1", "OPS")
	defer cleanup()

	if err := srv.syncServer.teamManager.Onboard(team.Member{UserID: "lead1", Role: "lead", Active: true}); err != nil {
		t.Fatalf("onboard lead: %v", err)
	}
	if err := srv.syncServer.teamManager.Onboard(team.Member{UserID: "dev1", Role: "dev", Active: true}); err != nil {
		t.Fatalf("onboard dev: %v", err)
	}

	reconfigureBody := []byte(`{"project_id":"OPS","voting_nodes":["node-1"]}`)
	reqReconf, err := http.NewRequest(http.MethodPost, srv.url+"/governance/reconfigure", bytes.NewReader(reconfigureBody))
	if err != nil {
		t.Fatalf("new reconfigure request: %v", err)
	}
	reqReconf.Header.Set("Content-Type", "application/json")
	resReconf, err := http.DefaultClient.Do(reqReconf)
	if err != nil {
		t.Fatalf("do reconfigure request: %v", err)
	}
	defer resReconf.Body.Close()

	offboardBody := []byte(`{"project_id":"OPS","user_id":"dev1"}`)
	reqOffboard, err := http.NewRequest(http.MethodPost, srv.url+"/team/offboard", bytes.NewReader(offboardBody))
	if err != nil {
		t.Fatalf("new offboard request: %v", err)
	}
	reqOffboard.Header.Set("Content-Type", "application/json")
	resOffboard, err := http.DefaultClient.Do(reqOffboard)
	if err != nil {
		t.Fatalf("do offboard request: %v", err)
	}
	defer resOffboard.Body.Close()

	inviteBody := []byte(`{"project_id":"OPS","node_id":"node-x","ttl_sec":300}`)
	reqInvite, err := http.NewRequest(http.MethodPost, srv.url+"/trust/invite", bytes.NewReader(inviteBody))
	if err != nil {
		t.Fatalf("new invite request: %v", err)
	}
	reqInvite.Header.Set("Content-Type", "application/json")
	resInvite, err := http.DefaultClient.Do(reqInvite)
	if err != nil {
		t.Fatalf("do invite request: %v", err)
	}
	defer resInvite.Body.Close()

	revokeBody := []byte(`{"project_id":"OPS","node_id":"node-x"}`)
	reqRevoke, err := http.NewRequest(http.MethodPost, srv.url+"/trust/revoke", bytes.NewReader(revokeBody))
	if err != nil {
		t.Fatalf("new revoke request: %v", err)
	}
	reqRevoke.Header.Set("Content-Type", "application/json")
	resRevoke, err := http.DefaultClient.Do(reqRevoke)
	if err != nil {
		t.Fatalf("do revoke request: %v", err)
	}
	defer resRevoke.Body.Close()

	records, err := srv.syncServer.auditManager.ReadTail(100)
	if err != nil {
		t.Fatalf("read audit tail: %v", err)
	}
	requireAuditQuorumMetadata(t, records, "governance.reconfigure")
	requireAuditQuorumMetadata(t, records, "team.offboard")
	requireAuditQuorumMetadata(t, records, "trust.invite")
	requireAuditQuorumMetadata(t, records, "trust.revoke")
}

func TestReassignFromEventsDeterministicOrder(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "d")
	nodeID := "node-1"
	projectID := "OPS"
	if err := appendIssueEvent(dataDir, nodeID, projectID, "OPS-2", "issue.create", map[string]string{
		"status":   "todo",
		"summary":  "Later id",
		"assignee": "dev1",
	}); err != nil {
		t.Fatalf("append OPS-2: %v", err)
	}
	if err := appendIssueEvent(dataDir, nodeID, projectID, "OPS-1", "issue.create", map[string]string{
		"status":   "todo",
		"summary":  "Earlier id",
		"assignee": "dev1",
	}); err != nil {
		t.Fatalf("append OPS-1: %v", err)
	}
	tm, err := team.Open(dataDir)
	if err != nil {
		t.Fatalf("team open: %v", err)
	}
	if err := tm.Onboard(team.Member{UserID: "lead1", Role: "lead", Active: true}); err != nil {
		t.Fatalf("onboard lead: %v", err)
	}
	if _, err := tm.Offboard("dev1"); err == nil {
		t.Fatalf("expected offboard error for missing dev1 before onboarding")
	}
	if err := tm.Onboard(team.Member{UserID: "dev1", Role: "dev", Active: true}); err != nil {
		t.Fatalf("onboard dev: %v", err)
	}
	if _, err := tm.Offboard("dev1"); err != nil {
		t.Fatalf("offboard dev: %v", err)
	}
	count, err := reassignOffboardedUser(dataDir, nodeID, projectID, "dev1", tm)
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if count != 2 {
		t.Fatalf("reassign count=%d want=2", count)
	}
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	all, err := logDB.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	reassigned := make([]string, 0, 2)
	for _, e := range all {
		if e.Type == "issue.reassign" {
			reassigned = append(reassigned, e.EntityID)
		}
	}
	if len(reassigned) != 2 || reassigned[0] != "OPS-1" || reassigned[1] != "OPS-2" {
		t.Fatalf("unexpected reassign order: %+v", reassigned)
	}
}

func requireAuditQuorumMetadata(t *testing.T, records []audit.Event, eventType string) {
	t.Helper()
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Type != eventType || rec.Status != "ok" {
			continue
		}
		if rec.Details == nil {
			t.Fatalf("%s details are missing", eventType)
		}
		granted, okGranted := rec.Details["granted"].(float64)
		needed, okNeeded := rec.Details["needed"].(float64)
		if !okGranted || !okNeeded {
			t.Fatalf("%s quorum fields are missing: %+v", eventType, rec.Details)
		}
		if granted < needed {
			t.Fatalf("%s invalid quorum metadata granted=%v needed=%v", eventType, granted, needed)
		}
		if _, ok := rec.Details["preferred_online"]; !ok {
			t.Fatalf("%s missing preferred_online in details: %+v", eventType, rec.Details)
		}
		if _, ok := rec.Details["failover_activated"]; !ok {
			t.Fatalf("%s missing failover_activated in details: %+v", eventType, rec.Details)
		}
		return
	}
	t.Fatalf("audit record not found for %s", eventType)
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
	tm, err := team.Open(dataDir)
	if err != nil {
		t.Fatalf("open team: %v", err)
	}
	trm, err := trust.Open(dataDir)
	if err != nil {
		t.Fatalf("open trust: %v", err)
	}
	id, err := node.LoadOrCreate(dataDir, nodeID)
	if err != nil {
		t.Fatalf("load identity: %v", err)
	}
	s := &syncServer{
		projectID:    projectID,
		nodeID:       nodeID,
		log:          logDB,
		peers:        nil,
		govManager:   gm,
		auditManager: am,
		teamManager:  tm,
		trustManager: trm,
		identity:     id,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sync/clock", s.syncClock)
	mux.HandleFunc("/sync/events", s.syncEvents)
	mux.HandleFunc("/sync/ingest", s.syncIngest)
	mux.HandleFunc("/raft/vote-transition", s.withAuthRoles(s.raftVoteTransition, "admin", "lead"))
	mux.HandleFunc("/raft/validate-transition", s.withAuthRoles(s.raftValidateTransition, "admin", "lead"))
	mux.HandleFunc("/raft/vote-governance-reconfigure", s.withAuthRoles(s.raftVoteGovernanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/raft/validate-governance-reconfigure", s.withAuthRoles(s.raftValidateGovernanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/raft/vote-team-offboard", s.withAuthRoles(s.raftVoteTeamOffboard, "admin", "lead"))
	mux.HandleFunc("/raft/validate-team-offboard", s.withAuthRoles(s.raftValidateTeamOffboard, "admin", "lead"))
	mux.HandleFunc("/raft/vote-trust-invite", s.withAuthRoles(s.raftVoteTrustInvite, "admin", "lead"))
	mux.HandleFunc("/raft/validate-trust-invite", s.withAuthRoles(s.raftValidateTrustInvite, "admin", "lead"))
	mux.HandleFunc("/raft/vote-trust-revoke", s.withAuthRoles(s.raftVoteTrustRevoke, "admin", "lead"))
	mux.HandleFunc("/raft/validate-trust-revoke", s.withAuthRoles(s.raftValidateTrustRevoke, "admin", "lead"))
	mux.HandleFunc("/governance/reconfigure", s.withAuthRoles(s.governanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/team/offboard", s.withAuthRoles(s.teamOffboard, "admin", "lead"))
	mux.HandleFunc("/trust/invite", s.withAuthRoles(s.trustInvite, "admin", "lead"))
	mux.HandleFunc("/trust/revoke", s.withAuthRoles(s.trustRevoke, "admin", "lead"))
	mux.HandleFunc("/trust/list", s.withAuthRoles(s.trustList, "admin", "lead"))
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
