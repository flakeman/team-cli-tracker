package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Identity struct {
	NodeID string
	Priv   ed25519.PrivateKey
	Pub    ed25519.PublicKey
}

func LoadOrCreate(dataDir, nodeID string) (Identity, error) {
	if strings.TrimSpace(nodeID) == "" {
		return Identity{}, fmt.Errorf("node_id is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return Identity{}, err
	}
	keyPath := filepath.Join(dataDir, "node_ed25519.key")
	pubPath := filepath.Join(dataDir, "node_ed25519.pub")

	if _, err := os.Stat(keyPath); err == nil {
		privRaw, err := os.ReadFile(keyPath)
		if err != nil {
			return Identity{}, err
		}
		privBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(privRaw)))
		if err != nil {
			return Identity{}, err
		}
		if len(privBytes) != ed25519.PrivateKeySize {
			return Identity{}, fmt.Errorf("invalid private key size")
		}
		priv := ed25519.PrivateKey(privBytes)
		pub := priv.Public().(ed25519.PublicKey)
		return Identity{NodeID: nodeID, Priv: priv, Pub: pub}, nil
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600); err != nil {
		return Identity{}, err
	}
	if err := os.WriteFile(pubPath, []byte(base64.StdEncoding.EncodeToString(pub)), 0o644); err != nil {
		return Identity{}, err
	}
	return Identity{NodeID: nodeID, Priv: priv, Pub: pub}, nil
}
