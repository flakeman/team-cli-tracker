package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/csv"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
	case "auth":
		runAuth(os.Args[2:])
	case "audit":
		runAudit(os.Args[2:])
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
	nodeID := fs.String("node-id", envOr("NODE_ID", "node-1"), "node identifier (interactive commands)")
	format := fs.String("format", "plain", "plain|json")
	view := fs.String("view", "all", "all|mine")
	assigneeID := fs.String("assignee-id", envOr("BOARD_ASSIGNEE_ID", ""), "assignee id for mine view")
	countsOnly := fs.Bool("counts-only", false, "print only column counts (plain mode)")
	interactive := fs.Bool("interactive", false, "interactive board session with embedded commands (plain mode)")
	interactiveRefresh := fs.Duration("interactive-refresh", mustDuration(envOr("BOARD_INTERACTIVE_REFRESH", "0s"), 0), "interactive auto-refresh interval; 0 disables periodic redraw")
	once := fs.Bool("once", false, "print once and exit (plain mode defaults to live updates)")
	refresh := fs.Duration("refresh", mustDuration(envOr("BOARD_REFRESH", "2s"), 2*time.Second), "live board refresh interval")
	peersCSV := fs.String("peers", envOr("PEERS", ""), "comma-separated peer base URLs for board sync")
	peerToken := fs.String("peer-token", envOr("PEER_TOKEN", ""), "outgoing peer bearer token for board sync")
	tlsCA := fs.String("tls-ca", envOr("TLS_CA_FILE", ""), "CA file for peer TLS")
	clientCert := fs.String("client-cert", envOr("CLIENT_TLS_CERT_FILE", ""), "client cert for peer TLS")
	clientKey := fs.String("client-key", envOr("CLIENT_TLS_KEY_FILE", ""), "client key for peer TLS")
	_ = fs.Parse(args)
	if *projectID == "" {
		fatal(fmt.Errorf("project-id is required"))
	}
	if strings.ToLower(strings.TrimSpace(*format)) == "json" {
		*once = true
	}
	if *interactive && strings.ToLower(strings.TrimSpace(*format)) != "plain" {
		fatal(fmt.Errorf("interactive mode is supported only for plain format"))
	}

	log, err := store.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	peers := parsePeers(*peersCSV)
	httpClient, err := buildPeerHTTPClient(*tlsCA, *clientCert, *clientKey)
	if err != nil {
		fatal(err)
	}
	syncHelper := &syncServer{
		projectID:  *projectID,
		log:        log,
		peers:      peers,
		httpClient: httpClient,
		peerToken:  strings.TrimSpace(*peerToken),
	}
	syncOnce := func() {
		for _, peer := range peers {
			_ = syncHelper.pullFromPeer(peer)
		}
	}
	state := boardRenderState{
		viewMode:            strings.ToLower(strings.TrimSpace(*view)),
		assigneeID:          strings.TrimSpace(*assigneeID),
		countsOnly:          *countsOnly,
		interactivePaused:   false,
		interactiveInterval: *interactiveRefresh,
	}
	render := func(status string) error {
		all, err := log.ReadAll()
		if err != nil {
			return err
		}
		boardAll := projectBoardFromEvents(*projectID, all)
		board := boardAll
		mode := strings.ToLower(strings.TrimSpace(state.viewMode))
		switch mode {
		case "all", "":
		case "mine":
			target := strings.TrimSpace(state.assigneeID)
			if target == "" {
				target = strings.TrimSpace(envOr("USER_ID", envOr("USER", "")))
			}
			if target == "" {
				return fmt.Errorf("mine view requires --assignee-id or USER_ID")
			}
			board = filterBoardByAssignee(board, target)
		default:
			return fmt.Errorf("unsupported --view value: %s", *view)
		}
		switch *format {
		case "json":
			raw, _ := json.MarshalIndent(board, "", "  ")
			fmt.Println(string(raw))
		default:
			if state.countsOnly {
				printBoardCounts(*projectID, board, boardAll, len(all), mode, state.assigneeID)
			} else {
				printBoardPlain(*projectID, board, boardAll, len(all), mode, state.assigneeID)
			}
			if strings.TrimSpace(status) != "" {
				fmt.Printf("Status: %s\n", strings.TrimSpace(status))
			}
		}
		return nil
	}

	syncOnce()
	if *once {
		if err := render(""); err != nil {
			fatal(err)
		}
		return
	}
	if strings.ToLower(strings.TrimSpace(*format)) != "plain" {
		fatal(fmt.Errorf("live mode is supported only for plain format"))
	}
	if *refresh <= 0 {
		*refresh = 2 * time.Second
	}
	if *interactive {
		if state.interactiveInterval < 0 {
			state.interactiveInterval = 0
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	t := time.NewTicker(*refresh)
	defer t.Stop()
	if *interactive {
		runBoardInteractiveLoop(boardInteractiveInput{
			projectID: *projectID,
			nodeID:    *nodeID,
			dataDir:   *dataDir,
			log:       log,
			syncOnce:  syncOnce,
			refresh:   *refresh,
			render:    render,
			state:     &state,
			stop:      stop,
		})
		return
	}
	for {
		syncOnce()
		fmt.Print("\033[H\033[2J")
		if err := render(""); err != nil {
			fatal(err)
		}
		select {
		case <-t.C:
		case <-stop:
			return
		}
	}
}

type boardRenderState struct {
	viewMode            string
	assigneeID          string
	countsOnly          bool
	interactivePaused   bool
	interactiveInterval time.Duration
}

type boardInteractiveInput struct {
	projectID string
	nodeID    string
	dataDir   string
	log       *store.EventLog
	syncOnce  func()
	refresh   time.Duration
	render    func(status string) error
	state     *boardRenderState
	stop      chan os.Signal
}

type boardInteractiveCommand struct {
	name string
	args []string
}

func runBoardInteractiveLoop(in boardInteractiveInput) {
	cmdCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			cmdCh <- sc.Text()
		}
		close(cmdCh)
	}()
	status := "interactive mode: help | create | move | comment | view | counts | quit"
	var t *time.Ticker
	if in.state.interactiveInterval > 0 {
		t = time.NewTicker(in.state.interactiveInterval)
		defer t.Stop()
	}
	for {
		in.syncOnce()
		fmt.Print("\033[H\033[2J")
		if err := in.render(status); err != nil {
			fatal(err)
		}
		fmt.Println("Command> ")
		select {
		case <-in.stop:
			return
		case <-tickerChan(t, in.state):
			continue
		case raw, ok := <-cmdCh:
			if !ok {
				return
			}
			quit, msg := applyBoardInteractiveCommand(strings.TrimSpace(raw), in)
			status = msg
			if quit {
				return
			}
		}
	}
}

func applyBoardInteractiveCommand(raw string, in boardInteractiveInput) (bool, string) {
	cmd, err := parseBoardInteractiveCommand(raw)
	if err != nil {
		return false, err.Error()
	}
	switch cmd.name {
	case "help":
		return false, interactiveHelpText()
	case "quit":
		return true, "bye"
	case "counts":
		in.state.countsOnly = true
		return false, "counts-only view enabled"
	case "table":
		in.state.countsOnly = false
		return false, "table view enabled"
	case "pause":
		in.state.interactivePaused = true
		return false, "auto-refresh paused"
	case "resume":
		in.state.interactivePaused = false
		return false, "auto-refresh resumed"
	case "view":
		mode := strings.ToLower(strings.TrimSpace(cmd.args[0]))
		switch mode {
		case "all":
			in.state.viewMode = "all"
			in.state.assigneeID = ""
			return false, "view switched to all"
		case "mine":
			in.state.viewMode = "mine"
			if len(cmd.args) > 1 {
				in.state.assigneeID = strings.TrimSpace(cmd.args[1])
			}
			return false, "view switched to mine"
		default:
			return false, "view must be all or mine"
		}
	case "create":
		if err := appendIssueEvent(in.dataDir, in.nodeID, in.projectID, cmd.args[0], "issue.create", map[string]string{
			"status":     "todo",
			"summary":    cmd.args[1],
			"priority":   "medium",
			"assignee":   "",
			"created_by": in.nodeID,
		}); err != nil {
			return false, err.Error()
		}
		return false, "created " + cmd.args[0]
	case "move":
		if err := validateWorkflowTransition(cmd.args[1], cmd.args[2]); err != nil {
			return false, err.Error()
		}
		if err := appendIssueEvent(in.dataDir, in.nodeID, in.projectID, cmd.args[0], "issue.transition", map[string]string{
			"from": cmd.args[1],
			"to":   cmd.args[2],
		}); err != nil {
			return false, err.Error()
		}
		return false, "moved " + cmd.args[0] + " " + cmd.args[1] + "->" + cmd.args[2]
	case "comment":
		if err := appendIssueEvent(in.dataDir, in.nodeID, in.projectID, cmd.args[0], "issue.comment", map[string]string{
			"text": cmd.args[1],
		}); err != nil {
			return false, err.Error()
		}
		return false, "commented " + cmd.args[0]
	default:
		return false, "unsupported command"
	}
}

func interactiveHelpText() string {
	return strings.Join([]string{
		"Interactive commands:",
		"  help                          - show this help",
		"  create <ISSUE_ID> <summary>   - create issue in To Do",
		"    example: create OPS-901 Fix auth timeout",
		"  move <ISSUE_ID> <from> <to>   - move issue across workflow",
		"    example: move OPS-901 todo in_progress",
		"  comment <ISSUE_ID> <text>     - add comment to issue",
		"    example: comment OPS-901 check logs on node-2",
		"  view all                      - show all issues",
		"  view mine [assignee]          - show only assignee issues",
		"    example: view mine vova",
		"  counts                        - switch to counts-only summary",
		"  table                         - switch back to table view",
		"  pause                         - pause periodic redraw",
		"  resume                        - resume periodic redraw",
		"  quit                          - exit interactive mode",
	}, "\n")
}

func parseBoardInteractiveCommand(raw string) (boardInteractiveCommand, error) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return boardInteractiveCommand{}, fmt.Errorf("empty command")
	}
	parts := strings.Fields(line)
	name := strings.ToLower(parts[0])
	switch name {
	case "help", "quit", "counts", "table", "pause", "resume":
		return boardInteractiveCommand{name: name}, nil
	case "view":
		if len(parts) < 2 {
			return boardInteractiveCommand{}, fmt.Errorf("usage: view all|mine [assignee]")
		}
		args := []string{parts[1]}
		if len(parts) > 2 {
			args = append(args, parts[2])
		}
		return boardInteractiveCommand{name: "view", args: args}, nil
	case "create":
		x := strings.SplitN(line, " ", 3)
		if len(x) < 3 || strings.TrimSpace(x[1]) == "" || strings.TrimSpace(x[2]) == "" {
			return boardInteractiveCommand{}, fmt.Errorf("usage: create <ISSUE_ID> <summary>")
		}
		return boardInteractiveCommand{name: "create", args: []string{strings.TrimSpace(x[1]), strings.TrimSpace(x[2])}}, nil
	case "move":
		if len(parts) != 4 {
			return boardInteractiveCommand{}, fmt.Errorf("usage: move <ISSUE_ID> <from> <to>")
		}
		return boardInteractiveCommand{name: "move", args: []string{parts[1], parts[2], parts[3]}}, nil
	case "comment":
		x := strings.SplitN(line, " ", 3)
		if len(x) < 3 || strings.TrimSpace(x[1]) == "" || strings.TrimSpace(x[2]) == "" {
			return boardInteractiveCommand{}, fmt.Errorf("usage: comment <ISSUE_ID> <text>")
		}
		return boardInteractiveCommand{name: "comment", args: []string{strings.TrimSpace(x[1]), strings.TrimSpace(x[2])}}, nil
	default:
		return boardInteractiveCommand{}, fmt.Errorf("unknown command: %s", name)
	}
}

func runStorage(args []string) {
	if len(args) < 1 {
		fmt.Println("storage commands: migrate | enable-encryption | rotate-key | verify-integrity | key-policy-check | recovery-drill")
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
		if am, err := audit.Open(*dataDir); err == nil {
			am.Append("storage.key.enable", "local-cli", "ok", map[string]any{"active_key_id": keyID})
		}
		fmt.Printf("ok: encryption enabled active_key_id=%s\n", keyID)
	case "rotate-key":
		fs := flag.NewFlagSet("storage rotate-key", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		maxAge := fs.Duration("max-age", mustDuration(envOr("KEY_ROTATION_MAX_AGE", "720h"), 720*time.Hour), "rotation policy max key age")
		enforceDue := fs.Bool("enforce-due", envOr("ENFORCE_KEY_ROTATION_DUE", "false") == "true", "fail if key is not yet due by policy")
		_ = fs.Parse(args[1:])
		logDB, err := store.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if *enforceDue {
			if _, createdAt, enabled, err := logDB.ActiveEncryptionKeyInfo(); err == nil && enabled && *maxAge > 0 {
				if time.Since(createdAt) < *maxAge {
					fatal(fmt.Errorf("key rotation not due yet (age=%s, max=%s)", time.Since(createdAt).Truncate(time.Second), maxAge.String()))
				}
			}
		}
		keyID, err := logDB.RotateEncryptionKey()
		if err != nil {
			fatal(err)
		}
		if am, err := audit.Open(*dataDir); err == nil {
			am.Append("storage.key.rotate", "local-cli", "ok", map[string]any{
				"active_key_id": keyID,
				"enforce_due":   *enforceDue,
			})
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
	case "key-policy-check":
		fs := flag.NewFlagSet("storage key-policy-check", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		maxAge := fs.Duration("max-age", mustDuration(envOr("KEY_ROTATION_MAX_AGE", "720h"), 720*time.Hour), "rotation policy max key age")
		_ = fs.Parse(args[1:])
		logDB, err := store.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		keyID, createdAt, enabled, err := logDB.ActiveEncryptionKeyInfo()
		if err != nil {
			fatal(err)
		}
		if !enabled {
			fmt.Println("ok: encryption is not enabled")
			return
		}
		age := time.Since(createdAt)
		due := *maxAge > 0 && age >= *maxAge
		if am, err := audit.Open(*dataDir); err == nil {
			am.Append("storage.key.policy_check", "local-cli", "ok", map[string]any{
				"active_key_id": keyID,
				"key_age_sec":   int(age.Seconds()),
				"max_age_sec":   int(maxAge.Seconds()),
				"rotation_due":  due,
			})
		}
		fmt.Printf("active_key_id=%s\ncreated_at=%s\nkey_age=%s\nmax_age=%s\nrotation_due=%v\n", keyID, createdAt.Format(time.RFC3339), age.Truncate(time.Second), maxAge.String(), due)
	case "recovery-drill":
		fs := flag.NewFlagSet("storage recovery-drill", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		logDB, err := store.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		before, err := logDB.ReadAll()
		if err != nil {
			fatal(err)
		}
		if _, err := logDB.EnableEncryption(); err != nil {
			fatal(err)
		}
		keyID, err := logDB.RotateEncryptionKey()
		if err != nil {
			fatal(err)
		}
		after, err := logDB.ReadAll()
		if err != nil {
			fatal(err)
		}
		if len(before) != len(after) {
			fatal(fmt.Errorf("recovery drill failed: event count mismatch before=%d after=%d", len(before), len(after)))
		}
		if am, err := audit.Open(*dataDir); err == nil {
			am.Append("storage.key.recovery_drill", "local-cli", "ok", map[string]any{
				"active_key_id":  keyID,
				"events_checked": len(after),
			})
		}
		fmt.Printf("ok: recovery drill passed active_key_id=%s events_checked=%d\n", keyID, len(after))
	default:
		fmt.Println("storage commands: migrate | enable-encryption | rotate-key | verify-integrity | key-policy-check | recovery-drill")
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

func runAuth(args []string) {
	if len(args) < 1 {
		fmt.Println("auth commands: issue | revoke | list | bind-role | list-bindings")
		return
	}
	switch args[0] {
	case "issue":
		fs := flag.NewFlagSet("auth issue", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		userID := fs.String("user-id", "", "user id")
		role := fs.String("role", "", "role")
		ttlSec := fs.Int("ttl-sec", 3600, "token ttl in seconds")
		_ = fs.Parse(args[1:])
		m, err := authstore.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		rec, err := m.Issue(*userID, *role, time.Duration(*ttlSec)*time.Second)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("token=%s\nuser_id=%s\nrole=%s\nexpires_at=%s\n", rec.Token, rec.UserID, rec.Role, rec.ExpiresAt)
	case "revoke":
		fs := flag.NewFlagSet("auth revoke", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		token := fs.String("token", "", "token")
		_ = fs.Parse(args[1:])
		m, err := authstore.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := m.Revoke(*token); err != nil {
			fatal(err)
		}
		fmt.Println("ok: token revoked")
	case "list":
		fs := flag.NewFlagSet("auth list", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		m, err := authstore.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		raw, _ := json.MarshalIndent(map[string]any{"tokens": m.ListTokens()}, "", "  ")
		fmt.Println(string(raw))
	case "bind-role":
		fs := flag.NewFlagSet("auth bind-role", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		userID := fs.String("user-id", "", "user id")
		role := fs.String("role", "", "role")
		_ = fs.Parse(args[1:])
		m, err := authstore.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		if err := m.SetRoleBinding(*userID, *role); err != nil {
			fatal(err)
		}
		fmt.Println("ok: role binding updated")
	case "list-bindings":
		fs := flag.NewFlagSet("auth list-bindings", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		m, err := authstore.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		raw, _ := json.MarshalIndent(map[string]any{"role_bindings": m.ListRoleBindings()}, "", "  ")
		fmt.Println(string(raw))
	default:
		fmt.Println("auth commands: issue | revoke | list | bind-role | list-bindings")
	}
}

func runAudit(args []string) {
	if len(args) < 1 {
		fmt.Println("audit commands: export | verify-integrity")
		return
	}
	switch args[0] {
	case "export":
		fs := flag.NewFlagSet("audit export", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		all := fs.Bool("all", false, "export all audit records")
		from := fs.String("from", "", "RFC3339 start time (inclusive)")
		to := fs.String("to", "", "RFC3339 end time (inclusive)")
		user := fs.String("user", "", "comma-separated user ids")
		format := fs.String("format", "jsonl", "jsonl|csv")
		limit := fs.Int("limit", 0, "page size (0 means all)")
		cursor := fs.String("cursor", "", "page cursor (offset)")
		_ = fs.Parse(args[1:])

		fromTS, toTS, err := parseAuditTimeRange(*from, *to)
		if err != nil {
			fatal(err)
		}
		users := parseUserFilter(*user)
		if !*all && fromTS == nil && toTS == nil && len(users) == 0 {
			*all = true
		}
		if !*all && fromTS == nil && toTS == nil && len(users) == 0 {
			fatal(fmt.Errorf("set --all or at least one filter: --from/--to/--user"))
		}
		am, err := audit.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		records, err := am.ReadAll()
		if err != nil {
			fatal(err)
		}
		records = filterAuditEvents(records, fromTS, toTS, users)
		paged, nextCursor, err := paginateAuditEvents(records, *limit, *cursor)
		if err != nil {
			fatal(err)
		}
		switch strings.ToLower(strings.TrimSpace(*format)) {
		case "jsonl":
			for _, rec := range paged {
				raw, err := json.Marshal(rec)
				if err != nil {
					continue
				}
				fmt.Println(string(raw))
			}
		case "csv":
			w := csv.NewWriter(os.Stdout)
			_ = w.Write([]string{"time", "type", "actor", "status", "details_json"})
			for _, rec := range paged {
				detailsRaw := "{}"
				if rec.Details != nil {
					if raw, err := json.Marshal(rec.Details); err == nil {
						detailsRaw = string(raw)
					}
				}
				_ = w.Write([]string{rec.Time, rec.Type, rec.Actor, rec.Status, detailsRaw})
			}
			w.Flush()
			if err := w.Error(); err != nil {
				fatal(err)
			}
		default:
			fatal(fmt.Errorf("unsupported format: %s", *format))
		}
		if nextCursor != "" {
			fmt.Fprintf(os.Stderr, "next_cursor=%s\n", nextCursor)
		}
		am.Append("audit.export", "local-cli", "ok", map[string]any{
			"all":         *all,
			"from":        strings.TrimSpace(*from),
			"to":          strings.TrimSpace(*to),
			"user":        strings.TrimSpace(*user),
			"format":      strings.ToLower(strings.TrimSpace(*format)),
			"count":       len(paged),
			"limit":       *limit,
			"cursor":      strings.TrimSpace(*cursor),
			"next_cursor": nextCursor,
		})
	case "verify-integrity":
		fs := flag.NewFlagSet("audit verify-integrity", flag.ExitOnError)
		dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory")
		_ = fs.Parse(args[1:])
		am, err := audit.Open(*dataDir)
		if err != nil {
			fatal(err)
		}
		checked, err := am.VerifyIntegrity()
		if err != nil {
			am.Append("audit.verify_integrity", "local-cli", "error", map[string]any{"error": err.Error()})
			fatal(err)
		}
		am.Append("audit.verify_integrity", "local-cli", "ok", map[string]any{"checked": checked})
		fmt.Printf("ok: audit integrity verified checked=%d\n", checked)
	default:
		fmt.Println("audit commands: export | verify-integrity")
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
	publicURL := fs.String("public-url", envOr("PUBLIC_URL", ""), "public base URL advertised to peers, e.g. http://node-1:4101")
	tlsCert := fs.String("tls-cert", envOr("TLS_CERT_FILE", ""), "server TLS cert file")
	tlsKey := fs.String("tls-key", envOr("TLS_KEY_FILE", ""), "server TLS key file")
	tlsCA := fs.String("tls-ca", envOr("TLS_CA_FILE", ""), "CA file for mTLS and client trust")
	tlsCRL := fs.String("tls-crl", envOr("TLS_CRL_FILE", ""), "CRL file for revoked client certificates (optional)")
	mtlsRequired := fs.Bool("mtls-required", envOr("MTLS_REQUIRED", "false") == "true", "require and verify client cert")
	clientCert := fs.String("client-cert", envOr("CLIENT_TLS_CERT_FILE", ""), "client cert for outgoing peer requests")
	clientKey := fs.String("client-key", envOr("CLIENT_TLS_KEY_FILE", ""), "client key for outgoing peer requests")
	secureModeRequired := fs.Bool("secure-mode-required", envOr("SECURE_MODE_REQUIRED", "true") == "true", "hard-fail startup when secure transport/auth requirements are unmet")
	rateLimitPerMin := fs.Int("rate-limit-per-min", mustAtoi(envOr("RATE_LIMIT_PER_MIN", "120")), "per-token or per-ip requests per minute")
	rateLimitSensitivePerMin := fs.Int("rate-limit-sensitive-per-min", mustAtoi(envOr("RATE_LIMIT_SENSITIVE_PER_MIN", "30")), "per-token or per-ip requests per minute for sensitive endpoints")
	tick := fs.Duration("sync-tick", 3*time.Second, "sync interval")
	discoveryEnabled := fs.Bool("discovery-enabled", envOr("DISCOVERY_ENABLED", "true") == "true", "enable peer discovery from /sync/peers")
	discoveryTTL := fs.Duration("discovery-ttl", mustDuration(envOr("DISCOVERY_TTL", "45s"), 45*time.Second), "ttl for discovered peers before prune")
	enforceKeyRotationPolicy := fs.Bool("enforce-key-rotation-policy", envOr("ENFORCE_KEY_ROTATION_POLICY", "true") == "true", "hard-fail startup if active encryption key exceeds max age")
	keyRotationMaxAge := fs.Duration("key-rotation-max-age", mustDuration(envOr("KEY_ROTATION_MAX_AGE", "720h"), 720*time.Hour), "maximum allowed active encryption key age before startup fails")
	_ = fs.Parse(args)

	id, err := node.LoadOrCreate(*dataDir, *nodeID)
	if err != nil {
		fatal(err)
	}
	log, err := store.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	if *enforceKeyRotationPolicy {
		keyID, createdAt, enabled, err := log.ActiveEncryptionKeyInfo()
		if err != nil {
			fatal(fmt.Errorf("encryption key policy check failed: %w", err))
		}
		if enabled && *keyRotationMaxAge > 0 {
			age := time.Since(createdAt)
			if age > *keyRotationMaxAge {
				fatal(fmt.Errorf("active encryption key %s age=%s exceeds max=%s; rotate key before starting", keyID, age.Truncate(time.Second), keyRotationMaxAge.String()))
			}
		}
	}
	peers := parsePeers(*peersCSV)
	tokenMap, err := parseAuthTokens(*authTokensJSON)
	if err != nil {
		fatal(err)
	}
	if err := validateServeSecurityPolicy(serveSecurityPolicyInput{
		SecureModeRequired: *secureModeRequired,
		AuthEnabled:        *authEnabled,
		HasPeers:           len(peers) > 0,
		TLSCert:            *tlsCert,
		TLSKey:             *tlsKey,
		TLSCA:              *tlsCA,
		MTLSRequired:       *mtlsRequired,
		ClientCert:         *clientCert,
		ClientKey:          *clientKey,
		PeerToken:          *peerToken,
	}); err != nil {
		fatal(err)
	}
	serverTLS, err := buildServerTLSConfig(*tlsCA, *tlsCRL, *mtlsRequired)
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
	authManager, err := authstore.Open(*dataDir)
	if err != nil {
		fatal(err)
	}
	staticSeed := make(map[string]authstore.Principal, len(tokenMap))
	for tok, p := range tokenMap {
		staticSeed[tok] = authstore.Principal{
			UserID: p.UserID,
			Role:   p.Role,
			Active: p.Active,
		}
	}
	if err := authManager.SeedStatic(staticSeed); err != nil {
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
		projectID:            *projectID,
		nodeID:               id.NodeID,
		log:                  log,
		peers:                peers,
		nodeRole:             strings.ToLower(strings.TrimSpace(*nodeRole)),
		preferredLeader:      strings.TrimSpace(*preferredLeader),
		authEnabled:          *authEnabled,
		authTokens:           tokenMap,
		authManager:          authManager,
		httpClient:           httpClient,
		peerToken:            strings.TrimSpace(*peerToken),
		publicURL:            normalizePeerURL(*publicURL),
		discoveryEnabled:     *discoveryEnabled,
		discoveryTTL:         *discoveryTTL,
		trustManager:         trustManager,
		teamManager:          teamManager,
		identity:             id,
		govManager:           govManager,
		auditManager:         auditManager,
		rateLimiter:          newSimpleRateLimiter(*rateLimitPerMin),
		sensitiveRateLimiter: newSimpleRateLimiter(*rateLimitSensitivePerMin),
		peerSync:             map[string]peerSyncState{},
		discoveredPeers:      map[string]int64{},
		peerNodeByURL:        map[string]string{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/sync/clock", s.withAuthAny(s.syncClock))
	mux.HandleFunc("/sync/events", s.withAuthAny(s.syncEvents))
	mux.HandleFunc("/sync/ingest", s.withAuthAny(s.syncIngest))
	mux.HandleFunc("/sync/peers", s.withAuthAny(s.syncPeers))
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
	mux.HandleFunc("/security/audit/export", s.withAuthRoles(s.securityAuditExport, "admin", "lead"))
	mux.HandleFunc("/auth/issue", s.withAuthRoles(s.authIssue, "admin"))
	mux.HandleFunc("/auth/revoke", s.withAuthRoles(s.authRevoke, "admin"))
	mux.HandleFunc("/auth/list", s.withAuthRoles(s.authList, "admin", "lead"))
	mux.HandleFunc("/auth/bind-role", s.withAuthRoles(s.authBindRole, "admin"))
	mux.HandleFunc("/auth/list-bindings", s.withAuthRoles(s.authListBindings, "admin", "lead"))

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
	projectID                string
	nodeID                   string
	log                      *store.EventLog
	peers                    []string
	nodeRole                 string
	preferredLeader          string
	pulledEvents             uint64
	pullErrors               uint64
	lastSyncUnix             int64
	authEnabled              bool
	authTokens               map[string]authPrincipal
	authManager              *authstore.Manager
	httpClient               *http.Client
	peerToken                string
	publicURL                string
	discoveryEnabled         bool
	discoveryTTL             time.Duration
	trustManager             *trust.Manager
	teamManager              *team.Manager
	identity                 node.Identity
	govManager               *governance.Manager
	auditManager             *audit.Manager
	rateLimiter              *simpleRateLimiter
	sensitiveRateLimiter     *simpleRateLimiter
	rateLimitDenied          atomic.Uint64
	rateLimitDeniedDefault   atomic.Uint64
	rateLimitDeniedSensitive atomic.Uint64
	authnDenied              atomic.Uint64
	authzDenied              atomic.Uint64
	syncPeerPulls            atomic.Uint64
	syncPeerPullSuccess      atomic.Uint64
	syncPeerSkippedBackoff   atomic.Uint64
	peerMu                   sync.Mutex
	peerSync                 map[string]peerSyncState
	discoveredPeers          map[string]int64
	peerNodeByURL            map[string]string
}

type peerSyncState struct {
	ConsecutiveFailures int
	BackoffUntilUnix    int64
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

type teamOnboardRequest struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	Duty      bool   `json:"duty"`
}

type teamRoleChangeRequest struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	Duty      bool   `json:"duty"`
}

type governanceNodeRoleRequest struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
}

type trustInviteRequest struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	TTLSec    int    `json:"ttl_sec"`
}

type trustRevokeRequest struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
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

func (s *syncServer) syncPeers(w http.ResponseWriter, _ *http.Request) {
	peers := s.activeSyncPeers()
	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":    s.nodeID,
		"public_url": s.publicURL,
		"peers":      peers,
	})
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
	var in trustInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" {
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
	granted, needed, preferredOnline, ok := s.validateTrustInviteWithQuorum(in)
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
	if in.TTLSec <= 0 {
		in.TTLSec = 3600
	}
	token, exp, err := s.trustManager.CreateInvite(strings.TrimSpace(in.NodeID), time.Duration(in.TTLSec)*time.Second)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("trust.invite", s.actorFromReq(r), "ok", map[string]any{
		"node_id":            in.NodeID,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
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
	var in trustRevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" {
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
	granted, needed, preferredOnline, ok := s.validateTrustRevokeWithQuorum(in)
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
	if err := s.trustManager.RevokeNode(strings.TrimSpace(in.NodeID)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("trust.revoke", s.actorFromReq(r), "ok", map[string]any{
		"node_id":            in.NodeID,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "node_id": strings.TrimSpace(in.NodeID)})
}

func (s *syncServer) raftVoteTrustInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in trustInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.NodeID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "node_id is required", "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
}

func (s *syncServer) raftValidateTrustInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in trustInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.NodeID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "node_id is required", "granted": 0, "needed": quorumNeeded(len(s.peers) + 1)})
		return
	}
	granted, needed, preferredOnline, ok := s.validateTrustInviteWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "quorum not reached", "granted": granted, "needed": needed})
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

func (s *syncServer) raftVoteTrustRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in trustRevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.NodeID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "node_id is required", "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
}

func (s *syncServer) raftValidateTrustRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in trustRevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.NodeID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "node_id is required", "granted": 0, "needed": quorumNeeded(len(s.peers) + 1)})
		return
	}
	granted, needed, preferredOnline, ok := s.validateTrustRevokeWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "quorum not reached", "granted": granted, "needed": needed})
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
	var in teamOnboardRequest
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
	if in.Role == "" {
		in.Role = "viewer"
	}
	granted, needed, preferredOnline, ok := s.validateTeamOnboardWithQuorum(in)
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
	if err := s.teamManager.Onboard(team.Member{UserID: in.UserID, Role: in.Role, Duty: in.Duty, Active: true}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("team.onboard", s.actorFromReq(r), "ok", map[string]any{
		"user_id":            in.UserID,
		"role":               in.Role,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"status": "onboarded", "user_id": in.UserID, "role": in.Role, "duty": in.Duty})
}

func (s *syncServer) teamRoleChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamRoleChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.Role) == "" {
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
	granted, needed, preferredOnline, ok := s.validateTeamRoleChangeWithQuorum(in)
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
	if err := s.teamManager.ChangeRole(in.UserID, in.Role, in.Duty); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("team.role_change", s.actorFromReq(r), "ok", map[string]any{
		"user_id":            in.UserID,
		"role":               in.Role,
		"duty":               in.Duty,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
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
	s.auditManager.Append("team.offboard", s.actorFromReq(r), "ok", map[string]any{
		"user_id":            in.UserID,
		"reassigned":         cnt,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
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

func (s *syncServer) raftVoteTeamOnboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamOnboardRequest
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

func (s *syncServer) raftValidateTeamOnboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamOnboardRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.UserID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "user_id is required", "granted": 0, "needed": quorumNeeded(len(s.peers) + 1)})
		return
	}
	granted, needed, preferredOnline, ok := s.validateTeamOnboardWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "quorum not reached", "granted": granted, "needed": needed})
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

func (s *syncServer) raftVoteTeamRoleChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamRoleChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "user_id and role are required", "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
}

func (s *syncServer) raftValidateTeamRoleChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in teamRoleChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "user_id and role are required", "granted": 0, "needed": quorumNeeded(len(s.peers) + 1)})
		return
	}
	granted, needed, preferredOnline, ok := s.validateTeamRoleChangeWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "quorum not reached", "granted": granted, "needed": needed})
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

func (s *syncServer) validateTeamOnboardWithQuorum(in teamOnboardRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestTeamOnboardVote(s.getHTTPClient(), s.peerToken, peer, in)
		if allow {
			granted++
		}
		if s.preferredLeader != "" && voterID == s.preferredLeader {
			preferredOnline = true
		}
	}
	return granted, needed, preferredOnline, granted >= needed
}

func (s *syncServer) validateTeamRoleChangeWithQuorum(in teamRoleChangeRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestTeamRoleChangeVote(s.getHTTPClient(), s.peerToken, peer, in)
		if allow {
			granted++
		}
		if s.preferredLeader != "" && voterID == s.preferredLeader {
			preferredOnline = true
		}
	}
	return granted, needed, preferredOnline, granted >= needed
}

func (s *syncServer) validateTrustInviteWithQuorum(in trustInviteRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestTrustInviteVote(s.getHTTPClient(), s.peerToken, peer, in)
		if allow {
			granted++
		}
		if s.preferredLeader != "" && voterID == s.preferredLeader {
			preferredOnline = true
		}
	}
	return granted, needed, preferredOnline, granted >= needed
}

func (s *syncServer) validateTrustRevokeWithQuorum(in trustRevokeRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestTrustRevokeVote(s.getHTTPClient(), s.peerToken, peer, in)
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
	var in governanceNodeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Role) == "" {
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
	granted, needed, preferredOnline, ok := s.validateGovernanceNodeRoleWithQuorum(in)
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
	if err := s.govManager.SetNodeRole(strings.TrimSpace(in.NodeID), strings.TrimSpace(in.Role), in.Active); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("governance.node_role", s.actorFromReq(r), "ok", map[string]any{
		"node_id":            in.NodeID,
		"role":               in.Role,
		"active":             in.Active,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
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

func (s *syncServer) raftVoteGovernanceNodeRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in governanceNodeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "node_id and role are required", "node_id": s.nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allow": true, "node_id": s.nodeID, "node_role": s.nodeRole})
}

func (s *syncServer) raftValidateGovernanceNodeRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in governanceNodeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(in.ProjectID) != s.projectID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project mismatch"})
		return
	}
	if strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "node_id and role are required", "granted": 0, "needed": quorumNeeded(len(s.peers) + 1)})
		return
	}
	granted, needed, preferredOnline, ok := s.validateGovernanceNodeRoleWithQuorum(in)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"allow": false, "reason": "quorum not reached", "granted": granted, "needed": needed})
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
	s.auditManager.Append("governance.reconfigure", s.actorFromReq(r), "ok", map[string]any{
		"voting_nodes":       in.VotingNodes,
		"granted":            granted,
		"needed":             needed,
		"mode":               modeName(s.preferredLeader),
		"preferred_leader":   s.preferredLeader,
		"preferred_online":   preferredOnline,
		"failover_activated": s.preferredLeader != "" && !preferredOnline,
	})
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

func (s *syncServer) validateGovernanceNodeRoleWithQuorum(in governanceNodeRoleRequest) (granted, needed int, preferredOnline bool, ok bool) {
	granted = 1
	totalNodes := len(s.peers) + 1
	needed = quorumNeeded(totalNodes)
	preferredOnline = false
	if s.preferredLeader != "" && s.nodeID == s.preferredLeader {
		preferredOnline = true
	}
	for _, peer := range s.peers {
		allow, voterID, _ := requestGovernanceNodeRoleVote(s.getHTTPClient(), s.peerToken, peer, in)
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

func (s *syncServer) securityAuditExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	fromTS, toTS, err := parseAuditTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	users := parseUserFilter(r.URL.Query().Get("user"))
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 || v > 10000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = v
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	items, err := s.auditManager.ReadAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	filtered := filterAuditEvents(items, fromTS, toTS, users)
	paged, next, err := paginateAuditEvents(filtered, limit, cursor)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auditManager.Append("audit.export_api", s.actorFromReq(r), "ok", map[string]any{
		"from":        strings.TrimSpace(r.URL.Query().Get("from")),
		"to":          strings.TrimSpace(r.URL.Query().Get("to")),
		"user":        strings.TrimSpace(r.URL.Query().Get("user")),
		"limit":       limit,
		"cursor":      cursor,
		"next_cursor": next,
		"count":       len(paged),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"events":      paged,
		"next_cursor": next,
		"count":       len(paged),
	})
}

func (s *syncServer) authIssue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.authManager == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth manager is not configured"})
		return
	}
	var in struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		TTLSec int    `json:"ttl_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if in.TTLSec == 0 {
		in.TTLSec = 3600
	}
	rec, err := s.authManager.Issue(strings.TrimSpace(in.UserID), strings.TrimSpace(in.Role), time.Duration(in.TTLSec)*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *syncServer) authRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.authManager == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth manager is not configured"})
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := s.authManager.Revoke(strings.TrimSpace(in.Token)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *syncServer) authList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.authManager == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth manager is not configured"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": s.authManager.ListTokens()})
}

func (s *syncServer) authBindRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.authManager == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth manager is not configured"})
		return
	}
	var in struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.Role) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := s.authManager.SetRoleBinding(strings.TrimSpace(in.UserID), strings.TrimSpace(in.Role)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *syncServer) authListBindings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.authManager == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth manager is not configured"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"role_bindings": s.authManager.ListRoleBindings()})
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
	now := time.Now().UTC()
	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":                     s.nodeID,
		"project_id":                  s.projectID,
		"pulled_events":               atomic.LoadUint64(&s.pulledEvents),
		"pull_errors":                 atomic.LoadUint64(&s.pullErrors),
		"sync_peer_pulls":             s.syncPeerPulls.Load(),
		"sync_peer_pull_success":      s.syncPeerPullSuccess.Load(),
		"sync_peer_skipped_backoff":   s.syncPeerSkippedBackoff.Load(),
		"sync_peer_backoff_active":    s.peerBackoffActive(now),
		"sync_discovered_peers":       s.discoveredPeerCount(),
		"rate_limit_denied_total":     s.rateLimitDenied.Load(),
		"rate_limit_denied_default":   s.rateLimitDeniedDefault.Load(),
		"rate_limit_denied_sensitive": s.rateLimitDeniedSensitive.Load(),
		"authn_denied_total":          s.authnDenied.Load(),
		"authz_denied_total":          s.authzDenied.Load(),
		"last_sync_at":                lastSyncAt,
	})
}

func (s *syncServer) syncLoop(interval time.Duration) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	if s.discoveryTTL <= 0 {
		s.discoveryTTL = 45 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		tickNow := time.Now().UTC()
		for _, peer := range s.activeSyncPeers() {
			now := time.Now().UTC()
			if !s.shouldPullPeer(peer, now) {
				s.syncPeerSkippedBackoff.Add(1)
				continue
			}
			ok := s.pullFromPeer(peer)
			s.recordPeerPullResult(peer, ok, time.Now().UTC())
		}
		if s.discoveryEnabled {
			s.refreshDiscoveredPeers(tickNow)
			s.pruneDiscoveredPeers(tickNow)
		}
	}
}

func (s *syncServer) pullFromPeer(peer string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	atomic.StoreInt64(&s.lastSyncUnix, time.Now().Unix())
	s.syncPeerPulls.Add(1)
	hadErr := false

	remoteClock, err := s.fetchClock(ctx, peer, s.projectID)
	if err != nil {
		atomic.AddUint64(&s.pullErrors, 1)
		hadErr = true
		log.Printf("sync clock error peer=%s err=%v", peer, err)
		return false
	}
	localClock, err := s.log.Clock(s.projectID)
	if err != nil {
		atomic.AddUint64(&s.pullErrors, 1)
		hadErr = true
		log.Printf("sync local clock error peer=%s err=%v", peer, err)
		return false
	}
	for signerID, remoteSeq := range remoteClock {
		localSeq := localClock[signerID]
		if remoteSeq <= localSeq {
			continue
		}
		items, err := s.fetchEvents(ctx, peer, s.projectID, signerID, localSeq)
		if err != nil {
			atomic.AddUint64(&s.pullErrors, 1)
			hadErr = true
			log.Printf("sync fetch events error peer=%s signer=%s err=%v", peer, signerID, err)
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Seq < items[j].Seq })
		for _, e := range items {
			if err := events.VerifyByEventKey(e); err != nil {
				atomic.AddUint64(&s.pullErrors, 1)
				hadErr = true
				log.Printf("sync verify event error peer=%s signer=%s seq=%d err=%v", peer, e.SignerID, e.Seq, err)
				continue
			}
			if err := s.log.Append(e); err == nil {
				atomic.AddUint64(&s.pulledEvents, 1)
			}
		}
	}
	if !hadErr {
		s.syncPeerPullSuccess.Add(1)
	}
	return !hadErr
}

func (s *syncServer) shouldPullPeer(peer string, now time.Time) bool {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if s.peerSync == nil {
		return true
	}
	st, ok := s.peerSync[peer]
	if !ok {
		return true
	}
	return st.BackoffUntilUnix <= now.Unix()
}

func (s *syncServer) recordPeerPullResult(peer string, ok bool, now time.Time) {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if s.peerSync == nil {
		s.peerSync = make(map[string]peerSyncState)
	}
	st := s.peerSync[peer]
	if ok {
		st.ConsecutiveFailures = 0
		st.BackoffUntilUnix = 0
		s.peerSync[peer] = st
		return
	}
	st.ConsecutiveFailures++
	delaySec := 1 << minInt(st.ConsecutiveFailures-1, 6) // 1s,2s,4s,... max 64s
	st.BackoffUntilUnix = now.Add(time.Duration(delaySec) * time.Second).Unix()
	s.peerSync[peer] = st
}

func (s *syncServer) peerBackoffActive(now time.Time) int {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if len(s.peerSync) == 0 {
		return 0
	}
	out := 0
	for _, st := range s.peerSync {
		if st.BackoffUntilUnix > now.Unix() {
			out++
		}
	}
	return out
}

func (s *syncServer) activeSyncPeers() []string {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	out := make([]string, 0, len(s.peers)+len(s.discoveredPeers))
	seen := map[string]struct{}{}
	for _, p := range s.peers {
		n := normalizePeerURL(p)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	nowUnix := time.Now().UTC().Unix()
	for p, seenAt := range s.discoveredPeers {
		if seenAt <= 0 || nowUnix-seenAt > int64(s.discoveryTTL.Seconds()) {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (s *syncServer) discoveredPeerCount() int {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	return len(s.discoveredPeers)
}

func (s *syncServer) refreshDiscoveredPeers(now time.Time) {
	sources := s.activeSyncPeers()
	for _, source := range sources {
		peerNodeID, peers, err := s.fetchPeerSnapshot(source)
		if err != nil {
			continue
		}
		if peerNodeID != "" {
			s.recordPeerIdentity(source, peerNodeID)
			if s.trustManager != nil && !s.trustManager.IsTrusted(peerNodeID) {
				s.removeDiscoveredPeer(source)
				continue
			}
		}
		for _, candidate := range peers {
			candidate = normalizePeerURL(candidate)
			if candidate == "" || candidate == source {
				continue
			}
			nodeID, err := s.fetchHealthNodeID(candidate)
			if err != nil || strings.TrimSpace(nodeID) == "" {
				continue
			}
			if s.trustManager != nil && !s.trustManager.IsTrusted(nodeID) {
				s.removeDiscoveredPeer(candidate)
				continue
			}
			s.addDiscoveredPeer(candidate, nodeID, now)
		}
	}
}

func (s *syncServer) pruneDiscoveredPeers(now time.Time) {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if s.discoveredPeers == nil {
		return
	}
	ttlSec := int64(s.discoveryTTL.Seconds())
	if ttlSec <= 0 {
		ttlSec = 45
	}
	nowUnix := now.Unix()
	for peer, seenAt := range s.discoveredPeers {
		if nowUnix-seenAt > ttlSec {
			delete(s.discoveredPeers, peer)
			delete(s.peerNodeByURL, peer)
			delete(s.peerSync, peer)
			continue
		}
		if s.trustManager != nil {
			if nodeID := strings.TrimSpace(s.peerNodeByURL[peer]); nodeID != "" && !s.trustManager.IsTrusted(nodeID) {
				delete(s.discoveredPeers, peer)
				delete(s.peerNodeByURL, peer)
				delete(s.peerSync, peer)
			}
		}
	}
}

func (s *syncServer) fetchPeerSnapshot(peerBaseURL string) (string, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(peerBaseURL, "/")+"/sync/peers", nil)
	if err != nil {
		return "", nil, err
	}
	s.attachAuth(req, s.peerToken)
	res, err := s.getHTTPClient().Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return "", nil, fmt.Errorf("sync peers status=%d", res.StatusCode)
	}
	var out struct {
		NodeID    string   `json:"node_id"`
		PublicURL string   `json:"public_url"`
		Peers     []string `json:"peers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", nil, err
	}
	if u := normalizePeerURL(out.PublicURL); u != "" {
		out.Peers = append(out.Peers, u)
	}
	return strings.TrimSpace(out.NodeID), out.Peers, nil
}

func (s *syncServer) fetchHealthNodeID(peerBaseURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(peerBaseURL, "/")+"/healthz", nil)
	if err != nil {
		return "", err
	}
	s.attachAuth(req, s.peerToken)
	res, err := s.getHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("health status=%d", res.StatusCode)
	}
	var out struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.NodeID), nil
}

func (s *syncServer) addDiscoveredPeer(peerURL, nodeID string, now time.Time) {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if s.discoveredPeers == nil {
		s.discoveredPeers = map[string]int64{}
	}
	if s.peerNodeByURL == nil {
		s.peerNodeByURL = map[string]string{}
	}
	s.discoveredPeers[peerURL] = now.Unix()
	s.peerNodeByURL[peerURL] = strings.TrimSpace(nodeID)
}

func (s *syncServer) removeDiscoveredPeer(peerURL string) {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	delete(s.discoveredPeers, peerURL)
	delete(s.peerNodeByURL, peerURL)
	delete(s.peerSync, peerURL)
}

func (s *syncServer) recordPeerIdentity(peerURL, nodeID string) {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if s.peerNodeByURL == nil {
		s.peerNodeByURL = map[string]string{}
	}
	s.peerNodeByURL[normalizePeerURL(peerURL)] = strings.TrimSpace(nodeID)
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
	filtered := make([]events.SignedEvent, 0, len(all))
	for _, e := range all {
		if e.ProjectID == projectID {
			filtered = append(filtered, e)
		}
	}
	// Deterministic replay order: independent from ingestion order after partition/rejoin.
	sort.Slice(filtered, func(i, j int) bool {
		a := filtered[i]
		b := filtered[j]
		at := a.Timestamp.UTC().UnixNano()
		bt := b.Timestamp.UTC().UnixNano()
		if at != bt {
			return at < bt
		}
		if a.SignerID != b.SignerID {
			return a.SignerID < b.SignerID
		}
		if a.Seq != b.Seq {
			return a.Seq < b.Seq
		}
		if a.EntityID != b.EntityID {
			return a.EntityID < b.EntityID
		}
		return a.Type < b.Type
	})

	issues := make(map[string]issueProjection)
	for _, e := range filtered {
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

func printBoardPlain(projectID string, board, boardAll map[string][]issueProjection, revision int, viewMode, assigneeID string) {
	fmt.Print(renderBoardPlain(projectID, board, boardAll, revision, viewMode, assigneeID))
}

func printBoardCounts(projectID string, board, boardAll map[string][]issueProjection, revision int, viewMode, assigneeID string) {
	const assigneeWIPLimit = 3
	totalIssues := 0
	allTotal := 0
	doneIssues := len(board["done"])
	allDone := len(boardAll["done"])
	for _, items := range board {
		totalIssues += len(items)
	}
	for _, items := range boardAll {
		allTotal += len(items)
	}
	openIssues := totalIssues - doneIssues
	allOpen := allTotal - allDone
	scope := "all"
	if viewMode == "mine" {
		scope = "mine"
		if strings.TrimSpace(assigneeID) != "" {
			scope += "(" + strings.TrimSpace(assigneeID) + ")"
		}
	}
	fmt.Printf("Project: %s  Revision: %d\n", projectID, revision)
	fmt.Printf("Assignee WIP limit: %d\n", assigneeWIPLimit)
	fmt.Printf("Scope: %s\n", scope)
	fmt.Printf("Displayed issues: %d  Open: %d  Done: %d\n", totalIssues, openIssues, doneIssues)
	fmt.Printf("All issues: %d  Open: %d  Done: %d\n", allTotal, allOpen, allDone)
	fmt.Printf("To Do=%d In Progress=%d Code Review=%d Testing=%d Done=%d\n",
		len(board["todo"]), len(board["in_progress"]), len(board["code_review"]), len(board["testing"]), len(board["done"]))
}

func renderBoardPlain(projectID string, board, boardAll map[string][]issueProjection, revision int, viewMode, assigneeID string) string {
	const boardCellWidth = 34
	const assigneeWIPLimit = 3
	cols := []struct {
		Key   string
		Title string
	}{
		{Key: "todo", Title: "To Do"},
		{Key: "in_progress", Title: "In Progress"},
		{Key: "code_review", Title: "Code Review"},
		{Key: "testing", Title: "Testing"},
		{Key: "done", Title: "Done"},
	}
	totalIssues := 0
	allTotal := 0
	doneIssues := len(board["done"])
	allDone := len(boardAll["done"])
	for _, c := range cols {
		totalIssues += len(board[c.Key])
		allTotal += len(boardAll[c.Key])
	}
	openIssues := totalIssues - doneIssues
	allOpen := allTotal - allDone
	scope := "all"
	if viewMode == "mine" {
		scope = "mine"
		if strings.TrimSpace(assigneeID) != "" {
			scope += "(" + strings.TrimSpace(assigneeID) + ")"
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Project: %s  Revision: %d\n", projectID, revision)
	fmt.Fprintf(&out, "Assignee WIP limit: %d\n", assigneeWIPLimit)
	fmt.Fprintf(&out, "Scope: %s\n", scope)
	fmt.Fprintf(&out, "Displayed issues: %d  Open: %d  Done: %d\n", totalIssues, openIssues, doneIssues)
	fmt.Fprintf(&out, "All issues: %d  Open: %d  Done: %d\n", allTotal, allOpen, allDone)
	sep := "+" + strings.Repeat(strings.Repeat("-", boardCellWidth)+"+", len(cols))
	out.WriteString(sep + "\n")
	headerCells := make([]string, 0, len(cols))
	for _, c := range cols {
		h := fmt.Sprintf("%s [%d]", c.Title, len(board[c.Key]))
		headerCells = append(headerCells, padOrTrim(h, boardCellWidth))
	}
	fmt.Fprintf(&out, "|%s|\n", strings.Join(headerCells, "|"))
	out.WriteString(sep + "\n")

	maxRows := 0
	for _, c := range cols {
		if n := len(board[c.Key]); n > maxRows {
			maxRows = n
		}
	}
	for row := 0; row < maxRows; row++ {
		line := make([]string, 0, len(cols))
		for _, c := range cols {
			if row >= len(board[c.Key]) {
				line = append(line, strings.Repeat(" ", boardCellWidth))
				continue
			}
			it := board[c.Key][row]
			assignee := strings.TrimSpace(it.Assignee)
			if assignee == "" {
				assignee = "unassigned"
			}
			card := fmt.Sprintf("%s %s @%s", it.ID, strings.TrimSpace(it.Summary), assignee)
			line = append(line, padOrTrim(card, boardCellWidth))
		}
		fmt.Fprintf(&out, "|%s|\n", strings.Join(line, "|"))
	}
	if maxRows == 0 {
		empty := make([]string, 0, len(cols))
		for range cols {
			empty = append(empty, strings.Repeat(" ", boardCellWidth))
		}
		fmt.Fprintf(&out, "|%s|\n", strings.Join(empty, "|"))
	}
	out.WriteString(sep + "\n")
	return out.String()
}

func padOrTrim(s string, w int) string {
	rs := []rune(s)
	if len(rs) > w {
		if w <= 3 {
			return string(rs[:w])
		}
		return string(rs[:w-3]) + "..."
	}
	if len(rs) < w {
		return s + strings.Repeat(" ", w-len(rs))
	}
	return s
}

func filterBoardByAssignee(board map[string][]issueProjection, assignee string) map[string][]issueProjection {
	target := strings.TrimSpace(assignee)
	out := map[string][]issueProjection{
		"todo":        {},
		"in_progress": {},
		"code_review": {},
		"testing":     {},
		"done":        {},
	}
	for k, items := range board {
		filtered := make([]issueProjection, 0, len(items))
		for _, it := range items {
			if strings.TrimSpace(it.Assignee) == target {
				filtered = append(filtered, it)
			}
		}
		out[k] = filtered
	}
	return out
}

func printUsage() {
	fmt.Println("team-cli-tracker node")
	fmt.Println("usage:")
	fmt.Println("  node identity --node-id node-1 --data-dir ./data")
	fmt.Println("  node issue create --project-id OPS --issue-id OPS-1 --summary \"...\" [--priority high] [--assignee user]")
	fmt.Println("  node issue transition --project-id OPS --issue-id OPS-1 --from todo --to in_progress [--policy-url http://127.0.0.1:4101]")
	fmt.Println("  node issue comment --project-id OPS --issue-id OPS-1 --text \"...\"")
	fmt.Println("  node board --project-id OPS [--format plain|json] [--view all|mine] [--assignee-id u1] [--counts-only] [--once] [--refresh 2s] [--peers http://127.0.0.1:4102]")
	fmt.Println("  node storage migrate [--data-dir ./data]")
	fmt.Println("  node storage enable-encryption [--data-dir ./data]")
	fmt.Println("  node storage rotate-key [--data-dir ./data] [--enforce-due] [--max-age 720h]")
	fmt.Println("  node storage verify-integrity [--data-dir ./data]")
	fmt.Println("  node storage key-policy-check [--data-dir ./data] [--max-age 720h]")
	fmt.Println("  node storage recovery-drill [--data-dir ./data]")
	fmt.Println("  node trust invite --node-id node-x [--ttl-sec 3600]")
	fmt.Println("  node trust use-invite --node-id node-x --token <token>")
	fmt.Println("  node trust revoke --node-id node-x")
	fmt.Println("  node trust list")
	fmt.Println("  node auth issue --user-id u1 [--role dev] [--ttl-sec 3600]")
	fmt.Println("  node auth revoke --token <token>")
	fmt.Println("  node auth list")
	fmt.Println("  node auth bind-role --user-id u1 --role lead")
	fmt.Println("  node auth list-bindings")
	fmt.Println("  node audit export [--all] [--from RFC3339] [--to RFC3339] [--user u1,u2] [--format jsonl|csv] [--limit N --cursor K]")
	fmt.Println("  node audit verify-integrity")
	fmt.Println("  node team onboard --user-id u1 --role dev [--duty]")
	fmt.Println("  node team role-change --user-id u1 --role lead [--duty]")
	fmt.Println("  node team offboard --project-id OPS --user-id u1")
	fmt.Println("  node team list")
	fmt.Println("  node serve ... [--rate-limit-per-min 120] [--rate-limit-sensitive-per-min 30] [--discovery-enabled true] [--public-url http://node-1:4101]")
	fmt.Println("  node serve --project-id OPS --listen :4101 --node-role admin --preferred-leader node-1 --peers http://127.0.0.1:4102,http://127.0.0.1:4103")
}

func parseAuditTimeRange(fromRaw, toRaw string) (*time.Time, *time.Time, error) {
	var fromTS *time.Time
	var toTS *time.Time
	if v := strings.TrimSpace(fromRaw); v != "" {
		tm, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --from value: %w", err)
		}
		u := tm.UTC()
		fromTS = &u
	}
	if v := strings.TrimSpace(toRaw); v != "" {
		tm, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --to value: %w", err)
		}
		u := tm.UTC()
		toTS = &u
	}
	if fromTS != nil && toTS != nil && fromTS.After(*toTS) {
		return nil, nil, fmt.Errorf("--from must be <= --to")
	}
	return fromTS, toTS, nil
}

func parseUserFilter(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		out[v] = struct{}{}
	}
	return out
}

func filterAuditEvents(in []audit.Event, fromTS, toTS *time.Time, users map[string]struct{}) []audit.Event {
	out := make([]audit.Event, 0, len(in))
	for _, e := range in {
		if fromTS != nil || toTS != nil {
			ts, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Time))
			if err != nil {
				continue
			}
			tu := ts.UTC()
			if fromTS != nil && tu.Before(*fromTS) {
				continue
			}
			if toTS != nil && tu.After(*toTS) {
				continue
			}
		}
		if len(users) > 0 {
			if _, ok := users[strings.TrimSpace(e.Actor)]; !ok {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

func paginateAuditEvents(in []audit.Event, limit int, cursor string) ([]audit.Event, string, error) {
	offset := 0
	v := strings.TrimSpace(cursor)
	if v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("invalid --cursor value")
		}
		offset = n
	}
	if offset >= len(in) {
		return []audit.Event{}, "", nil
	}
	if limit <= 0 {
		return in[offset:], "", nil
	}
	end := minInt(offset+limit, len(in))
	next := ""
	if end < len(in) {
		next = strconv.Itoa(end)
	}
	return in[offset:end], next, nil
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
	seen := map[string]struct{}{}
	for _, v := range items {
		v = strings.TrimSpace(v)
		if v != "" && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			v = "http://" + v
		}
		v = normalizePeerURL(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
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

func requestTeamOnboardVote(client *http.Client, peerToken, peerBaseURL string, in teamOnboardRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-team-onboard", bytes.NewReader(raw))
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

func requestTeamRoleChangeVote(client *http.Client, peerToken, peerBaseURL string, in teamRoleChangeRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-team-role-change", bytes.NewReader(raw))
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

func requestGovernanceNodeRoleVote(client *http.Client, peerToken, peerBaseURL string, in governanceNodeRoleRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-governance-node-role", bytes.NewReader(raw))
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

func requestTrustInviteVote(client *http.Client, peerToken, peerBaseURL string, in trustInviteRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-trust-invite", bytes.NewReader(raw))
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

func requestTrustRevokeVote(client *http.Client, peerToken, peerBaseURL string, in trustRevokeRequest) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := json.Marshal(in)
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(peerBaseURL, "/")+"/raft/vote-trust-revoke", bytes.NewReader(raw))
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
		if !s.allowRequest(r) {
			s.rateLimitDenied.Add(1)
			switch s.rateLimitTier(r.URL.Path) {
			case "sensitive":
				s.rateLimitDeniedSensitive.Add(1)
			default:
				s.rateLimitDeniedDefault.Add(1)
			}
			if s.auditManager != nil {
				s.auditManager.Append("rate_limit", s.rateLimitKey(r), "deny", map[string]any{
					"path": r.URL.Path,
					"tier": s.rateLimitTier(r.URL.Path),
				})
			}
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		if !s.authEnabled {
			next(w, r)
			return
		}
		if _, ok := s.authenticate(r); !ok {
			s.authnDenied.Add(1)
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
		if !s.allowRequest(r) {
			s.rateLimitDenied.Add(1)
			switch s.rateLimitTier(r.URL.Path) {
			case "sensitive":
				s.rateLimitDeniedSensitive.Add(1)
			default:
				s.rateLimitDeniedDefault.Add(1)
			}
			if s.auditManager != nil {
				s.auditManager.Append("rate_limit", s.rateLimitKey(r), "deny", map[string]any{
					"path": r.URL.Path,
					"tier": s.rateLimitTier(r.URL.Path),
				})
			}
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		if !s.authEnabled {
			next(w, r)
			return
		}
		p, ok := s.authenticate(r)
		if !ok {
			s.authnDenied.Add(1)
			if s.auditManager != nil {
				s.auditManager.Append("authn", s.actorFromReq(r), "deny", map[string]any{"path": r.URL.Path})
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(p.Role))]; !ok {
			s.authzDenied.Add(1)
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
	if s.authManager != nil {
		if p, ok := s.authManager.Validate(token); ok {
			return authPrincipal{
				UserID: p.UserID,
				Role:   p.Role,
				Active: p.Active,
			}, true
		}
	}
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

func buildServerTLSConfig(caFile, crlFile string, mtlsRequired bool) (*tls.Config, error) {
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
	revoked, err := loadRevokedSerials(crlFile)
	if err != nil {
		return nil, err
	}
	if len(revoked) > 0 {
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("missing peer certificate")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			if revoked[cert.SerialNumber.String()] {
				return fmt.Errorf("client certificate is revoked")
			}
			return nil
		}
	}
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

type serveSecurityPolicyInput struct {
	SecureModeRequired bool
	AuthEnabled        bool
	HasPeers           bool
	TLSCert            string
	TLSKey             string
	TLSCA              string
	MTLSRequired       bool
	ClientCert         string
	ClientKey          string
	PeerToken          string
}

func validateServeSecurityPolicy(in serveSecurityPolicyInput) error {
	tlsCert := strings.TrimSpace(in.TLSCert)
	tlsKey := strings.TrimSpace(in.TLSKey)
	tlsCA := strings.TrimSpace(in.TLSCA)
	clientCert := strings.TrimSpace(in.ClientCert)
	clientKey := strings.TrimSpace(in.ClientKey)
	peerToken := strings.TrimSpace(in.PeerToken)

	if (tlsCert == "") != (tlsKey == "") {
		return fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE must be provided together")
	}
	if (clientCert == "") != (clientKey == "") {
		return fmt.Errorf("CLIENT_TLS_CERT_FILE and CLIENT_TLS_KEY_FILE must be provided together")
	}
	if in.MTLSRequired {
		if tlsCert == "" || tlsKey == "" {
			return fmt.Errorf("mTLS requires server TLS cert/key")
		}
		if tlsCA == "" {
			return fmt.Errorf("mTLS requires TLS_CA_FILE")
		}
		if clientCert == "" || clientKey == "" {
			return fmt.Errorf("mTLS requires client TLS cert/key for peer requests")
		}
	}
	if !in.SecureModeRequired {
		return nil
	}
	if in.HasPeers {
		if !in.AuthEnabled {
			return fmt.Errorf("secure mode requires AUTH_ENABLED=true when peers are configured")
		}
		if tlsCert == "" || tlsKey == "" {
			return fmt.Errorf("secure mode requires TLS_CERT_FILE/TLS_KEY_FILE when peers are configured")
		}
		if tlsCA == "" {
			return fmt.Errorf("secure mode requires TLS_CA_FILE when peers are configured")
		}
		if clientCert == "" || clientKey == "" {
			return fmt.Errorf("secure mode requires CLIENT_TLS_CERT_FILE/CLIENT_TLS_KEY_FILE when peers are configured")
		}
		if peerToken == "" {
			return fmt.Errorf("secure mode requires PEER_TOKEN when peers are configured")
		}
	}
	return nil
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

func loadRevokedSerials(crlFile string) (map[string]bool, error) {
	out := map[string]bool{}
	crlFile = strings.TrimSpace(crlFile)
	if crlFile == "" {
		return out, nil
	}
	raw, err := os.ReadFile(crlFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("failed to parse CRL file")
	}
	rl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		return nil, err
	}
	for _, rc := range rl.RevokedCertificateEntries {
		out[rc.SerialNumber.String()] = true
	}
	return out, nil
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
	ip := clientIPFromReq(r)
	if ip == "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return ip
}

func (s *syncServer) rateLimitKey(r *http.Request) string {
	if p, ok := s.authenticate(r); ok {
		return "token:" + p.UserID
	}
	ip := clientIPFromReq(r)
	if ip == "" {
		ip = "unknown"
	}
	return "ip:" + ip
}

func clientIPFromReq(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			ip := strings.TrimSpace(part)
			if ip != "" {
				return ip
			}
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remote)
	if err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(remote, "[]")
}

func (s *syncServer) allowRequest(r *http.Request) bool {
	key := s.rateLimitKey(r)
	switch s.rateLimitTier(r.URL.Path) {
	case "sensitive":
		if s.sensitiveRateLimiter == nil {
			return true
		}
		return s.sensitiveRateLimiter.Allow(key)
	default:
		if s.rateLimiter == nil {
			return true
		}
		return s.rateLimiter.Allow(key)
	}
}

func (s *syncServer) rateLimitTier(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasPrefix(path, "/raft/"):
		return "sensitive"
	case strings.HasPrefix(path, "/governance/"):
		return "sensitive"
	case strings.HasPrefix(path, "/team/"):
		return "sensitive"
	case strings.HasPrefix(path, "/trust/"):
		return "sensitive"
	case strings.HasPrefix(path, "/security/"):
		return "sensitive"
	default:
		return "default"
	}
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

func mustDuration(raw string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func tickerChan(t *time.Ticker, state *boardRenderState) <-chan time.Time {
	if t == nil || (state != nil && state.interactivePaused) {
		return nil
	}
	return t.C
}

func normalizePeerURL(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return ""
	}
	u, err := url.Parse(v)
	if err != nil || strings.TrimSpace(u.Host) == "" {
		return ""
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}
