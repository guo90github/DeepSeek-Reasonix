package taskmonitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func appendEvent(t *testing.T, store WriteStore, project, taskID string, eventType string) int {
	t.Helper()
	ev := TaskEvent{Timestamp: time.Now(), EventType: eventType, TaskID: taskID, SessionID: "s", State: TaskStateRunning}
	if err := store.AppendAuditEvent(context.Background(), project, ev); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(context.Background(), project, taskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return events[len(events)-1].Sequence
}

func TestAppendAuditEventRotatesLongLog(t *testing.T) {
	project := t.TempDir()
	store := NewFileStore(".reasonix/tasks")
	// Pre-fill the log to exactly the rotation threshold.
	eventsPath := filepath.Join(project, ".reasonix", "tasks", "t1", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(eventsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	body := &strings.Builder{}
	base := time.Now()
	for i := 1; i <= maxEventsBeforeRotate; i++ {
		ev := TaskEvent{Sequence: i, Timestamp: base, EventType: "state_change", TaskID: "t1", SessionID: "s", State: TaskStateRunning}
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		body.Write(data)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(eventsPath, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	// The next append collapses the log.
	seq := appendEvent(t, store, project, "t1", "state_change")
	if seq != maxEventsBeforeRotate+2 {
		t.Fatalf("new event must follow the rotation marker, got sequence %d", seq)
	}
	rotated, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := parseEventLines(rotated)
	if len(events) > rotateKeepTail+3 {
		t.Fatalf("rotated log must stay small, got %d events", len(events))
	}
	if events[0].Sequence != 1 {
		t.Fatalf("the first event must be preserved, got sequence %d", events[0].Sequence)
	}
	marker := events[len(events)-2]
	if marker.EventType != "events_rotated" {
		t.Fatalf("expected an events_rotated summary, got %q", marker.EventType)
	}
	if !strings.Contains(marker.Detail, "collapsed") {
		t.Fatalf("marker must carry the collapsed count, got %q", marker.Detail)
	}
	if marker.Sequence != maxEventsBeforeRotate+1 {
		t.Fatalf("marker must take the next sequence, got %d", marker.Sequence)
	}
	if events[len(events)-1].Sequence != maxEventsBeforeRotate+2 {
		t.Fatalf("new event must be last, got %d", events[len(events)-1].Sequence)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			t.Fatalf("rotated log must stay monotonic at %d", i)
		}
	}

	// ListEvents must return the marker and the new event after the cursor.
	after, err := store.ListEvents(context.Background(), project, "t1", maxEventsBeforeRotate)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || after[0].EventType != "events_rotated" || after[1].Sequence != maxEventsBeforeRotate+2 {
		t.Fatalf("ListEvents after cursor must see the rotation, got %+v", after)
	}
}

func TestAppendAuditEventTrimsDanglingPartialLine(t *testing.T) {
	project := t.TempDir()
	store := NewFileStore(".reasonix/tasks")
	appendEvent(t, store, project, "t1", "state_change") // sequence 1
	eventsPath := filepath.Join(project, ".reasonix", "tasks", "t1", "events.jsonl")
	// Simulate a crash mid-write: a partial line with no trailing newline.
	if err := os.WriteFile(eventsPath, []byte(`{"sequence":1,"timestamp":"2026-01-01T00:00:00Z","event_type":"state_change","task_id":"t1","session_id":"s","state":"running"}
{"sequence":2,`), 0o600); err != nil {
		t.Fatal(err)
	}

	seq := appendEvent(t, store, project, "t1", "error")
	if seq != 2 {
		t.Fatalf("dangling partial line must not advance the sequence, got %d", seq)
	}
	body, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := parseEventLines(body)
	if len(events) != 2 {
		t.Fatalf("expected 2 valid events after recovery, got %d", len(events))
	}
	if events[1].EventType != "error" || events[1].Sequence != 2 {
		t.Fatalf("the appended event must be a standalone valid line, got %+v", events[1])
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Fatal("appended event must end with a newline")
	}
}

func TestTaskEventValidateDetailTooLong(t *testing.T) {
	ev := TaskEvent{Sequence: 1, Timestamp: time.Now(), EventType: "events_rotated", TaskID: "t1", SessionID: "s",
		State: TaskStateFailed, Detail: strings.Repeat("x", maxErrorSummaryLen+1)}
	if err := ev.Validate(); err == nil {
		t.Fatal("oversized Detail must fail validation")
	}
}
