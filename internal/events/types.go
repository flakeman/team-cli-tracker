package events

import "time"

type SignedEvent struct {
	Version   int       `json:"version"`
	ProjectID string    `json:"project_id"`
	EntityID  string    `json:"entity_id"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	SignerID  string    `json:"signer_id"`
	SignerPub []byte    `json:"signer_pub"`
	Signature []byte    `json:"signature"`
	Seq       uint64    `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
}
