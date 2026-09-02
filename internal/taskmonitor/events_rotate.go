package taskmonitor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Event-log rotation bounds (P0.4/N7): once a task's events.jsonl reaches
// maxEventsBeforeRotate lines, the next append collapses it to the first event,
// the most recent rotateKeepTail events, and an events_rotated marker.
const (
	maxEventsBeforeRotate = 1000
	rotateKeepTail        = 20
)

// parseEventLines splits JSONL bytes into the valid events they carry and the
// largest sequence seen. Corrupt lines are skipped — they cannot be trusted
// for sequence assignment.
func parseEventLines(raw []byte) ([]TaskEvent, int) {
	var events []TaskEvent
	max := 0
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var existing TaskEvent
		if err := json.Unmarshal([]byte(line), &existing); err != nil {
			continue
		}
		if existing.Sequence > max {
			max = existing.Sequence
		}
		events = append(events, existing)
	}
	return events, max
}

// rotateEvents rewrites a long event log as first + tail + summary marker and
// appends the new event, keeping the file small and the sequence monotonic.
func (s *FileStore) rotateEvents(f *os.File, existing []TaskEvent, ev TaskEvent, max int) error {
	tail := existing[len(existing)-rotateKeepTail:]
	kept := append([]TaskEvent{existing[0]}, tail...)
	marker := TaskEvent{
		Sequence:  max + 1,
		Timestamp: ev.Timestamp,
		EventType: "events_rotated",
		TaskID:    ev.TaskID,
		SessionID: ev.SessionID,
		State:     tail[len(tail)-1].State,
		Detail:    fmt.Sprintf("collapsed %d events", len(existing)-len(kept)),
	}
	if err := marker.Validate(); err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	ev.Sequence = max + 2
	lines := make([][]byte, 0, len(kept)+2)
	for _, e := range append(kept, marker, ev) {
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		lines = append(lines, append(data, '\n'))
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := f.Write(line); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("append audit event: sync: %w", err)
	}
	return nil
}
