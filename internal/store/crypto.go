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
	"strconv"
	"strings"
	"time"
)

type keyRing struct {
	ActiveKeyID string            `json:"active_key_id"`
	Keys        map[string]string `json:"keys"`
}

type externalKeyProvider interface {
	Put(keyID string, key []byte) error
	Get(keyID string) ([]byte, error)
	Name() string
}

type fileKeyProvider struct {
	path string
}

type fileKeyStore struct {
	Keys map[string]string `json:"keys"`
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

func initExternalKeyProviderFromEnv() (externalKeyProvider, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("EVENT_KEY_PROVIDER")))
	if provider == "" {
		return nil, nil
	}
	switch provider {
	case "file":
		path := strings.TrimSpace(os.Getenv("EVENT_KEY_PROVIDER_FILE"))
		if path == "" {
			return nil, fmt.Errorf("EVENT_KEY_PROVIDER_FILE is required when EVENT_KEY_PROVIDER=file")
		}
		return &fileKeyProvider{path: path}, nil
	default:
		return nil, fmt.Errorf("unsupported EVENT_KEY_PROVIDER: %s", provider)
	}
}

func (p *fileKeyProvider) Name() string { return "file" }

func (p *fileKeyProvider) Put(keyID string, key []byte) error {
	store, err := p.load()
	if err != nil {
		return err
	}
	if store.Keys == nil {
		store.Keys = map[string]string{}
	}
	store.Keys[keyID] = base64.StdEncoding.EncodeToString(key)
	return p.save(store)
}

func (p *fileKeyProvider) Get(keyID string) ([]byte, error) {
	store, err := p.load()
	if err != nil {
		return nil, err
	}
	raw, ok := store.Keys[keyID]
	if !ok {
		return nil, fmt.Errorf("external key not found: %s", keyID)
	}
	return base64.StdEncoding.DecodeString(raw)
}

func (p *fileKeyProvider) load() (*fileKeyStore, error) {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p.path)
	if os.IsNotExist(err) {
		return &fileKeyStore{Keys: map[string]string{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s fileKeyStore
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.Keys == nil {
		s.Keys = map[string]string{}
	}
	return &s, nil
}

func (p *fileKeyProvider) save(s *fileKeyStore) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.path, raw, 0o600)
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
		Keys:        map[string]string{},
	}
	if l.keyProvider != nil {
		if err := l.keyProvider.Put(keyID, key); err != nil {
			return "", err
		}
		kr.Keys[keyID] = ""
	} else {
		kr.Keys[keyID] = base64.StdEncoding.EncodeToString(key)
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
	if l.keyProvider != nil {
		if err := l.keyProvider.Put(keyID, key); err != nil {
			return "", err
		}
		l.keyRing.Keys[keyID] = ""
	} else {
		l.keyRing.Keys[keyID] = base64.StdEncoding.EncodeToString(key)
	}
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
	if strings.TrimSpace(raw) == "" && l.keyProvider != nil {
		key, err := l.keyProvider.Get(l.keyRing.ActiveKeyID)
		if err != nil {
			return "", nil, err
		}
		return l.keyRing.ActiveKeyID, key, nil
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
	if strings.TrimSpace(raw) == "" && l.keyProvider != nil {
		return l.keyProvider.Get(keyID)
	}
	return base64.StdEncoding.DecodeString(raw)
}

func (l *EventLog) ActiveEncryptionKeyInfo() (keyID string, createdAt time.Time, enabled bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.keyRing == nil || l.keyRing.ActiveKeyID == "" {
		return "", time.Time{}, false, nil
	}
	ts, err := keyIDToTime(l.keyRing.ActiveKeyID)
	if err != nil {
		return l.keyRing.ActiveKeyID, time.Time{}, true, err
	}
	return l.keyRing.ActiveKeyID, ts, true, nil
}

func keyIDToTime(keyID string) (time.Time, error) {
	keyID = strings.TrimSpace(keyID)
	if !strings.HasPrefix(keyID, "k-") {
		return time.Time{}, fmt.Errorf("unexpected key id format: %s", keyID)
	}
	raw := strings.TrimPrefix(keyID, "k-")
	nanos, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, nanos).UTC(), nil
}
