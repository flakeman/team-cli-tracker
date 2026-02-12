package governance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Manager struct {
	mu   sync.Mutex
	path string
	doc  document
}

type document struct {
	Nodes map[string]Node `json:"nodes"`
}

type Node struct {
	NodeID string `json:"node_id"`
	Role   string `json:"role"` // voting | non_voting
	Active bool   `json:"active"`
}

func Open(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		path: filepath.Join(dataDir, "governance.json"),
		doc:  document{Nodes: map[string]Node{}},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) SetNodeRole(nodeID, role string, active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if nodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if role != "voting" && role != "non_voting" {
		return fmt.Errorf("role must be voting or non_voting")
	}
	m.doc.Nodes[nodeID] = Node{
		NodeID: nodeID,
		Role:   role,
		Active: active,
	}
	return m.save()
}

func (m *Manager) List() []Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Node, 0, len(m.doc.Nodes))
	for _, v := range m.doc.Nodes {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

func (m *Manager) VotingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, v := range m.doc.Nodes {
		if v.Active && v.Role == "voting" {
			n++
		}
	}
	return n
}

func (m *Manager) IsVoting(nodeID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.doc.Nodes[nodeID]
	return ok && n.Active && n.Role == "voting"
}

func (m *Manager) ReplaceVotingSet(votingIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(votingIDs) == 0 {
		return fmt.Errorf("voting set cannot be empty")
	}
	if len(votingIDs)%2 == 0 {
		return fmt.Errorf("voting set must be odd-sized")
	}
	target := map[string]struct{}{}
	for _, id := range votingIDs {
		if id == "" {
			continue
		}
		target[id] = struct{}{}
	}
	if len(target)%2 == 0 {
		return fmt.Errorf("voting set must be odd-sized after dedupe")
	}
	// Keep existing nodes and convert role/active by target membership.
	for id, n := range m.doc.Nodes {
		if _, ok := target[id]; ok {
			n.Role = "voting"
			n.Active = true
			m.doc.Nodes[id] = n
		} else {
			n.Role = "non_voting"
			// keep active state as-is for non-voting members
			m.doc.Nodes[id] = n
		}
	}
	// Add missing ids as active voting nodes.
	for id := range target {
		if _, ok := m.doc.Nodes[id]; ok {
			continue
		}
		m.doc.Nodes[id] = Node{
			NodeID: id,
			Role:   "voting",
			Active: true,
		}
	}
	return m.save()
}

func (m *Manager) load() error {
	raw, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return m.save()
	}
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &m.doc); err != nil {
		return err
	}
	if m.doc.Nodes == nil {
		m.doc.Nodes = map[string]Node{}
	}
	return nil
}

func (m *Manager) save() error {
	raw, err := json.MarshalIndent(m.doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, raw, 0o644)
}
