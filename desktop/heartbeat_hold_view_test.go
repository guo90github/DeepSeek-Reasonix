package main

import "testing"

// A task that is due but did not run shows as overdue, and that is all the schedule can say.
// The reason belongs on the task, where the panel reads it: a deliberate hold (a running Goal,
// a spent window, a pending upgrade) and a failure that will repeat look identical otherwise
// (2026-10-04).
func TestAHoldRecordsItsReasonOnTheTaskWithoutLookingLikeARun(t *testing.T) {
	e := &HeartbeatEngine{}
	task := &HeartbeatTask{ID: "a", Title: "a"}
	if !e.holdTick(task, "a running Goal drives this session's own turns") {
		t.Fatal("holdTick reports a hold as taken")
	}
	if task.LastHold == "" {
		t.Fatal("the hold left no reason on the task, so the panel can only say it is overdue")
	}
	if task.LastHoldAt == 0 || task.LastHoldAt != task.LastAttemptAt {
		t.Fatalf("hold at %d / attempt at %d, want the same non-zero instant", task.LastHoldAt, task.LastAttemptAt)
	}
	if task.LastRunAt != 0 {
		t.Fatal("a hold must not look like a run: the schedule counts from it, LastRunAt does not")
	}
}
