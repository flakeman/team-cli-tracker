package events

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func Sign(priv ed25519.PrivateKey, e SignedEvent) ([]byte, error) {
	msg, err := signingMessage(e)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(priv, msg)
	return sig, nil
}

func Verify(pub ed25519.PublicKey, e SignedEvent) error {
	if len(e.Signature) == 0 {
		return fmt.Errorf("signature is empty")
	}
	msg, err := signingMessage(e)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, msg, e.Signature) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func signingMessage(e SignedEvent) ([]byte, error) {
	if e.ProjectID == "" || e.EntityID == "" || e.Type == "" || e.SignerID == "" {
		return nil, fmt.Errorf("missing required signing fields")
	}
	// Signature field is intentionally excluded from the digest.
	h := sha256.New()
	_, _ = h.Write([]byte(fmt.Sprintf(
		"v=%d|p=%s|e=%s|t=%s|sid=%s|seq=%d|ts=%s|payload=%s",
		e.Version,
		e.ProjectID,
		e.EntityID,
		e.Type,
		e.SignerID,
		e.Seq,
		e.Timestamp.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		base64.StdEncoding.EncodeToString(e.Payload),
	)))
	return h.Sum(nil), nil
}
