package main

import "time"

// What a scheduled task is, and the one place a tick records why it left it alone.

// HeartbeatTask defines a single scheduled prompt.
type HeartbeatTask struct {
	ID            string `json:"id"`
	Title         string `json:"title"`          // user-visible label
	Prompt        string `json:"prompt"`         // the prompt to submit
	Goal          string `json:"goal,omitempty"` // unattended Goal contract; empty = a plain scheduled prompt
	Interval      string `json:"interval"`       // e.g. "5m", "1h", "30s"
	Enabled       bool   `json:"enabled"`
	Scope         string `json:"scope,omitempty"`         // "global" or "project"
	WorkspaceRoot string `json:"workspaceRoot,omitempty"` // project root path when scope="project"
	TopicID       string `json:"topicId,omitempty"`       // created topic, reused on re-run
	LastRunAt     int64  `json:"lastRunAt,omitempty"`     // unix millis, moved only by a real run
	LastAttemptAt int64  `json:"lastAttemptAt,omitempty"` // unix millis, a tick spent without running (Goal hold / no topic)
	// Why the most recent tick did not run this task, and when that tick was. The panel shows
	// that a task is overdue; the reason is what separates a deliberate hold from a failure
	// that will keep repeating (2026-10-04). Cleared by a run that actually submits.
	LastHold               string         `json:"lastHold,omitempty"`
	LastHoldAt             int64          `json:"lastHoldAt,omitempty"`
	NewConversationEachRun bool           `json:"newConversationEachRun,omitempty"` // true = create new topic every run
	RunHistory             []HeartbeatRun `json:"runHistory,omitempty"`             // recent executions (oldest first, capped)
	CreatedAt              int64          `json:"createdAt,omitempty"`
	ApprovalMode           string         `json:"approvalMode"`              // "ask" | "auto" | "yolo"; empty defaults to "yolo"
	TimeWindowStart        string         `json:"timeWindowStart,omitempty"` // "HH:MM" — interval tasks only run after this time (inclusive)
	TimeWindowEnd          string         `json:"timeWindowEnd,omitempty"`   // "HH:MM" — interval tasks only run before this time (exclusive)
	NotifyChannels         *bool          `json:"notifyChannels,omitempty"`  // true = push to bot channels; nil/false = skip
}

// HeartbeatRun records a single successful execution of a heartbeat task.
// TopicID is the conversation created/reused by that run (may be empty if
// the run produced no topic).
type HeartbeatRun struct {
	At      int64  `json:"at"`      // unix millis execution time
	TopicID string `json:"topicId"` // topic used/created by this run
}

// noteHold records why a tick left the task alone. A deliberate hold (a running Goal, a spent
// window, a pending upgrade) and a failure that will repeat look identical in the schedule;
// the reason is what tells them apart in the panel (2026-10-04).
func noteHold(t *HeartbeatTask, reason string, now time.Time) {
	t.LastHold = reason
	t.LastHoldAt = now.UnixMilli()
	t.LastAttemptAt = now.UnixMilli()
}
