package events

type SignedEvent struct {
	ProjectID string
	EntityID  string
	Type      string
	Payload   []byte
	SignerID  string
	Signature []byte
	Seq       uint64
}
