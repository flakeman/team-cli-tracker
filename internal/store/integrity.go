package store

import (
	"fmt"

	"github.com/vladimir/team-cli-tracker/internal/events"
)

func (l *EventLog) VerifyIntegrity() error {
	all, err := l.ReadAll()
	if err != nil {
		return err
	}
	last := make(map[string]uint64)
	for _, e := range all {
		if err := events.VerifyByEventKey(e); err != nil {
			return fmt.Errorf("signature verify failed for %s/%s/%d: %w", e.ProjectID, e.SignerID, e.Seq, err)
		}
		key := e.ProjectID + "|" + e.SignerID
		if e.Seq != last[key]+1 {
			return fmt.Errorf("sequence gap for %s: got=%d want=%d", key, e.Seq, last[key]+1)
		}
		last[key] = e.Seq
	}
	return nil
}
