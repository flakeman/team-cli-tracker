package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vladimir/team-cli-tracker/internal/events"
	"github.com/vladimir/team-cli-tracker/internal/node"
	"github.com/vladimir/team-cli-tracker/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}
	switch os.Args[1] {
	case "identity":
		runIdentity(os.Args[2:])
	case "issue":
		runIssue(os.Args[2:])
	case "board":
		runBoard(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	default:
		printUsage()
	}
}

func runIdentity(args []string) {
	fs := flag.NewFlagSet("identity", flag.ExitOnError)
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	_ = fs.Parse(args)

	id, err := node.LoadOrCreate(*dataDir, *nodeID)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("node_id=%s\n", id.NodeID)
	fmt.Printf("public_key_len=%d\n", len(id.Pub))
	fmt.Printf("data_dir=%s\n", *dataDir)
}

func runIssue(args []string) {
	if len(args) < 1 {
		printIssueUsage()
		return
	}
	switch args[0] {
	case "create":
		runIssueCreate(args[1:])
	case "transition":
		runIssueTransition(args[1:])
	case "comment":
		runIssueComment(args[1:])
	default:
		printIssueUsage()
	}
}

func runIssueCreate(args []string) {
	fs := flag.NewFlagSet("issue create", flag.ExitOnError)
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	projectID := fs.String("project-id", "", "project id")
	issueID := fs.String("issue-id", "", "issue id")
	summary := fs.String("summary", "", "issue summary")
	priority := fs.String("priority", "medium", "priority")
	assignee := fs.String("assignee", "", "assignee id")
	_ = fs.Parse(args)

	if *projectID == "" || *issueID == "" || strings.TrimSpace(*summary) == "" {
		fatal(fmt.Errorf("project-id, issue-id and summary are required"))
	}
	payload := map[string]string{
		"status":     "todo",
		"summary":    strings.TrimSpace(*summary),
		"priority":   strings.TrimSpace(*priority),
		"assignee":   strings.TrimSpace(*assignee),
		"created_by": *nodeID,
	}
	if err := appendIssueEvent(*dataDir, *nodeID, *projectID, *issueID, "issue.create", payload); err != nil {
		fatal(err)
	}
	fmt.Println("ok: issue created event appended")
}

func runIssueTransition(args []string) {
	fs := flag.NewFlagSet("issue transition", flag.ExitOnError)
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	projectID := fs.String("project-id", "", "project id")
	issueID := fs.String("issue-id", "", "issue id")
	from := fs.String("from", "", "old status")
	to := fs.String("to", "", "new status")
	policyURL := fs.String("policy-url", envOr("POLICY_URL", ""), "policy endpoint base URL (optional)")
	_ = fs.Parse(args)

	if *projectID == "" || *issueID == "" || *to == "" {
		fatal(fmt.Errorf("project-id, issue-id and to are required"))
	}
	if err := validateTransitionProtected(*policyURL, *projectID, *issueID, *from, *to); err != nil {
		fatal(err)
	}
	payload := map[string]string{
		"from": *from,
		"to":   *to,
	}
	if err := appendIssueEvent(*dataDir, *nodeID, *projectID, *issueID, "issue.transition", payload); err != nil {
		fatal(err)
	}
	fmt.Println("ok: issue transition event appended")
}

func runIssueComment(args []string) {
	fs := flag.NewFlagSet("issue comment", flag.ExitOnError)
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	projectID := fs.String("project-id", "", "project id")
	issueID := fs.String("issue-id", "", "issue id")
	text := fs.String("text", "", "comment text")
	_ = fs.Parse(args)

	if *projectID == "" || *issueID == "" || strings.TrimSpace(*text) == "" {
		fatal(fmt.Errorf("project-id, issue-id and text are required"))
	}
	payload := map[string]string{
		"text": strings.TrimSpace(*text),
	}
	if err := appendIssueEvent(*dataDir, *nodeID, *projectID, *issueID, "issue.comment", payload); err != nil {
		fatal(err)
	}
	fmt.Println("ok: issue comment event appended")
}

func runBoard(args []string) {
	fs := flag.NewFlagSet("board", flag.ExitOnError)
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	projectID := fs.String("project-id", "", "project id")
	format := fs.String("format", "plain", "plain|json")
	_ = fs.Parse(args)
	if *projectID == "" {
		fatal(fmt.Errorf("project-id is required"))
	}

	log, err := store.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	all, err := log.ReadAll()
	if err != nil {
		fatal(err)
	}
	board := projectBoardFromEvents(*projectID, all)
	switch *format {
	case "json":
		raw, _ := json.MarshalIndent(board, "", "  ")
		fmt.Println(string(raw))
	default:
		printBoardPlain(*projectID, board)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	projectID := fs.String("project-id", envOr("PROJECT_ID", "OPS"), "project id")
	listenAddr := fs.String("listen", envOr("LISTEN_ADDR", ":4101"), "http listen address")
	peersCSV := fs.String("peers", envOr("PEERS", ""), "comma-separated peer base URLs")
	tick := fs.Duration("sync-tick", 3*time.Second, "sync interval")
	_ = fs.Parse(args)

	id, err := node.LoadOrCreate(*dataDir, *nodeID)
	if err != nil {
		fatal(err)
	}
	log, err := store.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	peers := parsePeers(*peersCSV)
	s := &syncServer{
		projectID: *projectID,
		nodeID:    id.NodeID,
		log:       log,
		peers:     peers,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/sync/clock", s.syncClock)
	mux.HandleFunc("/sync/events", s.syncEvents)
	mux.HandleFunc("/sync/ingest", s.syncIngest)
	mux.HandleFunc("/raft/vote-transition", s.raftVoteTransition)
	mux.HandleFunc("/raft/validate-transition", s.raftValidateTransition)

	go s.syncLoop(*tick)
	fmt.Printf("node=%s listen=%s project=%s peers=%d\n", *nodeID, *listenAddr, *projectID, len(peers))
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		fatal(err)
	}
}

type syncServer struct {
	projectID string
	nodeID    string
	log       *store.EventLog
	peers     []string
}

type transitionRequest struct {
	ProjectID string `json:"project_id"`
	IssueID   string `json:"issue_id"`
	From      string `json:"from"`
	To        string `json:"to"`
}

func (s *syncServer) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "node_id": s.nodeID})
}

func (s *syncServer) syncClock(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		projectID = s.projectID
	}
	clock, err := s.log.Clock(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project_id": projectID, "clock": clock})
}

func (s *syncServer) syncEvents(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	signerID := strings.TrimSpace(r.URL.Query().Get("signer_id"))
	afterSeqStr := strings.TrimSpace(r.URL.Query().Get("after_seq"))
	if projectID == "" || signerID == "" || afterSeqStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, signer_id, after_seq are required"})
		return
	}
	afterSeq, err := strconv.ParseUint(afterSeqStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_seq"})
		return
	}
	items, err := s.log.EventsAfter(projectID, signerID, afterSeq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items})
}

func (s *syncServer) syncIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in events.SignedEvent
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if in.ProjectID != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if err := events.VerifyByEventKey(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.log.Append(in); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ingested"})
}

func (s *syncServer) raftVoteTransition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in transitionRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if in.ProjectID != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if err := validateWorkflowTransition(in.From, in.To); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": err.Error(), "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID})
}

func (s *syncServer) raftValidateTransition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in transitionRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if in.ProjectID != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if err := validateWorkflowTransition(in.From, in.To); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"allow":   false,
			"reason":  err.Error(),
			"granted": 0,
			"needed":  quorumNeeded(len(s.peers) + 1),
		})
		return
	}

	granted := 1 // local vote
	totalNodes := len(s.peers) + 1
	needed := quorumNeeded(totalNodes)

	for _, peer := range s.peers {
		ok, _ := requestTransitionVote(peer, in)
		if ok {
			granted++
		}
	}
	if granted < needed {
		writeJSON(w, http.StatusOK, map[string]any{
			"allow":   false,
			"reason":  "quorum not reached",
			"granted": granted,
			"needed":  needed,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"allow":   true,
		"granted": granted,
		"needed":  needed,
	})
}

func (s *syncServer) syncLoop(interval time.Duration) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		for _, peer := range s.peers {
			s.pullFromPeer(peer)
		}
	}
}

func (s *syncServer) pullFromPeer(peer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	remoteClock, err := fetchClock(ctx, peer, s.projectID)
	if err != nil {
		return
	}
	localClock, err := s.log.Clock(s.projectID)
	if err != nil {
		return
	}
	for signerID, remoteSeq := range remoteClock {
		localSeq := localClock[signerID]
		if remoteSeq <= localSeq {
			continue
		}
		items, err := fetchEvents(ctx, peer, s.projectID, signerID, localSeq)
		if err != nil {
			continue
		}
		for _, e := range items {
			if err := events.VerifyByEventKey(e); err != nil {
				continue
			}
			_ = s.log.Append(e)
		}
	}
}

func fetchClock(ctx context.Context, peerBaseURL, projectID string) (map[string]uint64, error) {
	u := strings.TrimRight(peerBaseURL, "/") + "/sync/clock?project_id=" + projectID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("clock status=%d", res.StatusCode)
	}
	var out struct {
		Clock map[string]uint64 `json:"clock"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Clock, nil
}

func fetchEvents(ctx context.Context, peerBaseURL, projectID, signerID string, afterSeq uint64) ([]events.SignedEvent, error) {
	u := fmt.Sprintf("%s/sync/events?project_id=%s&signer_id=%s&after_seq=%d",
		strings.TrimRight(peerBaseURL, "/"),
		projectID,
		signerID,
		afterSeq,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("events status=%d", res.StatusCode)
	}
	var out struct {
		Events []events.SignedEvent `json:"events"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Events, nil
}

func appendIssueEvent(dataDir, nodeID, projectID, issueID, eventType string, payload any) error {
	id, err := node.LoadOrCreate(dataDir, nodeID)
	if err != nil {
		return err
	}
	log, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e := events.SignedEvent{
		Version:   1,
		ProjectID: projectID,
		EntityID:  issueID,
		Type:      eventType,
		Payload:   raw,
		SignerID:  id.NodeID,
		SignerPub: id.Pub,
		Seq:       log.NextSeq(projectID, id.NodeID),
		Timestamp: time.Now().UTC(),
	}
	sig, err := events.Sign(id.Priv, e)
	if err != nil {
		return err
	}
	e.Signature = sig
	if err := events.Verify(id.Pub, e); err != nil {
		return err
	}
	return log.Append(e)
}

type issueProjection struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Priority string   `json:"priority"`
	Assignee string   `json:"assignee"`
	Comments []string `json:"comments"`
}

func projectBoardFromEvents(projectID string, all []events.SignedEvent) map[string][]issueProjection {
	issues := make(map[string]issueProjection)
	for _, e := range all {
		if e.ProjectID != projectID {
			continue
		}
		it := issues[e.EntityID]
		if it.ID == "" {
			it = issueProjection{ID: e.EntityID, Status: "todo", Priority: "medium", Comments: make([]string, 0)}
		}
		switch e.Type {
		case "issue.create":
			var p struct {
				Summary  string `json:"summary"`
				Priority string `json:"priority"`
				Assignee string `json:"assignee"`
				Status   string `json:"status"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if strings.TrimSpace(p.Summary) != "" {
				it.Summary = p.Summary
			}
			if strings.TrimSpace(p.Priority) != "" {
				it.Priority = p.Priority
			}
			if strings.TrimSpace(p.Assignee) != "" {
				it.Assignee = p.Assignee
			}
			if strings.TrimSpace(p.Status) != "" {
				it.Status = p.Status
			}
		case "issue.transition":
			var p struct {
				To string `json:"to"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if strings.TrimSpace(p.To) != "" {
				it.Status = p.To
			}
		case "issue.comment":
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if strings.TrimSpace(p.Text) != "" {
				it.Comments = append(it.Comments, p.Text)
			}
		}
		issues[e.EntityID] = it
	}

	out := map[string][]issueProjection{
		"todo":        {},
		"in_progress": {},
		"code_review": {},
		"testing":     {},
		"done":        {},
	}
	for _, v := range issues {
		status := strings.TrimSpace(v.Status)
		if _, ok := out[status]; !ok {
			out[status] = make([]issueProjection, 0)
		}
		out[status] = append(out[status], v)
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool {
			return out[k][i].ID < out[k][j].ID
		})
	}
	return out
}

func printBoardPlain(projectID string, board map[string][]issueProjection) {
	columns := []string{"todo", "in_progress", "code_review", "testing", "done"}
	fmt.Printf("Project: %s\n", projectID)
	for _, c := range columns {
		fmt.Printf("[%s]\n", c)
		if len(board[c]) == 0 {
			fmt.Println("  (empty)")
			continue
		}
		for _, it := range board[c] {
			assignee := it.Assignee
			if assignee == "" {
				assignee = "unassigned"
			}
			fmt.Printf("  - %s | %s | %s | @%s\n", it.ID, it.Summary, it.Priority, assignee)
			if len(it.Comments) > 0 {
				fmt.Printf("    comments: %d\n", len(it.Comments))
			}
		}
	}
}

func printUsage() {
	fmt.Println("team-cli-tracker node")
	fmt.Println("usage:")
	fmt.Println("  node identity --node-id node-1 --data-dir ./data")
	fmt.Println("  node issue create --project-id OPS --issue-id OPS-1 --summary \"...\" [--priority high] [--assignee user]")
	fmt.Println("  node issue transition --project-id OPS --issue-id OPS-1 --from todo --to in_progress [--policy-url http://127.0.0.1:4101]")
	fmt.Println("  node issue comment --project-id OPS --issue-id OPS-1 --text \"...\"")
	fmt.Println("  node board --project-id OPS [--format plain|json]")
	fmt.Println("  node serve --project-id OPS --listen :4101 --peers http://127.0.0.1:4102,http://127.0.0.1:4103")
}

func printIssueUsage() {
	fmt.Println("issue commands: create | transition | comment")
}

func envOr(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func parsePeers(csv string) []string {
	items := strings.Split(csv, ",")
	out := make([]string, 0, len(items))
	for _, v := range items {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			v = "http://" + v
		}
		out = append(out, v)
	}
	return out
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func validateTransitionProtected(policyURL, projectID, issueID, from, to string) error {
	if err := validateWorkflowTransition(from, to); err != nil {
		return err
	}
	if strings.TrimSpace(policyURL) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reqBody := transitionRequest{
		ProjectID: projectID,
		IssueID:   issueID,
		From:      from,
		To:        to,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(policyURL, "/")+"/raft/validate-transition", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("policy status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Allow   bool   `json:"allow"`
		Reason  string `json:"reason"`
		Granted int    `json:"granted"`
		Needed  int    `json:"needed"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return err
	}
	if !out.Allow {
		if out.Reason == "" {
			out.Reason = "transition denied by protected policy"
		}
		return fmt.Errorf("%s (granted=%d needed=%d)", out.Reason, out.Granted, out.Needed)
	}
	return nil
}

func validateWorkflowTransition(from, to string) error {
	allowed := map[string]map[string]struct{}{
		"todo": {
			"in_progress": {},
		},
		"in_progress": {
			"todo":        {},
			"code_review": {},
		},
		"code_review": {
			"in_progress": {},
			"testing":     {},
		},
		"testing": {
			"in_progress": {},
			"done":        {},
		},
		"done": {},
	}
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || from == to {
		return nil
	}
	next, ok := allowed[from]
	if !ok {
		return fmt.Errorf("unknown from status: %s", from)
	}
	if _, ok := next[to]; !ok {
		return fmt.Errorf("transition %s -> %s is not allowed", from, to)
	}
	return nil
}

func requestTransitionVote(peerBaseURL string, in transitionRequest) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-transition", bytes.NewReader(raw))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return false, fmt.Errorf("vote status=%d", res.StatusCode)
	}
	var out struct {
		Allow bool `json:"allow"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Allow, nil
}

func quorumNeeded(total int) int {
	return total/2 + 1
}
