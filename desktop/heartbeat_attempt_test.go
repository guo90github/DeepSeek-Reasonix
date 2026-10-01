package main

import (
	"testing"
	"time"
)

// A tick the driver spent without running (a Goal hold) must not look like a
// run: the UI reads LastRunAt as "the last run".
func TestHeartbeatHoldMarksAnAttemptNotARun(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	task := HeartbeatTask{ID: "t", Title: "task", Goal: "ship"}
	if !engine.holdTick(&task, "a running Goal owns the next turn") {
		t.Fatal("holdTick must consume the tick")
	}
	if task.LastRunAt != 0 {
		t.Fatalf("LastRunAt = %d, want a hold to leave it alone", task.LastRunAt)
	}
	if task.LastAttemptAt == 0 {
		t.Fatal("a held tick must record the attempt, or it repeats every scheduler tick")
	}
}

// The schedule counts from a consumed attempt, so a hold still postpones the next
// attempt by one interval instead of hammering the session every 30 seconds.
func TestHeartbeatAttemptConsumesTheInterval(t *testing.T) {
	now := time.Now()
	created := now.Add(-time.Hour).UnixMilli()
	attempted := HeartbeatTask{Interval: "10m", CreatedAt: created, LastAttemptAt: now.UnixMilli()}
	if heartbeatTaskDueAt(attempted, now.Add(time.Minute)) {
		t.Fatal("a task whose tick was just consumed must not be due a minute later")
	}
	if !heartbeatTaskDueAt(attempted, now.Add(11*time.Minute)) {
		t.Fatal("a task is due again one interval after its last attempt")
	}

	ran := HeartbeatTask{Interval: "10m", CreatedAt: created, LastRunAt: now.UnixMilli()}
	if heartbeatTaskDueAt(ran, now.Add(time.Minute)) {
		t.Fatal("a task that just ran must not be due")
	}
	if !heartbeatTaskDueAt(ran, now.Add(11*time.Minute)) {
		t.Fatal("a task is due again one interval after its last run")
	}
}

// A busy skip returns without consuming the tick, so the task stays due: that is
// what lets the task list say it is overdue instead of pretending nothing is due.
func TestHeartbeatSkippedTaskStaysDue(t *testing.T) {
	now := time.Now()
	task := HeartbeatTask{
		Interval:  "10m",
		CreatedAt: now.Add(-time.Hour).UnixMilli(),
		LastRunAt: now.Add(-40 * time.Minute).UnixMilli(),
	}
	if !heartbeatTaskDueAt(task, now) {
		t.Fatal("a task whose last run is older than its interval must stay due while it is skipped")
	}
}
