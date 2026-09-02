package taskmonitor

import (
	"context"
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/jobs"
)

func TestTaskRecorder_RecordUsageWritesCostEventAndSnapshotFields(t *testing.T) {
	dir := t.TempDir()
	r, store := newRecorderForTest(t, dir)
	ctx := context.Background()
	r.RecordStart("task-1", "task", "demo")
	monitorID := monitorTaskID("sess-1", "task-1")

	quote := &billing.CostQuote{CostComplete: true, Selected: &billing.Money{Amount: "0.42", Currency: "$"}}
	r.RecordUsage("task-1", quote, 7)

	snap, err := store.GetTask(ctx, dir, monitorID)
	if err != nil || snap == nil {
		t.Fatalf("GetTask after usage: %+v, %v", snap, err)
	}
	if snap.StepsUsed != 7 {
		t.Fatalf("StepsUsed = %d, want 7", snap.StepsUsed)
	}
	if snap.CostTotal != "$ 0.42" || snap.CostStatus != "ok" {
		t.Fatalf("cost fields = %q/%q, want $ 0.42/ok", snap.CostTotal, snap.CostStatus)
	}
	events, err := store.ListEvents(ctx, dir, monitorID, 0)
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %+v, %v", events, err)
	}
	cost := events[len(events)-1]
	if cost.EventType != "cost" || cost.State != TaskStateRunning {
		t.Fatalf("cost event = %+v", cost)
	}
	if cost.Detail != "$ 0.42 over 7 steps" {
		t.Fatalf("cost detail = %q", cost.Detail)
	}
}

func TestTaskRecorder_RecordUsageDegradesWithoutPricing(t *testing.T) {
	dir := t.TempDir()
	r, store := newRecorderForTest(t, dir)
	ctx := context.Background()
	r.RecordStart("task-1", "task", "demo")
	monitorID := monitorTaskID("sess-1", "task-1")

	r.RecordUsage("task-1", &billing.CostQuote{CostComplete: false}, 3)

	snap, err := store.GetTask(ctx, dir, monitorID)
	if err != nil || snap == nil {
		t.Fatalf("GetTask after usage: %+v, %v", snap, err)
	}
	if snap.StepsUsed != 3 {
		t.Fatalf("StepsUsed = %d, want 3", snap.StepsUsed)
	}
	if snap.CostTotal != "" || snap.CostStatus != "unavailable" {
		t.Fatalf("cost fields = %q/%q, want unavailable without a total", snap.CostTotal, snap.CostStatus)
	}
	events, _ := store.ListEvents(ctx, dir, monitorID, 0)
	cost := events[len(events)-1]
	if cost.Detail != "pricing unavailable over 3 steps" {
		t.Fatalf("cost detail = %q", cost.Detail)
	}
}

func TestTaskRecorder_RecordUsageUnknownJobIsNoop(t *testing.T) {
	dir := t.TempDir()
	r, store := newRecorderForTest(t, dir)
	ctx := context.Background()
	r.RecordUsage("never-started", &billing.CostQuote{CostComplete: true, Selected: &billing.Money{Amount: "1", Currency: "$"}}, 1)
	if tasks, err := store.ListTasks(ctx, dir); err != nil || len(tasks) != 0 {
		t.Fatalf("usage for an unknown job must not create tasks: %+v, %v", tasks, err)
	}
}

func TestTaskRecorder_RecordUsageSurvivesConcurrentCompletion(t *testing.T) {
	dir := t.TempDir()
	r, store := newRecorderForTest(t, dir)
	r.RecordStart("task-1", "task", "demo")
	monitorID := monitorTaskID("sess-1", "task-1")
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		r.RecordDone("task-1", jobs.Done, nil)
		close(done)
	}()
	quote := &billing.CostQuote{CostComplete: true, Selected: &billing.Money{Amount: "0.12", Currency: "$"}}
	r.RecordUsage("task-1", quote, 5)
	<-done

	snap, err := store.GetTask(ctx, dir, monitorID)
	if err != nil || snap == nil {
		t.Fatalf("GetTask after concurrent usage+done: %+v, %v", snap, err)
	}
	if snap.StepsUsed != 5 {
		t.Fatalf("StepsUsed = %d, want 5 despite the completion race", snap.StepsUsed)
	}
	if snap.State != TaskStateSucceeded {
		t.Fatalf("state = %q, completion must win the terminal state", snap.State)
	}
}
