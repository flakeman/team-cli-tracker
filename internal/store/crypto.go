package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type keyRing struct {
	ActiveKeyID string            `json:"active_key_id"`
	Keys        map[string]string `json:"keys"`
}

func loadKeyRing(dataDir string) (*keyRing, string, error) {
	path := filepath.Join(dataDir, "events.keys.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	var kr keyRing
	if err := json.Unmarshal(raw, &kr); err != nil {
		return nil, path, err
	}
	if kr.Keys == nil {
		kr.Keys = map[string]string{}
	}
	return &kr, path, nil
}

func saveKeyRing(path string, kr *keyRing) error {
	raw, err := json.MarshalIndent(kr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptAESGCM(key, plaintext []byte) (nonce, ciphertext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

func decryptAESGCM(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (l *EventLog) EnableEncryption() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.keyRing != nil && l.keyRing.ActiveKeyID != "" {
		return l.keyRing.ActiveKeyID, nil
	}
	key, err := generateKey()
	if err != nil {
		return "", err
	}
	keyID := fmt.Sprintf("k-%d", time.Now().UnixNano())
	kr := &keyRing{
		ActiveKeyID: keyID,
		Keys: map[string]string{
			keyID: base64.StdEncoding.EncodeToString(key),
		},
	}
	if err := saveKeyRing(l.keyRingPath, kr); err != nil {
		return "", err
	}
	l.keyRing = kr
	// Rewrite existing data so payloads are encrypted at rest.
	all, err := l.readAllUnlocked()
	if err != nil {
		return "", err
	}
	if err := l.rewriteAllUnlocked(all); err != nil {
		return "", err
	}
	return keyID, nil
}

func (l *EventLog) RotateEncryptionKey() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.keyRing == nil || l.keyRing.ActiveKeyID == "" {
		return "", fmt.Errorf("encryption is not enabled")
	}
	key, err := generateKey()
	if err != nil {
		return "", err
	}
	keyID := fmt.Sprintf("k-%d", time.Now().UnixNano())
	l.keyRing.ActiveKeyID = keyID
	l.keyRing.Keys[keyID] = base64.StdEncoding.EncodeToString(key)
	if err := saveKeyRing(l.keyRingPath, l.keyRing); err != nil {
		return "", err
	}
	all, err := l.readAllUnlocked()
	if err != nil {
		return "", err
	}
	if err := l.rewriteAllUnlocked(all); err != nil {
		return "", err
	}
	return keyID, nil
}

func (l *EventLog) activeKey() (keyID string, key []byte, err error) {
	if l.keyRing == nil || l.keyRing.ActiveKeyID == "" {
		return "", nil, nil
	}
	raw, ok := l.keyRing.Keys[l.keyRing.ActiveKeyID]
	if !ok {
		return "", nil, fmt.Errorf("active key id not found in keyring")
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", nil, err
	}
	return l.keyRing.ActiveKeyID, decoded, nil
}

func (l *EventLog) keyByID(keyID string) ([]byte, error) {
	if l.keyRing == nil {
		return nil, fmt.Errorf("keyring is not enabled")
	}
	raw, ok := l.keyRing.Keys[keyID]
	if !ok {
		return nil, fmt.Errorf("key id not found: %s", keyID)
	}
	return base64.StdEncoding.DecodeString(raw)
}
