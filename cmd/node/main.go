package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
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
	_ = fs.Parse(args)

	if *projectID == "" || *issueID == "" || *to == "" {
		fatal(fmt.Errorf("project-id, issue-id and to are required"))
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
	fmt.Println("  node issue transition --project-id OPS --issue-id OPS-1 --from todo --to in_progress")
	fmt.Println("  node issue comment --project-id OPS --issue-id OPS-1 --text \"...\"")
	fmt.Println("  node board --project-id OPS [--format plain|json]")
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
