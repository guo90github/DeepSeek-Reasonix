package recap

import (
	"context"
	"testing"
	"time"
)

// The decision log is what makes a missing recap diagnosable from the projection
// alone, so it has to survive a write/read round trip.
func TestTraceRecordsLaneDecisions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Options{InMemory: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()

	at := time.Unix(1_700_000_000, 0)
	if err := store.Trace(ctx, "generate", "/tmp/s.jsonl", "stored", at); err != nil {
		t.Fatalf("trace: %v", err)
	}
	entries, err := store.Activity(ctx, 5)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("activity has %d entries, want 1", len(entries))
	}
	if got := entries[0]; got.Stage != "generate" || got.Path != "/tmp/s.jsonl" || got.Detail != "stored" {
		t.Fatalf("activity entry = %+v", got)
	}
}

// A close that hands the lane an empty path is exactly the bug that produced no
// recaps silently, so the rejection must be recorded with its reason.
func TestSubmitRejectionIsTraced(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Options{InMemory: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()

	runner := NewRunner(NewGenerator(GeneratorOptions{Store: store}))
	defer runner.Stop()

	if runner.Submit("") {
		t.Fatal("an empty session path must be rejected")
	}
	entries, err := store.Activity(ctx, 5)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	if len(entries) != 1 || entries[0].Stage != "submit" || entries[0].Detail != "rejected: empty path" {
		t.Fatalf("activity = %+v", entries)
	}
}
