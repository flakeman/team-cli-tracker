package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	mathrand "math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vladimir/team-cli-tracker/internal/audit"
	authstore "github.com/vladimir/team-cli-tracker/internal/auth"
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

func TestThreeNodeConvergeUnderChurnFlapping(t *testing.T) {
	base := t.TempDir()
	_, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	// Initial topology: B sees only A. C joins later (churn/join).
	srvA.syncServer.peers = []string{srvB.url}
	srvB.syncServer.peers = []string{srvA.url}
	srvC.syncServer.peers = []string{srvA.url, srvB.url}

	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-301", "issue.create", map[string]string{
		"status":  "todo",
		"summary": "from A",
	}); err != nil {
		t.Fatalf("append A create: %v", err)
	}
	if err := appendIssueEvent(filepath.Join(base, "c"), "node-c", "OPS", "OPS-302", "issue.create", map[string]string{
		"status":  "todo",
		"summary": "from C",
	}); err != nil {
		t.Fatalf("append C create: %v", err)
	}

	// Phase 1: B pulls from A only (C not joined yet from B perspective).
	srvB.syncServer.pullFromPeer(srvA.url)

	// Join: B learns C and syncs.
	srvB.syncServer.peers = []string{srvA.url, srvC.url}
	srvB.syncServer.pullFromPeer(srvC.url)

	// Flapping: temporarily drop A from B peers while new updates are written.
	srvB.syncServer.peers = []string{srvC.url}
	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-301", "issue.transition", map[string]string{
		"from": "todo",
		"to":   "in_progress",
	}); err != nil {
		t.Fatalf("append A transition: %v", err)
	}
	if err := appendIssueEvent(filepath.Join(base, "c"), "node-c", "OPS", "OPS-302", "issue.transition", map[string]string{
		"from": "todo",
		"to":   "code_review",
	}); err != nil {
		t.Fatalf("append C transition: %v", err)
	}
	srvB.syncServer.pullFromPeer(srvC.url)

	// Rejoin: restore A and converge from both sides.
	srvB.syncServer.peers = []string{srvA.url, srvC.url}
	for i := 0; i < 3; i++ {
		srvB.syncServer.pullFromPeer(srvA.url)
		srvB.syncServer.pullFromPeer(srvC.url)
	}

	all, err := srvB.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all on B: %v", err)
	}
	board := projectBoardFromEvents("OPS", all)
	if len(board["in_progress"]) != 1 || board["in_progress"][0].ID != "OPS-301" {
		t.Fatalf("OPS-301 not converged to in_progress: %+v", board["in_progress"])
	}
	if len(board["code_review"]) != 1 || board["code_review"][0].ID != "OPS-302" {
		t.Fatalf("OPS-302 not converged to code_review: %+v", board["code_review"])
	}
}

func TestProjectBoardFromEventsDeterministicReplayOrder(t *testing.T) {
	base := t.TempDir()
	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-401", "issue.create", map[string]string{
		"status":  "todo",
		"summary": "deterministic replay",
	}); err != nil {
		t.Fatalf("append create: %v", err)
	}
	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-401", "issue.transition", map[string]string{
		"from": "todo",
		"to":   "in_progress",
	}); err != nil {
		t.Fatalf("append transition: %v", err)
	}
	logA, err := store.Open(filepath.Join(base, "a"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	all, err := logA.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}

	one := append([]events.SignedEvent(nil), all...)
	two := append([]events.SignedEvent(nil), all...)
	r := mathrand.New(mathrand.NewSource(42))
	r.Shuffle(len(two), func(i, j int) {
		two[i], two[j] = two[j], two[i]
	})

	b1 := projectBoardFromEvents("OPS", one)
	b2 := projectBoardFromEvents("OPS", two)
	if len(b1["in_progress"]) != 1 || len(b2["in_progress"]) != 1 {
		t.Fatalf("unexpected in_progress lengths b1=%d b2=%d", len(b1["in_progress"]), len(b2["in_progress"]))
	}
	if b1["in_progress"][0].ID != b2["in_progress"][0].ID || b1["in_progress"][0].Status != b2["in_progress"][0].Status {
		t.Fatalf("replay result differs by input order: b1=%+v b2=%+v", b1["in_progress"][0], b2["in_progress"][0])
	}
}

func TestThreeNodeConflictingTransitionsDeterministicAfterRejoin(t *testing.T) {
	base := t.TempDir()
	_, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	srvA.syncServer.peers = []string{srvB.url, srvC.url}
	srvB.syncServer.peers = []string{srvA.url, srvC.url}
	srvC.syncServer.peers = []string{srvA.url, srvB.url}

	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-402", "issue.create", map[string]string{
		"status":  "todo",
		"summary": "conflict",
	}); err != nil {
		t.Fatalf("append create: %v", err)
	}
	srvB.syncServer.pullFromPeer(srvA.url)
	srvC.syncServer.pullFromPeer(srvA.url)

	// Partition-like conflicting local writes.
	if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", "OPS-402", "issue.transition", map[string]string{
		"from": "todo",
		"to":   "in_progress",
	}); err != nil {
		t.Fatalf("append a transition: %v", err)
	}
	if err := appendIssueEvent(filepath.Join(base, "b"), "node-b", "OPS", "OPS-402", "issue.transition", map[string]string{
		"from": "todo",
		"to":   "testing",
	}); err != nil {
		t.Fatalf("append b transition: %v", err)
	}

	// Rejoin: exchange updates until convergence.
	for i := 0; i < 4; i++ {
		srvA.syncServer.pullFromPeer(srvB.url)
		srvA.syncServer.pullFromPeer(srvC.url)
		srvB.syncServer.pullFromPeer(srvA.url)
		srvB.syncServer.pullFromPeer(srvC.url)
		srvC.syncServer.pullFromPeer(srvA.url)
		srvC.syncServer.pullFromPeer(srvB.url)
	}

	allA, err := srvA.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all A: %v", err)
	}
	allB, err := srvB.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all B: %v", err)
	}
	allC, err := srvC.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read all C: %v", err)
	}
	ba := projectBoardFromEvents("OPS", allA)
	bb := projectBoardFromEvents("OPS", allB)
	bc := projectBoardFromEvents("OPS", allC)

	getStatus := func(board map[string][]issueProjection, id string) string {
		for col, items := range board {
			for _, it := range items {
				if it.ID == id {
					return col
				}
			}
		}
		return ""
	}
	sa := getStatus(ba, "OPS-402")
	sb := getStatus(bb, "OPS-402")
	sc := getStatus(bc, "OPS-402")
	if sa == "" || sb == "" || sc == "" {
		t.Fatalf("missing issue status after convergence: a=%q b=%q c=%q", sa, sb, sc)
	}
	if sa != sb || sb != sc {
		t.Fatalf("divergent final status after rejoin: a=%q b=%q c=%q", sa, sb, sc)
	}
}

func TestPrintBoardPlainTableFormat(t *testing.T) {
	board := map[string][]issueProjection{
		"todo":        {{ID: "OPS-1", Summary: "first", Assignee: ""}},
		"in_progress": {{ID: "OPS-2", Summary: "second", Assignee: "dev1"}},
		"code_review": {},
		"testing":     {},
		"done":        {{ID: "OPS-3", Summary: "third", Assignee: "qa1"}},
	}
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	printBoardPlain("OPS", board, board, 7, "all", "")
	_ = w.Close()
	os.Stdout = oldStdout
	raw, _ := io.ReadAll(r)
	out := string(raw)
	if !strings.Contains(out, "Project: OPS  Revision: 7") {
		t.Fatalf("missing project header: %s", out)
	}
	if !strings.Contains(out, "Assignee WIP limit: 3") {
		t.Fatalf("missing wip header: %s", out)
	}
	if !strings.Contains(out, "Scope: all") {
		t.Fatalf("missing scope header: %s", out)
	}
	if !strings.Contains(out, "Displayed issues: 3  Open: 2  Done: 1") {
		t.Fatalf("missing displayed header: %s", out)
	}
	if !strings.Contains(out, "All issues: 3  Open: 2  Done: 1") {
		t.Fatalf("missing totals header: %s", out)
	}
	if !strings.Contains(out, "To Do [1]") || !strings.Contains(out, "In Progress [1/3]") || !strings.Contains(out, "Done [1/3]") {
		t.Fatalf("missing column headers: %s", out)
	}
	if !strings.Contains(out, "OPS-1 first @unassigned") || !strings.Contains(out, "OPS-2 second @dev1") || !strings.Contains(out, "OPS-3 third @qa1") {
		t.Fatalf("missing issue cards in table: %s", out)
	}
	if !strings.Contains(out, "+----------------------------------+") {
		t.Fatalf("missing table separator: %s", out)
	}
}

func TestRenderBoardPlainEmptyBoard(t *testing.T) {
	board := map[string][]issueProjection{
		"todo": {}, "in_progress": {}, "code_review": {}, "testing": {}, "done": {},
	}
	out := renderBoardPlain("OPS", board, board, 0, "all", "")
	if !strings.Contains(out, "Project: OPS  Revision: 0") {
		t.Fatalf("missing empty revision header: %s", out)
	}
	if !strings.Contains(out, "Displayed issues: 0  Open: 0  Done: 0") {
		t.Fatalf("missing empty totals header: %s", out)
	}
	if strings.Count(out, "|                                  |                                  |                                  |                                  |                                  |") < 1 {
		t.Fatalf("missing empty table row: %s", out)
	}
}

func TestRenderBoardPlainLongCardTruncates(t *testing.T) {
	board := map[string][]issueProjection{
		"todo":        {{ID: "OPS-100500", Summary: strings.Repeat("very-long-summary-", 6), Assignee: "verylongassigneeid"}},
		"in_progress": {}, "code_review": {}, "testing": {}, "done": {},
	}
	out := renderBoardPlain("OPS", board, board, 10, "all", "")
	if !strings.Contains(out, "...") {
		t.Fatalf("expected truncation marker: %s", out)
	}
	if strings.Contains(out, "вЂ¦") {
		t.Fatalf("found broken ellipsis encoding: %s", out)
	}
}

func TestRenderBoardPlainDeterministicAcrossCalls(t *testing.T) {
	board := map[string][]issueProjection{
		"todo":        {{ID: "OPS-1", Summary: "a", Assignee: ""}, {ID: "OPS-2", Summary: "b", Assignee: "u2"}},
		"in_progress": {{ID: "OPS-3", Summary: "c", Assignee: "u3"}},
		"code_review": {},
		"testing":     {{ID: "OPS-4", Summary: "d", Assignee: "u4"}},
		"done":        {},
	}
	out1 := renderBoardPlain("OPS", board, board, 11, "all", "")
	out2 := renderBoardPlain("OPS", board, board, 11, "all", "")
	if out1 != out2 {
		t.Fatalf("non-deterministic board render")
	}
	if strings.Count(out1, "\n") < 6 {
		t.Fatalf("unexpectedly short table output: %s", out1)
	}
}

func TestFilterBoardByAssignee(t *testing.T) {
	board := map[string][]issueProjection{
		"todo":        {{ID: "OPS-1", Summary: "a", Assignee: "u1"}, {ID: "OPS-2", Summary: "b", Assignee: "u2"}},
		"in_progress": {{ID: "OPS-3", Summary: "c", Assignee: "u1"}},
		"code_review": {},
		"testing":     {},
		"done":        {{ID: "OPS-4", Summary: "d", Assignee: "u2"}},
	}
	out := filterBoardByAssignee(board, "u1")
	if len(out["todo"]) != 1 || out["todo"][0].ID != "OPS-1" {
		t.Fatalf("unexpected todo filter: %+v", out["todo"])
	}
	if len(out["in_progress"]) != 1 || out["in_progress"][0].ID != "OPS-3" {
		t.Fatalf("unexpected in_progress filter: %+v", out["in_progress"])
	}
	if len(out["done"]) != 0 {
		t.Fatalf("unexpected done filter: %+v", out["done"])
	}
}

func TestPrintBoardCounts(t *testing.T) {
	board := map[string][]issueProjection{
		"todo":        {{ID: "OPS-1", Summary: "a", Assignee: "u1"}},
		"in_progress": {{ID: "OPS-2", Summary: "b", Assignee: "u1"}},
		"code_review": {},
		"testing":     {},
		"done":        {{ID: "OPS-3", Summary: "c", Assignee: "u1"}},
	}
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	printBoardCounts("OPS", board, board, 9, "all", "")
	_ = w.Close()
	os.Stdout = oldStdout
	raw, _ := io.ReadAll(r)
	out := string(raw)
	if !strings.Contains(out, "Project: OPS  Revision: 9") {
		t.Fatalf("missing revision: %s", out)
	}
	if !strings.Contains(out, "Displayed issues: 3  Open: 2  Done: 1") {
		t.Fatalf("missing totals: %s", out)
	}
	if !strings.Contains(out, "All issues: 3  Open: 2  Done: 1") {
		t.Fatalf("missing all totals: %s", out)
	}
	if !strings.Contains(out, "To Do=1 In Progress=1 Code Review=0 Testing=0 Done=1") {
		t.Fatalf("missing column counts: %s", out)
	}
}

func TestTickerChanPaused(t *testing.T) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	state := &boardRenderState{interactivePaused: true}
	if ch := tickerChan(ticker, state); ch != nil {
		t.Fatalf("expected nil ticker channel when paused")
	}
	state.interactivePaused = false
	if ch := tickerChan(ticker, state); ch == nil {
		t.Fatalf("expected ticker channel when resumed")
	}
}

func TestResolvedAssigneeForScope(t *testing.T) {
	st := boardRenderState{viewMode: "mine", assigneeID: "", effectiveViewAssigneeID: "vova"}
	if got := resolvedAssigneeForScope(st); got != "vova" {
		t.Fatalf("got=%q want=%q", got, "vova")
	}
	st = boardRenderState{viewMode: "mine", assigneeID: "dev1", effectiveViewAssigneeID: ""}
	if got := resolvedAssigneeForScope(st); got != "dev1" {
		t.Fatalf("got=%q want=%q", got, "dev1")
	}
	st = boardRenderState{viewMode: "all", assigneeID: "dev1", effectiveViewAssigneeID: "dev1"}
	if got := resolvedAssigneeForScope(st); got != "" {
		t.Fatalf("got=%q want empty", got)
	}
}

func TestInteractiveMovePolicyDenyDoesNotAppend(t *testing.T) {
	dataDir := t.TempDir()
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	in := boardInteractiveInput{
		projectID: "OPS",
		nodeID:    "node-1",
		dataDir:   dataDir,
		policyURL: srv.URL,
		state:     &boardRenderState{},
	}
	before, _ := logDB.ReadAll()
	quit, msg := applyBoardInteractiveCommand("move OPS-P1 todo in_progress", in)
	after, _ := logDB.ReadAll()
	if quit {
		t.Fatalf("unexpected quit on deny")
	}
	if !strings.Contains(strings.ToLower(msg), "policy") && !strings.Contains(strings.ToLower(msg), "forbidden") && !strings.Contains(strings.ToLower(msg), "status") {
		t.Fatalf("unexpected deny message: %s", msg)
	}
	if len(after) != len(before) {
		t.Fatalf("event appended on denied policy: before=%d after=%d", len(before), len(after))
	}
}

func TestInteractiveMovePolicyAllowAppends(t *testing.T) {
	dataDir := t.TempDir()
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"allow": true})
	}))
	defer srv.Close()

	in := boardInteractiveInput{
		projectID: "OPS",
		nodeID:    "node-1",
		dataDir:   dataDir,
		policyURL: srv.URL,
		state:     &boardRenderState{},
	}
	before, _ := logDB.ReadAll()
	quit, msg := applyBoardInteractiveCommand("move OPS-P2 todo in_progress", in)
	after, _ := logDB.ReadAll()
	if quit {
		t.Fatalf("unexpected quit on allow")
	}
	if !strings.Contains(msg, "moved OPS-P2") {
		t.Fatalf("unexpected allow message: %s", msg)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("expected appended transition event: before=%d after=%d", len(before), len(after))
	}
}

func TestInteractiveCreatePolicyDenyDoesNotAppend(t *testing.T) {
	dataDir := t.TempDir()
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()
	in := boardInteractiveInput{
		projectID: "OPS",
		nodeID:    "node-1",
		dataDir:   dataDir,
		policyURL: srv.URL,
		state:     &boardRenderState{},
	}
	before, _ := logDB.ReadAll()
	_, msg := applyBoardInteractiveCommand("create OPS-C1 test create", in)
	after, _ := logDB.ReadAll()
	if len(after) != len(before) {
		t.Fatalf("event appended on denied create: before=%d after=%d", len(before), len(after))
	}
	if !strings.Contains(strings.ToLower(msg), "denied") && !strings.Contains(strings.ToLower(msg), "status") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestInteractiveCommentPolicyAllowAppends(t *testing.T) {
	dataDir := t.TempDir()
	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	// allow create/comment endpoints in test server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()
	in := boardInteractiveInput{
		projectID: "OPS",
		nodeID:    "node-1",
		dataDir:   dataDir,
		policyURL: srv.URL,
		state:     &boardRenderState{},
	}
	// create via protected endpoint should not append locally in this mode.
	before, _ := logDB.ReadAll()
	_, _ = applyBoardInteractiveCommand("create OPS-C2 test", in)
	mid, _ := logDB.ReadAll()
	if len(mid) != len(before) {
		t.Fatalf("expected no local append for protected create path")
	}
	// comment via protected endpoint should not append locally in this mode.
	_, _ = applyBoardInteractiveCommand("comment OPS-C2 hello", in)
	after, _ := logDB.ReadAll()
	if len(after) != len(before) {
		t.Fatalf("expected no local append for protected comment path")
	}
}

func TestParseBoardInteractiveCommand(t *testing.T) {
	cases := []struct {
		in   string
		name string
		args int
		ok   bool
	}{
		{in: "help", name: "help", args: 0, ok: true},
		{in: "quit", name: "quit", args: 0, ok: true},
		{in: "counts", name: "counts", args: 0, ok: true},
		{in: "view mine vova", name: "view", args: 2, ok: true},
		{in: "create OPS-1 hello world", name: "create", args: 2, ok: true},
		{in: "move OPS-1 todo in_progress", name: "move", args: 3, ok: true},
		{in: "comment OPS-1 text here", name: "comment", args: 2, ok: true},
		{in: "move OPS-1 todo", ok: false},
		{in: "", ok: false},
	}
	for _, tc := range cases {
		cmd, err := parseBoardInteractiveCommand(tc.in)
		if tc.ok && err != nil {
			t.Fatalf("input=%q unexpected err=%v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("input=%q expected error", tc.in)
		}
		if tc.ok {
			if cmd.name != tc.name {
				t.Fatalf("input=%q name=%q want=%q", tc.in, cmd.name, tc.name)
			}
			if len(cmd.args) != tc.args {
				t.Fatalf("input=%q args=%d want=%d", tc.in, len(cmd.args), tc.args)
			}
		}
	}
}

func TestApplyBoardInteractiveCommandCreateAutoIDMultiple(t *testing.T) {
	dataDir := t.TempDir()
	projectID := "OPS"
	nodeID := "srv1"
	state := &boardRenderState{viewMode: "all"}
	in := boardInteractiveInput{
		dataDir:    dataDir,
		projectID:  projectID,
		nodeID:     nodeID,
		state:      state,
		policyURL:  "",
	}

	for _, raw := range []string{"create First issue", "create Second issue", "create Third issue"} {
		quit, msg := applyBoardInteractiveCommand(raw, in)
		if quit {
			t.Fatalf("unexpected quit for %q", raw)
		}
		if !strings.Contains(msg, "created OPS-") {
			t.Fatalf("expected created message for %q, got %q", raw, msg)
		}
	}

	logDB, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	all, err := logDB.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	board := projectBoardFromEvents(projectID, all)
	if got := len(board["todo"]); got != 3 {
		t.Fatalf("unexpected todo count: got=%d want=3", got)
	}

	seen := map[string]bool{}
	for _, it := range board["todo"] {
		seen[it.ID] = true
	}
	for _, id := range []string{"OPS-1", "OPS-2", "OPS-3"} {
		if !seen[id] {
			t.Fatalf("missing issue id %s in todo column", id)
		}
	}
}

func TestInteractiveHelpTextHasExamples(t *testing.T) {
	help := interactiveHelpText()
	required := []string{
		"create <ISSUE_ID> <summary>",
		"move <ISSUE_ID> <from> <to>",
		"comment <ISSUE_ID> <text>",
		"view mine [assignee]",
		"example: create OPS-901 Fix auth timeout",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("help missing token %q", token)
		}
	}
}

func TestFilterAuditEventsByTimeAndUser(t *testing.T) {
	in := []audit.Event{
		{Time: "2026-02-12T10:00:00Z", Type: "team.onboard", Actor: "u1", Status: "ok"},
		{Time: "2026-02-12T11:00:00Z", Type: "team.role_change", Actor: "u2", Status: "ok"},
		{Time: "2026-02-12T12:00:00Z", Type: "trust.invite", Actor: "u1", Status: "ok"},
	}
	from, to, err := parseAuditTimeRange("2026-02-12T10:30:00Z", "2026-02-12T12:00:00Z")
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	users := parseUserFilter("u1")
	out := filterAuditEvents(in, from, to, users)
	if len(out) != 1 {
		t.Fatalf("unexpected filtered size: got=%d want=1", len(out))
	}
	if out[0].Type != "trust.invite" {
		t.Fatalf("unexpected event type: got=%s", out[0].Type)
	}
}

func TestParseAuditTimeRangeRejectsInvertedRange(t *testing.T) {
	_, _, err := parseAuditTimeRange("2026-02-12T12:00:00Z", "2026-02-12T11:00:00Z")
	if err == nil {
		t.Fatalf("expected range error")
	}
}

func TestPaginateAuditEvents(t *testing.T) {
	in := []audit.Event{
		{Time: "2026-02-12T10:00:00Z", Type: "a"},
		{Time: "2026-02-12T11:00:00Z", Type: "b"},
		{Time: "2026-02-12T12:00:00Z", Type: "c"},
	}
	page1, next, err := paginateAuditEvents(in, 2, "")
	if err != nil {
		t.Fatalf("paginate page1: %v", err)
	}
	if len(page1) != 2 || next != "2" {
		t.Fatalf("unexpected page1 len=%d next=%q", len(page1), next)
	}
	page2, next2, err := paginateAuditEvents(in, 2, next)
	if err != nil {
		t.Fatalf("paginate page2: %v", err)
	}
	if len(page2) != 1 || next2 != "" {
		t.Fatalf("unexpected page2 len=%d next=%q", len(page2), next2)
	}
}

func TestSecurityAuditExportEndpoint(t *testing.T) {
	dataDir := t.TempDir()
	am, err := audit.Open(dataDir)
	if err != nil {
		t.Fatalf("open audit: %v", err)
	}
	am.Append("team.onboard", "u1", "ok", map[string]any{"user_id": "dev1"})
	time.Sleep(10 * time.Millisecond)
	am.Append("team.role_change", "u2", "ok", map[string]any{"role": "lead"})

	s := &syncServer{auditManager: am}
	req := httptest.NewRequest(http.MethodGet, "/security/audit/export?user=u2&limit=1", nil)
	w := httptest.NewRecorder()
	s.securityAuditExport(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Events []audit.Event `json:"events"`
		Count  int           `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Count != 1 || len(out.Events) != 1 {
		t.Fatalf("unexpected count=%d len=%d", out.Count, len(out.Events))
	}
	if out.Events[0].Actor != "u2" {
		t.Fatalf("unexpected actor=%q", out.Events[0].Actor)
	}
}

func TestChaosPartitionRejoinDeterministicConvergence(t *testing.T) {
	base := t.TempDir()
	_, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	srvA.syncServer.peers = []string{srvB.url, srvC.url}
	srvB.syncServer.peers = []string{srvA.url, srvC.url}
	srvC.syncServer.peers = []string{srvA.url, srvB.url}

	seed := mathrand.New(mathrand.NewSource(20260212))
	for i := 1; i <= 25; i++ {
		issueID := "OPS-C" + strconv.Itoa(i)
		if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", issueID, "issue.create", map[string]string{
			"status":  "todo",
			"summary": "chaos",
		}); err != nil {
			t.Fatalf("append create %s: %v", issueID, err)
		}
		targetStatus := []string{"in_progress", "code_review", "testing"}[seed.Intn(3)]
		if err := appendIssueEvent(filepath.Join(base, "b"), "node-b", "OPS", issueID, "issue.transition", map[string]string{
			"from": "todo",
			"to":   targetStatus,
		}); err != nil {
			t.Fatalf("append transition %s: %v", issueID, err)
		}

		// Partition/rejoin simulation with deterministic pattern.
		switch i % 3 {
		case 0:
			srvA.syncServer.pullFromPeer(srvB.url)
			srvC.syncServer.pullFromPeer(srvA.url)
		case 1:
			srvB.syncServer.pullFromPeer(srvC.url)
			srvA.syncServer.pullFromPeer(srvC.url)
		default:
			srvC.syncServer.pullFromPeer(srvB.url)
			srvB.syncServer.pullFromPeer(srvA.url)
		}
	}

	for i := 0; i < 8; i++ {
		srvA.syncServer.pullFromPeer(srvB.url)
		srvA.syncServer.pullFromPeer(srvC.url)
		srvB.syncServer.pullFromPeer(srvA.url)
		srvB.syncServer.pullFromPeer(srvC.url)
		srvC.syncServer.pullFromPeer(srvA.url)
		srvC.syncServer.pullFromPeer(srvB.url)
	}

	allA, _ := srvA.syncServer.log.ReadAll()
	allB, _ := srvB.syncServer.log.ReadAll()
	allC, _ := srvC.syncServer.log.ReadAll()
	ba := projectBoardFromEvents("OPS", allA)
	bb := projectBoardFromEvents("OPS", allB)
	bc := projectBoardFromEvents("OPS", allC)
	if fmt.Sprintf("%v", ba) != fmt.Sprintf("%v", bb) || fmt.Sprintf("%v", bb) != fmt.Sprintf("%v", bc) {
		t.Fatalf("boards diverged after chaos/rejoin")
	}
}

func TestLongRunningSyncSoakDeterministic(t *testing.T) {
	base := t.TempDir()
	_, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	srvA.syncServer.peers = []string{srvB.url, srvC.url}
	srvB.syncServer.peers = []string{srvA.url, srvC.url}
	srvC.syncServer.peers = []string{srvA.url, srvB.url}

	for i := 1; i <= 60; i++ {
		issueID := "OPS-S" + strconv.Itoa(i)
		if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", issueID, "issue.create", map[string]string{
			"status":  "todo",
			"summary": "soak",
		}); err != nil {
			t.Fatalf("append create %s: %v", issueID, err)
		}
		if i%2 == 0 {
			if err := appendIssueEvent(filepath.Join(base, "a"), "node-a", "OPS", issueID, "issue.transition", map[string]string{
				"from": "todo",
				"to":   "in_progress",
			}); err != nil {
				t.Fatalf("append transition %s: %v", issueID, err)
			}
		}
		srvB.syncServer.pullFromPeer(srvA.url)
		srvC.syncServer.pullFromPeer(srvA.url)
		if i%5 == 0 {
			srvA.syncServer.pullFromPeer(srvB.url)
			srvA.syncServer.pullFromPeer(srvC.url)
		}
	}

	allA, _ := srvA.syncServer.log.ReadAll()
	allB, _ := srvB.syncServer.log.ReadAll()
	allC, _ := srvC.syncServer.log.ReadAll()
	if len(allA) != len(allB) || len(allB) != len(allC) {
		t.Fatalf("event log lengths diverged in soak: a=%d b=%d c=%d", len(allA), len(allB), len(allC))
	}
}

func TestGovernanceReconfigureStressNoDivergence(t *testing.T) {
	base := t.TempDir()
	_, srvA, cleanupA := newSyncServerForTest(t, filepath.Join(base, "a"), "node-a", "OPS")
	defer cleanupA()
	_, srvB, cleanupB := newSyncServerForTest(t, filepath.Join(base, "b"), "node-b", "OPS")
	defer cleanupB()
	_, srvC, cleanupC := newSyncServerForTest(t, filepath.Join(base, "c"), "node-c", "OPS")
	defer cleanupC()

	for _, srv := range []*testNodeServer{srvA, srvB, srvC} {
		if err := srv.syncServer.govManager.SetNodeRole("node-a", "voting", true); err != nil {
			t.Fatalf("seed voting a: %v", err)
		}
		if err := srv.syncServer.govManager.SetNodeRole("node-b", "voting", true); err != nil {
			t.Fatalf("seed voting b: %v", err)
		}
		if err := srv.syncServer.govManager.SetNodeRole("node-c", "voting", true); err != nil {
			t.Fatalf("seed voting c: %v", err)
		}
	}

	sets := [][]string{
		{"node-a", "node-b", "node-c"},
		{"node-a", "node-c", "node-d"},
		{"node-a", "node-b", "node-e"},
		{"node-b", "node-c", "node-f"},
	}
	for i := 0; i < 20; i++ {
		vset := sets[i%len(sets)]
		body, _ := json.Marshal(map[string]any{"project_id": "OPS", "voting_nodes": vset})
		for _, srv := range []*testNodeServer{srvA, srvB, srvC} {
			req, _ := http.NewRequest(http.MethodPost, srv.url+"/governance/reconfigure", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("stress reconfigure request: %v", err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("reconfigure status=%d want=200", res.StatusCode)
			}
		}
	}

	lA := srvA.syncServer.govManager.List()
	lB := srvB.syncServer.govManager.List()
	lC := srvC.syncServer.govManager.List()
	if fmt.Sprintf("%v", lA) != fmt.Sprintf("%v", lB) || fmt.Sprintf("%v", lB) != fmt.Sprintf("%v", lC) {
		t.Fatalf("governance state diverged after stress")
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
	req.Header.Set("X-Request-Id", "req-master-777")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusForbidden)
	}
}

func TestSensitiveEndpointsUseStricterRateLimit(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "rl"), "node-s", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"admin-token": {UserID: "admin1", Role: "admin", Active: true},
	}
	srv.syncServer.rateLimiter = newSimpleRateLimiter(5)
	srv.syncServer.sensitiveRateLimiter = newSimpleRateLimiter(1)

	req1, err := http.NewRequest(http.MethodGet, srv.url+"/metrics", nil)
	if err != nil {
		t.Fatalf("new request metrics #1: %v", err)
	}
	req1.Header.Set("Authorization", "Bearer admin-token")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("do metrics #1: %v", err)
	}
	defer res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("metrics #1 status=%d want=%d", res1.StatusCode, http.StatusOK)
	}

	req2, err := http.NewRequest(http.MethodGet, srv.url+"/metrics", nil)
	if err != nil {
		t.Fatalf("new request metrics #2: %v", err)
	}
	req2.Header.Set("Authorization", "Bearer admin-token")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do metrics #2: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("metrics #2 status=%d want=%d", res2.StatusCode, http.StatusOK)
	}

	body := []byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)
	req3, err := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request raft #1: %v", err)
	}
	req3.Header.Set("Authorization", "Bearer admin-token")
	req3.Header.Set("Content-Type", "application/json")
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("do raft #1: %v", err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusOK {
		t.Fatalf("raft #1 status=%d want=%d", res3.StatusCode, http.StatusOK)
	}

	req4, err := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request raft #2: %v", err)
	}
	req4.Header.Set("Authorization", "Bearer admin-token")
	req4.Header.Set("Content-Type", "application/json")
	res4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatalf("do raft #2: %v", err)
	}
	defer res4.Body.Close()
	if res4.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("raft #2 status=%d want=%d", res4.StatusCode, http.StatusTooManyRequests)
	}
}

func TestMasterAPIBootstrapEndpoints(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "master"), "srv1", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"admin-token": {UserID: "admin1", Role: "admin", Active: true},
	}

	req1, err := http.NewRequest(http.MethodGet, srv.url+"/master/health", nil)
	if err != nil {
		t.Fatalf("new request master/health: %v", err)
	}
	req1.Header.Set("Authorization", "Bearer admin-token")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("do master/health: %v", err)
	}
	defer res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("master/health status=%d want=%d", res1.StatusCode, http.StatusOK)
	}
	var h map[string]any
	if err := json.NewDecoder(res1.Body).Decode(&h); err != nil {
		t.Fatalf("decode master/health: %v", err)
	}
	if strings.TrimSpace(fmt.Sprint(h["status"])) != "ok" {
		t.Fatalf("master/health unexpected status: %v", h["status"])
	}

	req2, err := http.NewRequest(http.MethodGet, srv.url+"/master/capabilities", nil)
	if err != nil {
		t.Fatalf("new request master/capabilities: %v", err)
	}
	req2.Header.Set("Authorization", "Bearer admin-token")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do master/capabilities: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("master/capabilities status=%d want=%d", res2.StatusCode, http.StatusOK)
	}
	var c map[string]any
	if err := json.NewDecoder(res2.Body).Decode(&c); err != nil {
		t.Fatalf("decode master/capabilities: %v", err)
	}
	if strings.TrimSpace(fmt.Sprint(c["status"])) != "ok" {
		t.Fatalf("master/capabilities unexpected status: %v", c["status"])
	}
}

func TestMasterIssueCreateAlias(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "master-issues"), "srv1", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"dev-token": {UserID: "dev1", Role: "dev", Active: true},
	}

	body := []byte(`{"project_id":"OPS","issue_id":"OPS-777","summary":"master alias create","priority":"medium","assignee":""}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/master/issues/create", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer dev-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req-master-777")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d want=%d", res.StatusCode, http.StatusCreated)
	}

	all, err := srv.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if !issueExistsInEvents("OPS", "OPS-777", all) {
		t.Fatalf("issue OPS-777 was not created via master alias")
	}
}

func TestMasterIssueCreateIdempotencyByRequestID(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "master-idem"), "srv1", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"dev-token": {UserID: "dev1", Role: "dev", Active: true},
	}

	body := []byte(`{"project_id":"OPS","issue_id":"OPS-778","summary":"idem create","priority":"medium","assignee":""}`)
	req1, _ := http.NewRequest(http.MethodPost, srv.url+"/master/issues/create", bytes.NewReader(body))
	req1.Header.Set("Authorization", "Bearer dev-token")
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Request-Id", "req-778")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	defer res1.Body.Close()
	if res1.StatusCode != http.StatusCreated {
		t.Fatalf("first status=%d want=%d", res1.StatusCode, http.StatusCreated)
	}

	req2, _ := http.NewRequest(http.MethodPost, srv.url+"/master/issues/create", bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer dev-token")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Request-Id", "req-778")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("second status=%d want=%d", res2.StatusCode, http.StatusOK)
	}

	all, err := srv.syncServer.log.ReadAll()
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	createCount := 0
	for _, e := range all {
		if e.ProjectID == "OPS" && e.EntityID == "OPS-778" && e.Type == "issue.create" {
			createCount++
		}
	}
	if createCount != 1 {
		t.Fatalf("issue.create count=%d want=1", createCount)
	}
}

func TestMasterControlAliasesTeamAuthTrustGovernance(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "master-control"), "srv1", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"admin-token": {UserID: "admin1", Role: "admin", Active: true},
		"lead-token":  {UserID: "lead1", Role: "lead", Active: true},
	}

	// team alias: onboard
	teamBody := []byte(`{"user_id":"u-master","role":"dev","duty":false}`)
	reqTeam, _ := http.NewRequest(http.MethodPost, srv.url+"/master/team/onboard", bytes.NewReader(teamBody))
	reqTeam.Header.Set("Authorization", "Bearer lead-token")
	reqTeam.Header.Set("Content-Type", "application/json")
	reqTeam.Header.Set("X-Request-Id", "req-master-team-1")
	resTeam, err := http.DefaultClient.Do(reqTeam)
	if err != nil {
		t.Fatalf("master/team/onboard request failed: %v", err)
	}
	defer resTeam.Body.Close()
	if resTeam.StatusCode != http.StatusCreated {
		t.Fatalf("master/team/onboard status=%d want=%d", resTeam.StatusCode, http.StatusCreated)
	}

	// auth alias: list
	reqAuth, _ := http.NewRequest(http.MethodGet, srv.url+"/master/auth/list", nil)
	reqAuth.Header.Set("Authorization", "Bearer admin-token")
	resAuth, err := http.DefaultClient.Do(reqAuth)
	if err != nil {
		t.Fatalf("master/auth/list request failed: %v", err)
	}
	defer resAuth.Body.Close()
	if resAuth.StatusCode != http.StatusOK {
		t.Fatalf("master/auth/list status=%d want=%d", resAuth.StatusCode, http.StatusOK)
	}

	// trust alias: list
	reqTrust, _ := http.NewRequest(http.MethodGet, srv.url+"/master/trust/list", nil)
	reqTrust.Header.Set("Authorization", "Bearer lead-token")
	resTrust, err := http.DefaultClient.Do(reqTrust)
	if err != nil {
		t.Fatalf("master/trust/list request failed: %v", err)
	}
	defer resTrust.Body.Close()
	if resTrust.StatusCode != http.StatusOK {
		t.Fatalf("master/trust/list status=%d want=%d", resTrust.StatusCode, http.StatusOK)
	}

	// governance alias: node-role
	govBody := []byte(`{"project_id":"OPS","node_id":"srv2","role":"voting","active":true}`)
	reqGov, _ := http.NewRequest(http.MethodPost, srv.url+"/master/governance/node-role", bytes.NewReader(govBody))
	reqGov.Header.Set("Authorization", "Bearer lead-token")
	reqGov.Header.Set("Content-Type", "application/json")
	reqGov.Header.Set("X-Request-Id", "req-master-gov-1")
	resGov, err := http.DefaultClient.Do(reqGov)
	if err != nil {
		t.Fatalf("master/governance/node-role request failed: %v", err)
	}
	defer resGov.Body.Close()
	if resGov.StatusCode != http.StatusOK {
		t.Fatalf("master/governance/node-role status=%d want=%d", resGov.StatusCode, http.StatusOK)
	}
}

func TestMasterClusterControlEndpoints(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "master-cluster"), "srv1", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"lead-token": {UserID: "lead1", Role: "lead", Active: true},
	}
	srv.syncServer.peers = []string{"http://srv2:4101", "http://srv3:4101"}

	reqHealth, _ := http.NewRequest(http.MethodGet, srv.url+"/master/cluster/health", nil)
	reqHealth.Header.Set("Authorization", "Bearer lead-token")
	resHealth, err := http.DefaultClient.Do(reqHealth)
	if err != nil {
		t.Fatalf("master/cluster/health request failed: %v", err)
	}
	defer resHealth.Body.Close()
	if resHealth.StatusCode != http.StatusOK {
		t.Fatalf("master/cluster/health status=%d want=%d", resHealth.StatusCode, http.StatusOK)
	}

	reqNodes, _ := http.NewRequest(http.MethodGet, srv.url+"/master/cluster/nodes", nil)
	reqNodes.Header.Set("Authorization", "Bearer lead-token")
	resNodes, err := http.DefaultClient.Do(reqNodes)
	if err != nil {
		t.Fatalf("master/cluster/nodes request failed: %v", err)
	}
	defer resNodes.Body.Close()
	if resNodes.StatusCode != http.StatusOK {
		t.Fatalf("master/cluster/nodes status=%d want=%d", resNodes.StatusCode, http.StatusOK)
	}

	body := []byte(`{"project_id":"OPS","voting_nodes":["srv1","srv2","srv3"]}`)
	reqReconf, _ := http.NewRequest(http.MethodPost, srv.url+"/master/cluster/reconfigure", bytes.NewReader(body))
	reqReconf.Header.Set("Authorization", "Bearer lead-token")
	reqReconf.Header.Set("Content-Type", "application/json")
	reqReconf.Header.Set("X-Request-Id", "req-master-cluster-reconf-1")
	resReconf, err := http.DefaultClient.Do(reqReconf)
	if err != nil {
		t.Fatalf("master/cluster/reconfigure request failed: %v", err)
	}
	defer resReconf.Body.Close()
	if resReconf.StatusCode != http.StatusOK {
		t.Fatalf("master/cluster/reconfigure status=%d want=%d", resReconf.StatusCode, http.StatusOK)
	}
}

func TestRateLimitKeyUsesStableClientIP(t *testing.T) {
	s := &syncServer{}
	req1, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if err != nil {
		t.Fatalf("new request #1: %v", err)
	}
	req1.RemoteAddr = "10.20.30.40:51001"
	key1 := s.rateLimitKey(req1)

	req2, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if err != nil {
		t.Fatalf("new request #2: %v", err)
	}
	req2.RemoteAddr = "10.20.30.40:51099"
	key2 := s.rateLimitKey(req2)

	if key1 != "ip:10.20.30.40" {
		t.Fatalf("key1=%q want=%q", key1, "ip:10.20.30.40")
	}
	if key2 != "ip:10.20.30.40" {
		t.Fatalf("key2=%q want=%q", key2, "ip:10.20.30.40")
	}
}

func TestRateLimitKeyUsesForwardedIP(t *testing.T) {
	s := &syncServer{}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.RemoteAddr = "127.0.0.1:49999"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	key := s.rateLimitKey(req)
	if key != "ip:198.51.100.7" {
		t.Fatalf("key=%q want=%q", key, "ip:198.51.100.7")
	}
}

func TestRateLimitIsPerTokenBucket(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "rl-token"), "node-s", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"admin-a": {UserID: "admin-a", Role: "admin", Active: true},
		"admin-b": {UserID: "admin-b", Role: "admin", Active: true},
	}
	srv.syncServer.sensitiveRateLimiter = newSimpleRateLimiter(1)

	body := []byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)
	call := func(token string) int {
		req, err := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	if got := call("admin-a"); got != http.StatusOK {
		t.Fatalf("admin-a first status=%d want=%d", got, http.StatusOK)
	}
	if got := call("admin-a"); got != http.StatusTooManyRequests {
		t.Fatalf("admin-a second status=%d want=%d", got, http.StatusTooManyRequests)
	}
	if got := call("admin-b"); got != http.StatusOK {
		t.Fatalf("admin-b first status=%d want=%d", got, http.StatusOK)
	}
}

func TestRateLimitIsPerIPBucketWithoutAuth(t *testing.T) {
	s := &syncServer{
		authEnabled: false,
		rateLimiter: newSimpleRateLimiter(1),
	}
	req1, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if err != nil {
		t.Fatalf("new request #1: %v", err)
	}
	req1.Header.Set("X-Forwarded-For", "203.0.113.10")
	req1.RemoteAddr = "127.0.0.1:51001"
	if !s.allowRequest(req1) {
		t.Fatalf("first request for ip1 should be allowed")
	}

	req2, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if err != nil {
		t.Fatalf("new request #2: %v", err)
	}
	req2.Header.Set("X-Forwarded-For", "203.0.113.10")
	req2.RemoteAddr = "127.0.0.1:51099"
	if s.allowRequest(req2) {
		t.Fatalf("second request for ip1 should be denied by rate limit")
	}

	req3, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if err != nil {
		t.Fatalf("new request #3: %v", err)
	}
	req3.Header.Set("X-Forwarded-For", "203.0.113.11")
	req3.RemoteAddr = "127.0.0.1:51111"
	if !s.allowRequest(req3) {
		t.Fatalf("first request for ip2 should be allowed")
	}
}

func TestMetricsExposeSecurityCounters(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "metrics"), "node-s", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"admin-token": {UserID: "admin1", Role: "admin", Active: true},
		"dev-token":   {UserID: "dev1", Role: "dev", Active: true},
	}
	srv.syncServer.rateLimiter = newSimpleRateLimiter(1)
	srv.syncServer.sensitiveRateLimiter = newSimpleRateLimiter(1)

	mustDo := func(req *http.Request) *http.Response {
		t.Helper()
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		return res
	}

	reqAuthn, _ := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader([]byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)))
	reqAuthn.Header.Set("Content-Type", "application/json")
	resAuthn := mustDo(reqAuthn)
	resAuthn.Body.Close()

	reqAuthz, _ := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader([]byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)))
	reqAuthz.Header.Set("Authorization", "Bearer dev-token")
	reqAuthz.Header.Set("Content-Type", "application/json")
	resAuthz := mustDo(reqAuthz)
	resAuthz.Body.Close()

	reqOK, _ := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader([]byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)))
	reqOK.Header.Set("Authorization", "Bearer admin-token")
	reqOK.Header.Set("Content-Type", "application/json")
	resOK := mustDo(reqOK)
	resOK.Body.Close()

	reqRate, _ := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader([]byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)))
	reqRate.Header.Set("Authorization", "Bearer admin-token")
	reqRate.Header.Set("Content-Type", "application/json")
	resRate := mustDo(reqRate)
	resRate.Body.Close()

	mReq, _ := http.NewRequest(http.MethodGet, srv.url+"/metrics", nil)
	mRes := mustDo(mReq)
	defer mRes.Body.Close()
	if mRes.StatusCode != http.StatusOK {
		t.Fatalf("metrics status=%d want=%d", mRes.StatusCode, http.StatusOK)
	}
	var out map[string]any
	if err := json.NewDecoder(mRes.Body).Decode(&out); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}

	if int(out["authn_denied_total"].(float64)) < 1 {
		t.Fatalf("authn_denied_total=%v want>=1", out["authn_denied_total"])
	}
	if int(out["authz_denied_total"].(float64)) < 1 {
		t.Fatalf("authz_denied_total=%v want>=1", out["authz_denied_total"])
	}
	if int(out["rate_limit_denied_total"].(float64)) < 1 {
		t.Fatalf("rate_limit_denied_total=%v want>=1", out["rate_limit_denied_total"])
	}
	if int(out["rate_limit_denied_sensitive"].(float64)) < 1 {
		t.Fatalf("rate_limit_denied_sensitive=%v want>=1", out["rate_limit_denied_sensitive"])
	}
}

func TestPeerBackoffStateProgression(t *testing.T) {
	s := &syncServer{}
	peer := "http://127.0.0.1:4102"
	now := time.Unix(1_700_000_000, 0).UTC()

	if !s.shouldPullPeer(peer, now) {
		t.Fatalf("fresh peer should be pullable")
	}

	s.recordPeerPullResult(peer, false, now)
	if s.shouldPullPeer(peer, now) {
		t.Fatalf("peer should be in backoff after first failure")
	}
	if !s.shouldPullPeer(peer, now.Add(2*time.Second)) {
		t.Fatalf("peer should exit first backoff window")
	}

	s.recordPeerPullResult(peer, false, now.Add(2*time.Second))
	if !s.shouldPullPeer(peer, now.Add(5*time.Second)) {
		t.Fatalf("peer should exit second (2s) backoff window")
	}

	s.recordPeerPullResult(peer, true, now.Add(5*time.Second))
	if !s.shouldPullPeer(peer, now.Add(5*time.Second)) {
		t.Fatalf("peer should be immediately pullable after success reset")
	}
}

func TestPeerBackoffActiveCount(t *testing.T) {
	s := &syncServer{}
	now := time.Unix(1_700_000_100, 0).UTC()
	s.recordPeerPullResult("http://127.0.0.1:4102", false, now)
	s.recordPeerPullResult("http://127.0.0.1:4103", true, now)
	if got := s.peerBackoffActive(now); got != 1 {
		t.Fatalf("active backoff peers=%d want=1", got)
	}
}

func TestRefreshDiscoveredPeersTrustGated(t *testing.T) {
	base := t.TempDir()
	tm, err := trust.Open(base)
	if err != nil {
		t.Fatalf("open trust: %v", err)
	}
	_ = tm.TrustNode("node-b")
	_ = tm.TrustNode("node-c")

	candidateC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "node_id": "node-c"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}))
	defer candidateC.Close()

	candidateD := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "node_id": "node-d"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}))
	defer candidateD.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/peers" {
			writeJSON(w, http.StatusOK, map[string]any{
				"node_id": "node-b",
				"peers":   []string{candidateC.URL, candidateD.URL},
			})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}))
	defer source.Close()

	s := &syncServer{
		peers:           []string{source.URL},
		httpClient:      http.DefaultClient,
		trustManager:    tm,
		discoveryTTL:    45 * time.Second,
		discoveredPeers: map[string]int64{},
		peerNodeByURL:   map[string]string{},
	}
	s.refreshDiscoveredPeers(time.Now().UTC())

	s.peerMu.Lock()
	_, hasC := s.discoveredPeers[normalizePeerURL(candidateC.URL)]
	_, hasD := s.discoveredPeers[normalizePeerURL(candidateD.URL)]
	s.peerMu.Unlock()
	if !hasC {
		t.Fatalf("trusted peer C should be discovered")
	}
	if hasD {
		t.Fatalf("untrusted peer D should not be discovered")
	}
}

func TestPruneDiscoveredPeersByTTLAndTrust(t *testing.T) {
	base := t.TempDir()
	tm, err := trust.Open(base)
	if err != nil {
		t.Fatalf("open trust: %v", err)
	}
	_ = tm.TrustNode("node-c")
	_ = tm.TrustNode("node-e")

	s := &syncServer{
		trustManager:    tm,
		discoveryTTL:    10 * time.Second,
		discoveredPeers: map[string]int64{},
		peerNodeByURL:   map[string]string{},
		peerSync:        map[string]peerSyncState{},
	}
	now := time.Unix(1_700_000_500, 0).UTC()
	s.addDiscoveredPeer("http://peer-c:4101", "node-c", now)
	s.addDiscoveredPeer("http://peer-e:4101", "node-e", now.Add(-20*time.Second))

	if err := tm.RevokeNode("node-c"); err != nil {
		t.Fatalf("revoke node-c: %v", err)
	}
	s.pruneDiscoveredPeers(now)

	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if _, ok := s.discoveredPeers["http://peer-c:4101"]; ok {
		t.Fatalf("revoked trusted peer should be pruned")
	}
	if _, ok := s.discoveredPeers["http://peer-e:4101"]; ok {
		t.Fatalf("expired peer should be pruned by ttl")
	}
}

func TestMTLSConfigRequiresCA(t *testing.T) {
	_, err := buildServerTLSConfig("", "", true)
	if err == nil {
		t.Fatalf("expected error when mtls is enabled without CA file")
	}
}

func TestMTLSRejectsExpiredClientCertRuntime(t *testing.T) {
	base := t.TempDir()
	files := writeTestPKI(t, base, true, false)
	srvTLS, err := buildServerTLSConfig(files.caCertFile, "", true)
	if err != nil {
		t.Fatalf("build server tls: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = srvTLS
	srv.StartTLS()
	defer srv.Close()

	cert, err := tls.LoadX509KeyPair(files.expiredCertFile, files.expiredKeyFile)
	if err != nil {
		t.Fatalf("load expired cert: %v", err)
	}
	pool := x509.NewCertPool()
	rawCA, _ := os.ReadFile(files.caCertFile)
	pool.AppendCertsFromPEM(rawCA)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}}}
	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatalf("expected TLS handshake error for expired client cert")
	}
}

func TestMTLSRejectsRevokedClientCertRuntime(t *testing.T) {
	base := t.TempDir()
	files := writeTestPKI(t, base, false, true)
	srvTLS, err := buildServerTLSConfig(files.caCertFile, files.crlFile, true)
	if err != nil {
		t.Fatalf("build server tls: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = srvTLS
	srv.StartTLS()
	defer srv.Close()

	cert, err := tls.LoadX509KeyPair(files.clientCertFile, files.clientKeyFile)
	if err != nil {
		t.Fatalf("load client cert: %v", err)
	}
	pool := x509.NewCertPool()
	rawCA, _ := os.ReadFile(files.caCertFile)
	pool.AppendCertsFromPEM(rawCA)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}}}
	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatalf("expected TLS handshake error for revoked client cert")
	}
}

func TestServeSecurityPolicyAllowsSecurePeerSetup(t *testing.T) {
	err := validateServeSecurityPolicy(serveSecurityPolicyInput{
		SecureModeRequired: true,
		AuthEnabled:        true,
		HasPeers:           true,
		TLSCert:            "server.pem",
		TLSKey:             "server.key",
		TLSCA:              "ca.pem",
		MTLSRequired:       true,
		ClientCert:         "client.pem",
		ClientKey:          "client.key",
		PeerToken:          "token",
	})
	if err != nil {
		t.Fatalf("expected secure policy to pass, got error: %v", err)
	}
}

func TestServeSecurityPolicyRejectsPeersWithoutAuth(t *testing.T) {
	err := validateServeSecurityPolicy(serveSecurityPolicyInput{
		SecureModeRequired: true,
		AuthEnabled:        false,
		HasPeers:           true,
		TLSCert:            "server.pem",
		TLSKey:             "server.key",
		TLSCA:              "ca.pem",
		ClientCert:         "client.pem",
		ClientKey:          "client.key",
		PeerToken:          "token",
	})
	if err == nil {
		t.Fatalf("expected error for peers without auth in secure mode")
	}
}

func TestServeSecurityPolicyRejectsPartialTLSKeyPair(t *testing.T) {
	err := validateServeSecurityPolicy(serveSecurityPolicyInput{
		SecureModeRequired: false,
		TLSCert:            "server.pem",
	})
	if err == nil {
		t.Fatalf("expected error for partial server tls pair")
	}
}

func TestServeSecurityPolicyRejectsMTLSWithoutClientCert(t *testing.T) {
	err := validateServeSecurityPolicy(serveSecurityPolicyInput{
		SecureModeRequired: false,
		MTLSRequired:       true,
		TLSCert:            "server.pem",
		TLSKey:             "server.key",
		TLSCA:              "ca.pem",
	})
	if err == nil {
		t.Fatalf("expected error for mtls without client cert/key")
	}
}

func TestStorageRecoveryDrillWritesAuditEvidence(t *testing.T) {
	base := t.TempDir()
	logDB, err := store.Open(base)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	e := events.SignedEvent{
		Version:   1,
		ProjectID: "OPS",
		EntityID:  "OPS-901",
		Type:      "issue.create",
		Payload:   []byte(`{"status":"todo","summary":"drill"}`),
		SignerID:  "node-1",
		SignerPub: []byte("pub"),
		Signature: []byte("sig"),
		Seq:       1,
		Timestamp: time.Now().UTC(),
	}
	if err := logDB.Append(e); err != nil {
		t.Fatalf("append seed event: %v", err)
	}

	runStorage([]string{"recovery-drill", "--data-dir", base})

	am, err := audit.Open(base)
	if err != nil {
		t.Fatalf("open audit: %v", err)
	}
	items, err := am.ReadTail(50)
	if err != nil {
		t.Fatalf("read audit tail: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Type == "storage.key.recovery_drill" && it.Status == "ok" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected storage.key.recovery_drill audit evidence")
	}
}

func TestAuthIssueAndRevokeLifecycle(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "auth"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.authEnabled = true
	srv.syncServer.authTokens = map[string]authPrincipal{
		"admin-token": {UserID: "admin1", Role: "admin", Active: true},
	}
	if srv.syncServer.authManager == nil {
		t.Fatalf("auth manager must be initialized")
	}
	_ = srv.syncServer.authManager.SeedStatic(map[string]authstore.Principal{
		"admin-token": {UserID: "admin1", Role: "admin", Active: true},
	})

	issueReq, _ := http.NewRequest(http.MethodPost, srv.url+"/auth/issue", bytes.NewReader([]byte(`{"user_id":"dev-x","role":"dev","ttl_sec":3600}`)))
	issueReq.Header.Set("Authorization", "Bearer admin-token")
	issueReq.Header.Set("Content-Type", "application/json")
	issueRes, err := http.DefaultClient.Do(issueReq)
	if err != nil {
		t.Fatalf("issue do: %v", err)
	}
	defer issueRes.Body.Close()
	if issueRes.StatusCode != http.StatusCreated {
		t.Fatalf("issue status=%d want=%d", issueRes.StatusCode, http.StatusCreated)
	}
	var rec map[string]any
	if err := json.NewDecoder(issueRes.Body).Decode(&rec); err != nil {
		t.Fatalf("decode issued token: %v", err)
	}
	token, _ := rec["token"].(string)
	if strings.TrimSpace(token) == "" {
		t.Fatalf("issued token is empty")
	}

	protectedReq, _ := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader([]byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)))
	protectedReq.Header.Set("Authorization", "Bearer "+token)
	protectedReq.Header.Set("Content-Type", "application/json")
	protectedRes, err := http.DefaultClient.Do(protectedReq)
	if err != nil {
		t.Fatalf("protected do: %v", err)
	}
	defer protectedRes.Body.Close()
	if protectedRes.StatusCode != http.StatusForbidden {
		t.Fatalf("expected dev token to pass authn then fail authz; got=%d", protectedRes.StatusCode)
	}

	revokeReq, _ := http.NewRequest(http.MethodPost, srv.url+"/auth/revoke", bytes.NewReader([]byte(`{"token":"`+token+`"}`)))
	revokeReq.Header.Set("Authorization", "Bearer admin-token")
	revokeReq.Header.Set("Content-Type", "application/json")
	revokeRes, err := http.DefaultClient.Do(revokeReq)
	if err != nil {
		t.Fatalf("revoke do: %v", err)
	}
	defer revokeRes.Body.Close()
	if revokeRes.StatusCode != http.StatusOK {
		t.Fatalf("revoke status=%d want=%d", revokeRes.StatusCode, http.StatusOK)
	}

	afterReq, _ := http.NewRequest(http.MethodPost, srv.url+"/raft/validate-transition", bytes.NewReader([]byte(`{"project_id":"OPS","issue_id":"OPS-1","from":"todo","to":"in_progress"}`)))
	afterReq.Header.Set("Authorization", "Bearer "+token)
	afterReq.Header.Set("Content-Type", "application/json")
	afterRes, err := http.DefaultClient.Do(afterReq)
	if err != nil {
		t.Fatalf("after revoke do: %v", err)
	}
	defer afterRes.Body.Close()
	if afterRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token must fail authn status=%d want=%d", afterRes.StatusCode, http.StatusUnauthorized)
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

func TestTeamOnboardRejectedWithoutQuorum(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "on"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.peers = []string{"http://127.0.0.1:1"}

	body := []byte(`{"project_id":"OPS","user_id":"dev1","role":"dev"}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/team/onboard", bytes.NewReader(body))
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
	if got := srv.syncServer.teamManager.List(); len(got) != 0 {
		t.Fatalf("team member must not be created on rejected quorum: %+v", got)
	}
}

func TestTeamRoleChangeRejectedWithoutQuorum(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "rc"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.peers = []string{"http://127.0.0.1:1"}
	if err := srv.syncServer.teamManager.Onboard(team.Member{UserID: "dev1", Role: "dev", Active: true}); err != nil {
		t.Fatalf("onboard seed: %v", err)
	}

	body := []byte(`{"project_id":"OPS","user_id":"dev1","role":"lead"}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/team/role-change", bytes.NewReader(body))
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
	if len(members) != 1 || members[0].Role != "dev" {
		t.Fatalf("role must remain unchanged on rejected quorum: %+v", members)
	}
}

func TestGovernanceNodeRoleRejectedWithoutQuorum(t *testing.T) {
	base := t.TempDir()
	_, srv, cleanup := newSyncServerForTest(t, filepath.Join(base, "gn"), "node-1", "OPS")
	defer cleanup()
	srv.syncServer.peers = []string{"http://127.0.0.1:1"}

	body := []byte(`{"project_id":"OPS","node_id":"node-2","role":"voting","active":true}`)
	req, err := http.NewRequest(http.MethodPost, srv.url+"/governance/node-role", bytes.NewReader(body))
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
	if srv.syncServer.govManager.IsVoting("node-2") {
		t.Fatalf("node-2 role must not change on rejected quorum")
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

type testPKIFiles struct {
	caCertFile      string
	clientCertFile  string
	clientKeyFile   string
	expiredCertFile string
	expiredKeyFile  string
	crlFile         string
}

func writeTestPKI(t *testing.T, dir string, wantExpiredClient bool, wantRevokeClient bool) testPKIFiles {
	t.Helper()
	now := time.Now().UTC()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             now.Add(-2 * time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}
	caCertFile := filepath.Join(dir, "ca.pem")
	writePEMFile(t, caCertFile, "CERTIFICATE", caDER)

	makeClient := func(serial int64, notBefore, notAfter time.Time, certFile, keyFile string) *x509.Certificate {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate client key: %v", err)
		}
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: "client"},
			NotBefore:    notBefore,
			NotAfter:     notAfter,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatalf("create client cert: %v", err)
		}
		writePEMFile(t, certFile, "CERTIFICATE", der)
		writePEMFile(t, keyFile, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
		parsed, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("parse client cert: %v", err)
		}
		return parsed
	}

	files := testPKIFiles{
		caCertFile:      caCertFile,
		clientCertFile:  filepath.Join(dir, "client.pem"),
		clientKeyFile:   filepath.Join(dir, "client.key"),
		expiredCertFile: filepath.Join(dir, "expired.pem"),
		expiredKeyFile:  filepath.Join(dir, "expired.key"),
		crlFile:         filepath.Join(dir, "ca.crl.pem"),
	}

	clientCert := makeClient(11, now.Add(-1*time.Hour), now.Add(24*time.Hour), files.clientCertFile, files.clientKeyFile)
	if wantExpiredClient {
		_ = makeClient(12, now.Add(-48*time.Hour), now.Add(-24*time.Hour), files.expiredCertFile, files.expiredKeyFile)
	}
	if wantRevokeClient {
		rlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
			Number:     big.NewInt(1),
			ThisUpdate: now.Add(-1 * time.Hour),
			NextUpdate: now.Add(24 * time.Hour),
			RevokedCertificateEntries: []x509.RevocationListEntry{
				{
					SerialNumber:   clientCert.SerialNumber,
					RevocationTime: now.Add(-30 * time.Minute),
				},
			},
		}, caCert, caKey)
		if err != nil {
			t.Fatalf("create crl: %v", err)
		}
		writePEMFile(t, files.crlFile, "X509 CRL", rlDER)
	}

	return files
}

func writePEMFile(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	raw := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write pem %s: %v", path, err)
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
	tm, err := team.Open(dataDir)
	if err != nil {
		t.Fatalf("open team: %v", err)
	}
	trm, err := trust.Open(dataDir)
	if err != nil {
		t.Fatalf("open trust: %v", err)
	}
	au, err := authstore.Open(dataDir)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
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
		authManager:  au,
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
	mux.HandleFunc("/raft/vote-team-onboard", s.withAuthRoles(s.raftVoteTeamOnboard, "admin", "lead"))
	mux.HandleFunc("/raft/validate-team-onboard", s.withAuthRoles(s.raftValidateTeamOnboard, "admin", "lead"))
	mux.HandleFunc("/raft/vote-team-role-change", s.withAuthRoles(s.raftVoteTeamRoleChange, "admin", "lead"))
	mux.HandleFunc("/raft/validate-team-role-change", s.withAuthRoles(s.raftValidateTeamRoleChange, "admin", "lead"))
	mux.HandleFunc("/raft/vote-trust-invite", s.withAuthRoles(s.raftVoteTrustInvite, "admin", "lead"))
	mux.HandleFunc("/raft/validate-trust-invite", s.withAuthRoles(s.raftValidateTrustInvite, "admin", "lead"))
	mux.HandleFunc("/raft/vote-trust-revoke", s.withAuthRoles(s.raftVoteTrustRevoke, "admin", "lead"))
	mux.HandleFunc("/raft/validate-trust-revoke", s.withAuthRoles(s.raftValidateTrustRevoke, "admin", "lead"))
	mux.HandleFunc("/raft/vote-governance-node-role", s.withAuthRoles(s.raftVoteGovernanceNodeRole, "admin", "lead"))
	mux.HandleFunc("/raft/validate-governance-node-role", s.withAuthRoles(s.raftValidateGovernanceNodeRole, "admin", "lead"))
	mux.HandleFunc("/governance/reconfigure", s.withAuthRoles(s.governanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/team/offboard", s.withAuthRoles(s.teamOffboard, "admin", "lead"))
	mux.HandleFunc("/trust/invite", s.withAuthRoles(s.trustInvite, "admin", "lead"))
	mux.HandleFunc("/trust/revoke", s.withAuthRoles(s.trustRevoke, "admin", "lead"))
	mux.HandleFunc("/trust/list", s.withAuthRoles(s.trustList, "admin", "lead"))
	mux.HandleFunc("/master/trust/invite", s.withAuthRoles(s.withMasterIdempotency(s.trustInvite), "admin", "lead"))
	mux.HandleFunc("/master/trust/revoke", s.withAuthRoles(s.withMasterIdempotency(s.trustRevoke), "admin", "lead"))
	mux.HandleFunc("/master/trust/list", s.withAuthRoles(s.trustList, "admin", "lead"))
	mux.HandleFunc("/master/issues/create", s.withAuthRoles(s.withMasterIdempotency(s.issueCreate), "admin", "lead", "dev", "qa"))
	mux.HandleFunc("/master/issues/transition", s.withAuthRoles(s.withMasterIdempotency(s.issueTransition), "admin", "lead", "dev", "qa"))
	mux.HandleFunc("/master/issues/comment", s.withAuthRoles(s.withMasterIdempotency(s.issueComment), "admin", "lead", "dev", "qa"))
	mux.HandleFunc("/master/issues/archive", s.withAuthRoles(s.withMasterIdempotency(s.issueArchive), "admin", "lead", "dev", "qa"))
	mux.HandleFunc("/master/issues/unarchive", s.withAuthRoles(s.withMasterIdempotency(s.issueUnarchive), "admin", "lead", "dev", "qa"))
	mux.HandleFunc("/team/onboard", s.withAuthRoles(s.teamOnboard, "admin", "lead"))
	mux.HandleFunc("/team/role-change", s.withAuthRoles(s.teamRoleChange, "admin", "lead"))
	mux.HandleFunc("/master/team/onboard", s.withAuthRoles(s.withMasterIdempotency(s.teamOnboard), "admin", "lead"))
	mux.HandleFunc("/master/team/role-change", s.withAuthRoles(s.withMasterIdempotency(s.teamRoleChange), "admin", "lead"))
	mux.HandleFunc("/governance/node-role", s.withAuthRoles(s.governanceNodeRole, "admin", "lead"))
	mux.HandleFunc("/master/governance/node-role", s.withAuthRoles(s.withMasterIdempotency(s.governanceNodeRole), "admin", "lead"))
	mux.HandleFunc("/master/cluster/health", s.withAuthRoles(s.masterClusterHealth, "admin", "lead"))
	mux.HandleFunc("/master/cluster/nodes", s.withAuthRoles(s.masterClusterNodes, "admin", "lead"))
	mux.HandleFunc("/master/cluster/reconfigure", s.withAuthRoles(s.withMasterIdempotency(s.masterClusterReconfigure), "admin", "lead"))
	mux.HandleFunc("/auth/issue", s.withAuthRoles(s.authIssue, "admin"))
	mux.HandleFunc("/auth/revoke", s.withAuthRoles(s.authRevoke, "admin"))
	mux.HandleFunc("/auth/list", s.withAuthRoles(s.authList, "admin", "lead"))
	mux.HandleFunc("/auth/bind-role", s.withAuthRoles(s.authBindRole, "admin"))
	mux.HandleFunc("/auth/list-bindings", s.withAuthRoles(s.authListBindings, "admin", "lead"))
	mux.HandleFunc("/master/auth/issue", s.withAuthRoles(s.withMasterIdempotency(s.authIssue), "admin"))
	mux.HandleFunc("/master/auth/list", s.withAuthRoles(s.authList, "admin", "lead"))
	mux.HandleFunc("/master/health", s.withAuthRoles(s.masterHealth, "admin", "lead"))
	mux.HandleFunc("/master/capabilities", s.withAuthRoles(s.masterCapabilities, "admin", "lead"))
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
