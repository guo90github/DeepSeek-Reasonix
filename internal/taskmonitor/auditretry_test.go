package taskmonitor

import (
	"context"
	"errors"
	"testing"

	"reasonix/internal/jobs"
)

// flakyAuditStore fails AppendAuditEvent the first failAfter times, then
// delegates. SaveTask and everything else pass through to the real store.
type flakyAuditStore struct {
	*FileStore
	failAfter int
	attempts  int
}

func (s *flakyAuditStore) AppendAuditEvent(ctx context.Context, projectDir string, ev TaskEvent) error {
	s.attempts++
	if s.attempts <= s.failAfter {
		return errors.New("injected audit failure")
	}
	return s.FileStore.AppendAuditEvent(ctx, projectDir, ev)
}

func TestTaskRecorder_RecordDoneRetriesTransientAuditFailure(t *testing.T) {
	dir := t.TempDir()
	inner := NewFileStore(".reasonix/tasks")
	store := &flakyAuditStore{FileStore: inner}
	r := NewTaskRecorder(store, dir, func() string { return "sess-1" })
	ctx := context.Background()

	r.RecordStart("task-1", "task", "demo")
	// Arm the flake only for RecordDone's audit so the start event is intact.
	store.failAfter = store.attempts + 2
	r.RecordDone("task-1", jobs.Done, nil)

	monitorID := monitorTaskID("sess-1", "task-1")
	snap, err := inner.GetTask(ctx, dir, monitorID)
	if err != nil || snap == nil || snap.State != TaskStateSucceeded {
		t.Fatalf("snapshot must reach the terminal state despite audit flakiness: %+v, %v", snap, err)
	}
	events, err := inner.ListEvents(ctx, dir, monitorID, 0)
	if err != nil || len(events) != 2 {
		t.Fatalf("audit trail must be complete after retries: %d events, %v", len(events), err)
	}
	if store.attempts != store.failAfter+1 {
		t.Fatalf("audit attempts = %d, want %d (2 failures + 1 success for RecordDone)", store.attempts, store.failAfter+1)
	}
	if events[len(events)-1].State != TaskStateSucceeded {
		t.Fatalf("terminal audit event state = %q, want succeeded", events[len(events)-1].State)
	}
}

func TestTaskRecorder_RecordDonePersistentAuditFailureStaysTerminal(t *testing.T) {
	dir := t.TempDir()
	inner := NewFileStore(".reasonix/tasks")
	store := &flakyAuditStore{FileStore: inner, failAfter: 100}
	r := NewTaskRecorder(store, dir, func() string { return "sess-1" })
	ctx := context.Background()

	r.RecordStart("task-1", "task", "demo")
	r.RecordDone("task-1", jobs.Done, nil) // must not panic or hang

	snap, err := inner.GetTask(ctx, dir, monitorTaskID("sess-1", "task-1"))
	if err != nil || snap == nil || snap.State != TaskStateSucceeded {
		t.Fatalf("snapshot must stay terminal even when the audit never lands: %+v, %v", snap, err)
	}
	if store.attempts < 3 {
		t.Fatalf("audit attempts = %d, want at least the bounded 3 retries for RecordDone", store.attempts)
	}
}
