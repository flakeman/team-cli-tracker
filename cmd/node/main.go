package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vladimir/team-cli-tracker/internal/audit"
	"github.com/vladimir/team-cli-tracker/internal/events"
	"github.com/vladimir/team-cli-tracker/internal/governance"
	"github.com/vladimir/team-cli-tracker/internal/node"
	"github.com/vladimir/team-cli-tracker/internal/store"
	"github.com/vladimir/team-cli-tracker/internal/team"
	"github.com/vladimir/team-cli-tracker/internal/trust"
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
	case "storage":
		runStorage(os.Args[2:])
	case "team":
		runTeam(os.Args[2:])
	case "trust":
		runTrust(os.Args[2:])
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

func runStorage(args []string) {
	if len(args) < 1 {
		fmt.Println("storage commands: migrate | enable-encryption | rotate-key | verify-integrity")
		return
	}
	switch args[0] {
	case "migrate":
		fs := flag.NewFlagSet("storage migrate", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		if _, err := store.Open(*dataDir); err != nil {
			fatal(err)
		}
		fmt.Printf("ok: storage migration check complete data-dir=%s\n", *dataDir)
	case "enable-encryption":
		fs := flag.NewFlagSet("storage enable-encryption", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		logDB, err := store.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		keyID, err := logDB.EnableEncryption()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("ok: encryption enabled active_key_id=%s\n", keyID)
	case "rotate-key":
		fs := flag.NewFlagSet("storage rotate-key", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		logDB, err := store.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		keyID, err := logDB.RotateEncryptionKey()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("ok: encryption key rotated active_key_id=%s\n", keyID)
	case "verify-integrity":
		fs := flag.NewFlagSet("storage verify-integrity", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		logDB, err := store.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := logDB.VerifyIntegrity(); err != nil {
			fatal(err)
		}
		fmt.Println("ok: event chain integrity verified")
	default:
		fmt.Println("storage commands: migrate | enable-encryption | rotate-key | verify-integrity")
	}
}

func runTrust(args []string) {
	if len(args) < 1 {
		fmt.Println("trust commands: invite | use-invite | revoke | list")
		return
	}
	switch args[0] {
	case "invite":
		fs := flag.NewFlagSet("trust invite", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		nodeID := fs.String("node-id", "", "node id to invite")
		ttlSec := fs.Int("ttl-sec", 3600, "invite ttl in seconds")
		_ = fs.Parse(args[1:])
		if *nodeID == "" {
			fatal(fmt.Errorf("node-id is required"))
		}
		tm, err := trust.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		token, exp, err := tm.CreateInvite(*nodeID, time.Duration(*ttlSec)*time.Second)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("invite_token=%s\nexpires_at=%s\n", token, exp.UTC().Format(time.RFC3339))
	case "use-invite":
		fs := flag.NewFlagSet("trust use-invite", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		nodeID := fs.String("node-id", "", "node id")
		token := fs.String("token", "", "invite token")
		_ = fs.Parse(args[1:])
		if *nodeID == "" || *token == "" {
			fatal(fmt.Errorf("node-id and token are required"))
		}
		tm, err := trust.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := tm.UseInvite(*token, *nodeID); err != nil {
			fatal(err)
		}
		fmt.Println("ok: node trusted")
	case "revoke":
		fs := flag.NewFlagSet("trust revoke", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		nodeID := fs.String("node-id", "", "node id")
		_ = fs.Parse(args[1:])
		if *nodeID == "" {
			fatal(fmt.Errorf("node-id is required"))
		}
		tm, err := trust.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := tm.RevokeNode(*nodeID); err != nil {
			fatal(err)
		}
		fmt.Println("ok: node revoked")
	case "list":
		fs := flag.NewFlagSet("trust list", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		tm, err := trust.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		raw, _ := json.MarshalIndent(map[string]any{"trusted_nodes": tm.ListTrusted()}, "", "  ")
		fmt.Println(string(raw))
	default:
		fmt.Println("trust commands: invite | use-invite | revoke | list")
	}
}

func runTeam(args []string) {
	if len(args) < 1 {
		fmt.Println("team commands: onboard | role-change | offboard | list")
		return
	}
	switch args[0] {
	case "onboard":
		fs := flag.NewFlagSet("team onboard", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		userID := fs.String("user-id", "", "user id")
		role := fs.String("role", "viewer", "role")
		duty := fs.Bool("duty", false, "duty member")
		_ = fs.Parse(args[1:])
		tm, err := team.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := tm.Onboard(team.Member{UserID: *userID, Role: *role, Duty: *duty, Active: true}); err != nil {
			fatal(err)
		}
		fmt.Printf("ok: onboarded user=%s role=%s duty=%v\n", *userID, *role, *duty)
	case "role-change":
		fs := flag.NewFlagSet("team role-change", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		userID := fs.String("user-id", "", "user id")
		role := fs.String("role", "", "new role")
		duty := fs.Bool("duty", false, "duty member")
		_ = fs.Parse(args[1:])
		tm, err := team.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := tm.ChangeRole(*userID, *role, *duty); err != nil {
			fatal(err)
		}
		fmt.Printf("ok: role changed user=%s role=%s duty=%v\n", *userID, *role, *duty)
	case "offboard":
		fs := flag.NewFlagSet("team offboard", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		projectID := fs.String("project-id", "", "project id")
		nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
		userID := fs.String("user-id", "", "user id")
		_ = fs.Parse(args[1:])
		if *projectID == "" || *userID == "" {
			fatal(fmt.Errorf("project-id and user-id are required"))
		}
		tm, err := team.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		_, err = tm.Offboard(*userID)
		if err != nil {
			fatal(err)
		}
		cnt, err := reassignOffboardedUser(*dataDir, *nodeID, *projectID, *userID, tm)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("ok: offboarded user=%s reassigned_issues=%d\n", *userID, cnt)
	case "list":
		fs := flag.NewFlagSet("team list", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		tm, err := team.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		raw, _ := json.MarshalIndent(map[string]any{"members": tm.List()}, "", "  ")
		fmt.Println(string(raw))
	default:
		fmt.Println("team commands: onboard | role-change | offboard | list")
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
	projectID := fs.String("project-id", envOr("PROJECT_ID", "OPS"), "project id")
	listenAddr := fs.String("listen", envOr("LISTEN_ADDR", ":4101"), "http listen address")
	peersCSV := fs.String("peers", envOr("PEERS", ""), "comma-separated peer base URLs")
	nodeRole := fs.String("node-role", envOr("NODE_ROLE", "member"), "node role: admin|member")
	preferredLeader := fs.String("preferred-leader", envOr("PREFERRED_LEADER", ""), "preferred leader node id (optional)")
	authEnabled := fs.Bool("auth-enabled", envOr("AUTH_ENABLED", "false") == "true", "enable token auth")
	authTokensJSON := fs.String("auth-tokens-json", envOr("AUTH_TOKENS_JSON", ""), "auth token map json")
	peerToken := fs.String("peer-token", envOr("PEER_TOKEN", ""), "outgoing peer bearer token")
	tlsCert := fs.String("tls-cert", envOr("TLS_CERT_FILE", ""), "server TLS cert file")
	tlsKey := fs.String("tls-key", envOr("TLS_KEY_FILE", ""), "server TLS key file")
	tlsCA := fs.String("tls-ca", envOr("TLS_CA_FILE", ""), "CA file for mTLS and client trust")
	mtlsRequired := fs.Bool("mtls-required", envOr("MTLS_REQUIRED", "false") == "true", "require and verify client cert")
	clientCert := fs.String("client-cert", envOr("CLIENT_TLS_CERT_FILE", ""), "client cert for outgoing peer requests")
	clientKey := fs.String("client-key", envOr("CLIENT_TLS_KEY_FILE", ""), "client key for outgoing peer requests")
	rateLimitPerMin := fs.Int("rate-limit-per-min", mustAtoi(envOr("RATE_LIMIT_PER_MIN", "120")), "per-token or per-ip requests per minute")
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
	tokenMap, err := parseAuthTokens(*authTokensJSON)
	if err != nil {
		fatal(err)
	}
	serverTLS, err := buildServerTLSConfig(*tlsCA, *mtlsRequired)
	if err != nil {
		fatal(err)
	}
	httpClient, err := buildPeerHTTPClient(*tlsCA, *clientCert, *clientKey)
	if err != nil {
		fatal(err)
	}
	trustManager, err := trust.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	if err := trustManager.TrustNode(id.NodeID); err != nil {
		fatal(err)
	}
	teamManager, err := team.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	govManager, err := governance.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	if err := govManager.SetNodeRole(id.NodeID, "voting", true); err != nil {
		fatal(err)
	}
	auditManager, err := audit.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	s := &syncServer{
		projectID:       *projectID,
		nodeID:          id.NodeID,
		log:             log,
		peers:           peers,
		nodeRole:        strings.ToLower(strings.TrimSpace(*nodeRole)),
		preferredLeader: strings.TrimSpace(*preferredLeader),
		authEnabled:     *authEnabled,
		authTokens:      tokenMap,
		httpClient:      httpClient,
		peerToken:       strings.TrimSpace(*peerToken),
		trustManager:    trustManager,
		teamManager:     teamManager,
		identity:        id,
		govManager:      govManager,
		auditManager:    auditManager,
		rateLimiter:     newSimpleRateLimiter(*rateLimitPerMin),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/sync/clock", s.withAuthAny(s.syncClock))
	mux.HandleFunc("/sync/events", s.withAuthAny(s.syncEvents))
	mux.HandleFunc("/sync/ingest", s.withAuthAny(s.syncIngest))
	mux.HandleFunc("/raft/vote-transition", s.withAuthRoles(s.raftVoteTransition, "admin", "lead"))
	mux.HandleFunc("/raft/validate-transition", s.withAuthRoles(s.raftValidateTransition, "admin", "lead"))
	mux.HandleFunc("/raft/vote-governance-reconfigure", s.withAuthRoles(s.raftVoteGovernanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/raft/validate-governance-reconfigure", s.withAuthRoles(s.raftValidateGovernanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/raft/vote-team-offboard", s.withAuthRoles(s.raftVoteTeamOffboard, "admin", "lead"))
	mux.HandleFunc("/raft/validate-team-offboard", s.withAuthRoles(s.raftValidateTeamOffboard, "admin", "lead"))
	mux.HandleFunc("/metrics", s.metrics)
	mux.HandleFunc("/trust/invite", s.withAuthRoles(s.trustInvite, "admin", "lead"))
	mux.HandleFunc("/trust/join", s.withAuthAny(s.trustJoin))
	mux.HandleFunc("/trust/revoke", s.withAuthRoles(s.trustRevoke, "admin", "lead"))
	mux.HandleFunc("/trust/list", s.withAuthRoles(s.trustList, "admin", "lead"))
	mux.HandleFunc("/team/onboard", s.withAuthRoles(s.teamOnboard, "admin", "lead"))
	mux.HandleFunc("/team/role-change", s.withAuthRoles(s.teamRoleChange, "admin", "lead"))
	mux.HandleFunc("/team/offboard", s.withAuthRoles(s.teamOffboard, "admin", "lead"))
	mux.HandleFunc("/team/list", s.withAuthRoles(s.teamList, "admin", "lead"))
	mux.HandleFunc("/governance/node-role", s.withAuthRoles(s.governanceNodeRole, "admin", "lead"))
	mux.HandleFunc("/governance/reconfigure", s.withAuthRoles(s.governanceReconfigure, "admin", "lead"))
	mux.HandleFunc("/governance/list", s.withAuthRoles(s.governanceList, "admin", "lead"))
	mux.HandleFunc("/security/audit", s.withAuthRoles(s.securityAudit, "admin", "lead"))

	go s.syncLoop(*tick)
	fmt.Printf("node=%s listen=%s project=%s peers=%d\n", *nodeID, *listenAddr, *projectID, len(peers))
	server := &http.Server{Addr: *listenAddr, Handler: mux, TLSConfig: serverTLS}
	if strings.TrimSpace(*tlsCert) != "" && strings.TrimSpace(*tlsKey) != "" {
		if err := server.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil {
			fatal(err)
		}
		return
	}
	if err := server.ListenAndServe(); err != nil {
		fatal(err)
	}
}

type syncServer struct {
	projectID       string
	nodeID          string
	log             *store.EventLog
	peers           []string
	nodeRole        string
	preferredLeader string
	pulledEvents    uint64
	pullErrors      uint64
	lastSyncUnix    int64
	authEnabled     bool
	authTokens      map[string]authPrincipal
	httpClient      *http.Client
	peerToken       string
	trustManager    *trust.Manager
	teamManager     *team.Manager
	identity        node.Identity
	govManager      *governance.Manager
	auditManager    *audit.Manager
	rateLimiter     *simpleRateLimiter
}

type transitionRequest struct {
	ProjectID string `json:"project_id"`
	IssueID   string `json:"issue_id"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type governanceReconfigureRequest struct {
	ProjectID   string   `json:"project_id"`
	VotingNodes []string `json:"voting_nodes"`
}

type teamOffboardRequest struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
}

type authPrincipal struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
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
	if s.trustManager != nil && !s.trustManager.IsTrusted(in.SignerID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "signer node is not trusted"})
		return
	}
	if err := s.log.Append(in); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ingested"})
}

func (s *syncServer) trustInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		NodeID string `json:"node_id"`
		TTLSec int    `json:"ttl_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if in.TTLSec <= 0 {
		in.TTLSec = 3600
	}
	token, exp, err := s.trustManager.CreateInvite(strings.TrimSpace(in.NodeID), time.Duration(in.TTLSec)*time.Second)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("trust.invite", s.actorFromReq(r), "ok", map[string]any{"node_id": in.NodeID})
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "expires_at": exp.UTC().Format(time.RFC3339), "node_id": in.NodeID})
}

func (s *syncServer) trustJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		NodeID string `json:"node_id"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := s.trustManager.UseInvite(strings.TrimSpace(in.Token), strings.TrimSpace(in.NodeID)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("trust.join", in.NodeID, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "trusted", "node_id": strings.TrimSpace(in.NodeID)})
}

func (s *syncServer) trustRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := s.trustManager.RevokeNode(strings.TrimSpace(in.NodeID)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("trust.revoke", s.actorFromReq(r), "ok", map[string]any{"node_id": in.NodeID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "node_id": strings.TrimSpace(in.NodeID)})
}

func (s *syncServer) trustList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trusted_nodes": s.trustManager.ListTrusted()})
}

func (s *syncServer) teamOnboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		Duty   bool   `json:"duty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if in.Role == "" {
		in.Role = "viewer"
	}
	if err := s.teamManager.Onboard(team.Member{UserID: in.UserID, Role: in.Role, Duty: in.Duty, Active: true}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("team.onboard", s.actorFromReq(r), "ok", map[string]any{"user_id": in.UserID, "role": in.Role})
	writeJSON(w, http.StatusCreated, map[string]any{"status": "onboarded", "user_id": in.UserID, "role": in.Role, "duty": in.Duty})
}

func (s *syncServer) teamRoleChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		Duty   bool   `json:"duty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := s.teamManager.ChangeRole(in.UserID, in.Role, in.Duty); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("team.role_change", s.actorFromReq(r), "ok", map[string]any{"user_id": in.UserID, "role": in.Role, "duty": in.Duty})
	writeJSON(w, http.StatusOK, map[string]any{"status": "role_changed", "user_id": in.UserID, "role": in.Role, "duty": in.Duty})
}

func (s *syncServer) teamOffboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamOffboardRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	if in.ProjectID == "" {
		in.ProjectID = s.projectID
	}
	if in.ProjectID != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	granted, needed, preferredOnline, ok := s.validateTeamOffboardWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":             "rejected",
			"reason":             "quorum not reached",
			"granted":            granted,
			"needed":             needed,
			"mode":               modeName(s.preferredLeader),
			"preferred_leader":   s.preferredLeader,
			"preferred_online":   preferredOnline,
			"failover_activated": s.preferredLeader != "" && !preferredOnline,
		})
		return
	}
	if _, err := s.teamManager.Offboard(in.UserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cnt, err := reassignOffboardedUserByServer(s, in.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("team.offboard", s.actorFromReq(r), "ok", map[string]any{"user_id": in.UserID, "reassigned": cnt})
	writeJSON(w, http.StatusOK, map[string]any{"status": "offboarded", "user_id": in.UserID, "reassigned_issues": cnt})
}

func (s *syncServer) raftVoteTeamOffboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamOffboardRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.UserID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "user_id is required", "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
}

func (s *syncServer) raftValidateTeamOffboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamOffboardRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.UserID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"allow":   false,
			"reason":  "user_id is required",
			"granted": 0,
			"needed":  quorumNeeded(len(s.peers) + 1),
		})
		return
	}
	granted, needed, preferredOnline, ok := s.validateTeamOffboardWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"allow":   false,
			"reason":  "quorum not reached",
			"granted": granted,
			"needed":  needed,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"allow":              true,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
}

func (s *syncServer) validateTeamOffboardWithQuorum(in teamOffboardRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestTeamOffboardVote(s.getHTTPClient(), s.peerToken, peer, in)
		if allow {
			granted++
		}
		if s.preferredLeader != "" && voterID == s.preferredLeader {
			preferredOnline = true
		}
	}
	return granted, needed, preferredOnline, granted >= needed
}

func (s *syncServer) teamList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": s.teamManager.List()})
}

func (s *syncServer) governanceNodeRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		NodeID string `json:"node_id"`
		Role   string `json:"role"`
		Active bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := s.govManager.SetNodeRole(strings.TrimSpace(in.NodeID), strings.TrimSpace(in.Role), in.Active); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("governance.node_role", s.actorFromReq(r), "ok", map[string]any{"node_id": in.NodeID, "role": in.Role, "active": in.Active})
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (s *syncServer) governanceList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	nodes := s.govManager.List()
	voting := s.govManager.VotingCount()
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":        nodes,
		"voting_count": voting,
		"quorum":       quorumNeeded(voting),
	})
}

func (s *syncServer) governanceReconfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in governanceReconfigureRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.VotingNodes) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	if in.ProjectID == "" {
		in.ProjectID = s.projectID
	}
	if in.ProjectID != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	granted, needed, preferredOnline, ok := s.validateGovernanceReconfigureWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":             "rejected",
			"reason":             "quorum not reached",
			"granted":            granted,
			"needed":             needed,
			"mode":               modeName(s.preferredLeader),
			"preferred_leader":   s.preferredLeader,
			"preferred_online":   preferredOnline,
			"failover_activated": s.preferredLeader != "" && !preferredOnline,
		})
		return
	}
	if err := s.govManager.ReplaceVotingSet(in.VotingNodes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	nodes := s.govManager.List()
	voting := s.govManager.VotingCount()
	s.auditManager.Append("governance.reconfigure", s.actorFromReq(r), "ok", map[string]any{"voting_nodes": in.VotingNodes})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "reconfigured",
		"nodes":        nodes,
		"voting_count": voting,
		"quorum":       quorumNeeded(voting),
	})
}

func (s *syncServer) raftVoteGovernanceReconfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in governanceReconfigureRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if err := validateVotingSet(in.VotingNodes); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": err.Error(), "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
}

func (s *syncServer) raftValidateGovernanceReconfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in governanceReconfigureRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if err := validateVotingSet(in.VotingNodes); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"allow":   false,
			"reason":  err.Error(),
			"granted": 0,
			"needed":  quorumNeeded(len(s.peers) + 1),
		})
		return
	}

	granted, needed, preferredOnline, ok := s.validateGovernanceReconfigureWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"allow":   false,
			"reason":  "quorum not reached",
			"granted": granted,
			"needed":  needed,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"allow":              true,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
}

func (s *syncServer) validateGovernanceReconfigureWithQuorum(in governanceReconfigureRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestGovernanceReconfigureVote(s.getHTTPClient(), s.peerToken, peer, in)
		if allow {
			granted++
		}
		if s.preferredLeader != "" && voterID == s.preferredLeader {
			preferredOnline = true
		}
	}
	return granted, needed, preferredOnline, granted >= needed
}

func (s *syncServer) securityAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	items, err := s.auditManager.ReadTail(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items})
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
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
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
	preferredOnline := false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}

	for _, peer := range s.peers {
		ok, voterID, _ := requestTransitionVote(s.getHTTPClient(), s.peerToken, peer, in)
		if ok {
			granted++
		}
		if s.preferredLeader != "" && voterID == s.preferredLeader {
			preferredOnline = true
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
		"allow":              true,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
}

func (s *syncServer) metrics(w http.ResponseWriter, _ *http.Request) {
	last := atomic.LoadInt64(&s.lastSyncUnix)
	lastSyncAt := ""
	if last > 0 {
		lastSyncAt = time.Unix(last, 0).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":       s.nodeID,
		"project_id":    s.projectID,
		"pulled_events": atomic.LoadUint64(&s.pulledEvents),
		"pull_errors":   atomic.LoadUint64(&s.pullErrors),
		"last_sync_at":  lastSyncAt,
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
	atomic.StoreInt64(&s.lastSyncUnix, time.Now().Unix())

	remoteClock, err := s.fetchClock(ctx, peer, s.projectID)
	if err != nil {
		atomic.AddUint64(&s.pullErrors, 1)
		log.Printf("sync clock error peer=%s err=%v", peer, err)
		return
	}
	localClock, err := s.log.Clock(s.projectID)
	if err != nil {
		atomic.AddUint64(&s.pullErrors, 1)
		log.Printf("sync local clock error peer=%s err=%v", peer, err)
		return
	}
	for signerID, remoteSeq := range remoteClock {
		localSeq := localClock[signerID]
		if remoteSeq <= localSeq {
			continue
		}
		items, err := s.fetchEvents(ctx, peer, s.projectID, signerID, localSeq)
		if err != nil {
			atomic.AddUint64(&s.pullErrors, 1)
			log.Printf("sync fetch events error peer=%s signer=%s err=%v", peer, signerID, err)
			continue
		}
		for _, e := range items {
			if err := events.VerifyByEventKey(e); err != nil {
				atomic.AddUint64(&s.pullErrors, 1)
				log.Printf("sync verify event error peer=%s signer=%s seq=%d err=%v", peer, e.SignerID, e.Seq, err)
				continue
			}
			if err := s.log.Append(e); err == nil {
				atomic.AddUint64(&s.pulledEvents, 1)
			}
		}
	}
}

func (s *syncServer) fetchClock(ctx context.Context, peerBaseURL, projectID string) (map[string]uint64, error) {
	u := strings.TrimRight(peerBaseURL, "/") + "/sync/clock?project_id=" + projectID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	s.attachAuth(req, s.peerToken)
	res, err := s.getHTTPClient().Do(req)
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

func (s *syncServer) fetchEvents(ctx context.Context, peerBaseURL, projectID, signerID string, afterSeq uint64) ([]events.SignedEvent, error) {
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
	s.attachAuth(req, s.peerToken)
	res, err := s.getHTTPClient().Do(req)
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
		case "issue.assign", "issue.reassign":
			var p struct {
				Assignee string `json:"assignee"`
				New      string `json:"new_assignee"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if strings.TrimSpace(p.New) != "" {
				it.Assignee = strings.TrimSpace(p.New)
			} else {
				it.Assignee = strings.TrimSpace(p.Assignee)
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
	fmt.Println("  node storage migrate [--data-dir ./data]")
	fmt.Println("  node storage enable-encryption [--data-dir ./data]")
	fmt.Println("  node storage rotate-key [--data-dir ./data]")
	fmt.Println("  node storage verify-integrity [--data-dir ./data]")
	fmt.Println("  node trust invite --node-id node-x [--ttl-sec 3600]")
	fmt.Println("  node trust use-invite --node-id node-x --token <token>")
	fmt.Println("  node trust revoke --node-id node-x")
	fmt.Println("  node trust list")
	fmt.Println("  node team onboard --user-id u1 --role dev [--duty]")
	fmt.Println("  node team role-change --user-id u1 --role lead [--duty]")
	fmt.Println("  node team offboard --project-id OPS --user-id u1")
	fmt.Println("  node team list")
	fmt.Println("  node serve ... [--rate-limit-per-min 120]")
	fmt.Println("  node serve --project-id OPS --listen :4101 --node-role admin --preferred-leader node-1 --peers http://127.0.0.1:4102,http://127.0.0.1:4103")
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
	if token := strings.TrimSpace(envOr("USER_TOKEN", "")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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

func requestTransitionVote(client *http.Client, peerToken, peerBaseURL string, in transitionRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-transition", bytes.NewReader(raw))
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(peerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(peerToken))
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return false, "", fmt.Errorf("vote status=%d", res.StatusCode)
	}
	var out struct {
		Allow  bool   `json:"allow"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, "", err
	}
	return out.Allow, out.NodeID, nil
}

func requestGovernanceReconfigureVote(client *http.Client, peerToken, peerBaseURL string, in governanceReconfigureRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-governance-reconfigure", bytes.NewReader(raw))
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(peerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(peerToken))
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return false, "", fmt.Errorf("vote status=%d", res.StatusCode)
	}
	var out struct {
		Allow  bool   `json:"allow"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, "", err
	}
	return out.Allow, out.NodeID, nil
}

func requestTeamOffboardVote(client *http.Client, peerToken, peerBaseURL string, in teamOffboardRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-team-offboard", bytes.NewReader(raw))
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(peerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(peerToken))
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return false, "", fmt.Errorf("vote status=%d", res.StatusCode)
	}
	var out struct {
		Allow  bool   `json:"allow"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, "", err
	}
	return out.Allow, out.NodeID, nil
}

func validateVotingSet(nodes []string) error {
	if len(nodes) == 0 {
		return fmt.Errorf("voting_nodes must not be empty")
	}
	if len(nodes)%2 == 0 {
		return fmt.Errorf("voting_nodes count must be odd")
	}
	seen := make(map[string]struct{}, len(nodes))
	for _, raw := range nodes {
		id := strings.TrimSpace(raw)
		if id == "" {
			return fmt.Errorf("voting_nodes must not contain empty node id")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("voting_nodes must be unique")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func quorumNeeded(total int) int {
	return total/2 + 1
}

func modeName(preferredLeader string) string {
	if strings.TrimSpace(preferredLeader) == "" {
		return "normal"
	}
	return "admin_preferred"
}

func reassignOffboardedUser(dataDir, nodeID, projectID, offboardedUser string, tm *team.Manager) (int, error) {
	id, err := node.LoadOrCreate(dataDir, nodeID)
	if err != nil {
		return 0, err
	}
	logDB, err := store.Open(dataDir)
	if err != nil {
		return 0, err
	}
	all, err := logDB.ReadAll()
	if err != nil {
		return 0, err
	}
	return reassignFromEvents(logDB, id, tm, projectID, offboardedUser, all)
}

func reassignOffboardedUserByServer(s *syncServer, offboardedUser string) (int, error) {
	all, err := s.log.ReadAll()
	if err != nil {
		return 0, err
	}
	return reassignFromEvents(s.log, s.identity, s.teamManager, s.projectID, offboardedUser, all)
}

func reassignFromEvents(logDB *store.EventLog, id node.Identity, tm *team.Manager, projectID, offboardedUser string, all []events.SignedEvent) (int, error) {
	nextAssignee := tm.ActiveLead()
	reason := "lead_fallback"
	if nextAssignee == "" || nextAssignee == offboardedUser {
		nextAssignee = tm.ActiveDuty()
		reason = "duty_fallback"
	}
	if nextAssignee == offboardedUser {
		nextAssignee = ""
	}
	if nextAssignee == "" {
		reason = "unassigned_fallback"
	}
	board := projectBoardFromEvents(projectID, all)
	columns := make([]string, 0, len(board))
	for col := range board {
		if col == "done" {
			continue
		}
		columns = append(columns, col)
	}
	sort.Strings(columns)
	count := 0
	for _, col := range columns {
		issues := board[col]
		sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
		for _, it := range issues {
			if it.Assignee != offboardedUser {
				continue
			}
			payload := map[string]string{
				"old_assignee": offboardedUser,
				"new_assignee": nextAssignee,
				"reason":       reason,
				"status":       it.Status,
			}
			raw, _ := json.Marshal(payload)
			e := events.SignedEvent{
				Version:   1,
				ProjectID: projectID,
				EntityID:  it.ID,
				Type:      "issue.reassign",
				Payload:   raw,
				SignerID:  id.NodeID,
				SignerPub: id.Pub,
				Seq:       logDB.NextSeq(projectID, id.NodeID),
				Timestamp: time.Now().UTC(),
			}
			if len(id.Priv) > 0 {
				sig, err := events.Sign(id.Priv, e)
				if err != nil {
					return count, err
				}
				e.Signature = sig
			}
			if err := logDB.Append(e); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func (s *syncServer) withAuthAny(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter != nil {
			key := s.rateLimitKey(r)
			if !s.rateLimiter.Allow(key) {
				if s.auditManager != nil {
					s.auditManager.Append("rate_limit", key, "deny", nil)
				}
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
				return
			}
		}
		if !s.authEnabled {
			next(w, r)
			return
		}
		if _, ok := s.authenticate(r); !ok {
			if s.auditManager != nil {
				s.auditManager.Append("authn", s.actorFromReq(r), "deny", map[string]any{"path": r.URL.Path})
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *syncServer) withAuthRoles(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, v := range roles {
		allowed[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authEnabled {
			next(w, r)
			return
		}
		p, ok := s.authenticate(r)
		if !ok {
			if s.auditManager != nil {
				s.auditManager.Append("authn", s.actorFromReq(r), "deny", map[string]any{"path": r.URL.Path})
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(p.Role))]; !ok {
			if s.auditManager != nil {
				s.auditManager.Append("authz", p.UserID, "deny", map[string]any{"path": r.URL.Path, "role": p.Role})
			}
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		next(w, r)
	}
}

func (s *syncServer) authenticate(r *http.Request) (authPrincipal, bool) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return authPrincipal{}, false
	}
	token := strings.TrimSpace(auth[len("Bearer "):])
	p, ok := s.authTokens[token]
	if !ok || !p.Active {
		return authPrincipal{}, false
	}
	return p, true
}

func parseAuthTokens(raw string) (map[string]authPrincipal, error) {
	out := make(map[string]authPrincipal)
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("invalid auth tokens json: %w", err)
	}
	return out, nil
}

func buildServerTLSConfig(caFile string, mtlsRequired bool) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if !mtlsRequired {
		return cfg, nil
	}
	if strings.TrimSpace(caFile) == "" {
		return nil, fmt.Errorf("TLS_CA_FILE is required when mTLS is enabled")
	}
	pool, err := loadCertPool(caFile)
	if err != nil {
		return nil, err
	}
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	cfg.ClientCAs = pool
	return cfg, nil
}

func buildPeerHTTPClient(caFile, certFile, keyFile string) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(caFile) != "" {
		pool, err := loadCertPool(caFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}
	if strings.TrimSpace(certFile) != "" || strings.TrimSpace(keyFile) != "" {
		if strings.TrimSpace(certFile) == "" || strings.TrimSpace(keyFile) == "" {
			return nil, fmt.Errorf("both client cert and key are required")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

func loadCertPool(caFile string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("failed to parse CA file")
	}
	return pool, nil
}

func (s *syncServer) attachAuth(req *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func (s *syncServer) getHTTPClient() *http.Client {
	if s.httpClient != nil {
		return s.httpClient
	}
	return http.DefaultClient
}

func (s *syncServer) actorFromReq(r *http.Request) string {
	if p, ok := s.authenticate(r); ok {
		return p.UserID
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *syncServer) rateLimitKey(r *http.Request) string {
	if p, ok := s.authenticate(r); ok {
		return "token:" + p.UserID
	}
	return "ip:" + strings.TrimSpace(r.RemoteAddr)
}

type simpleRateLimiter struct {
	limit    int
	mu       sync.Mutex
	windowAt int64
	counts   map[string]int
}

func newSimpleRateLimiter(limit int) *simpleRateLimiter {
	if limit <= 0 {
		limit = 120
	}
	return &simpleRateLimiter{
		limit:  limit,
		counts: map[string]int{},
	}
}

func (r *simpleRateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	nowMin := time.Now().Unix() / 60
	if r.windowAt != nowMin {
		r.windowAt = nowMin
		r.counts = map[string]int{}
	}
	r.counts[key]++
	return r.counts[key] <= r.limit
}

func mustAtoi(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 120
	}
	return v
}
