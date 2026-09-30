package main

import (
	"sort"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/recap"
	"reasonix/internal/secrets"
)

// SessionRecapEntry is one distilled note as the recap page consumes it. ID names
// the note for a review action, Target says where an accepted note lands, and
// Decision carries the choice already made ("" when the note is untouched).
type SessionRecapEntry struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Body     string `json:"body"`
	Evidence string `json:"evidence,omitempty"`
	Target   string `json:"target"`
	Decision string `json:"decision,omitempty"`
}

// SessionRecapPending is a failed attempt, as the page reports it. A failure
// stores no record, so this is the only sign a person gets that the recap they
// expect was attempted and refused.
type SessionRecapPending struct {
	Attempts  int    `json:"attempts"`
	Reason    string `json:"reason"`
	UpdatedAt string `json:"updatedAt"`
}

// What a listed row actually is. The host decides it, because only the host can
// tell "no reusable notes" from "the attempt failed" — a page that guesses shows
// the wrong one of the two.
const (
	RecapStateStored  = "stored"
	RecapStatePending = "pending"
	RecapStateEmpty   = "empty"
)

// SessionRecapView is one session's recap as the read-only page consumes it.
type SessionRecapView struct {
	Path        string               `json:"path"`
	State       string               `json:"state"`
	Entries     []SessionRecapEntry  `json:"entries"`
	Model       string               `json:"model"`
	GeneratedAt string               `json:"generatedAt"`
	Pending     *SessionRecapPending `json:"pending,omitempty"`
}

// ListSessionRecaps returns the recaps the 会话回顾 page shows, newest first.
// Sessions that left the visible set (moved to trash, purged) are dropped here
// rather than in the UI, so the page lists exactly what retrieval still sees.
// Failed attempts are listed too: they have no record to be found by.
func (a *App) ListSessionRecaps() []SessionRecapView {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return []SessionRecapView{}
	}
	defer func() { _ = store.Close() }()
	records, err := store.List(ctx)
	if err != nil {
		return []SessionRecapView{}
	}
	decisions, err := store.Decisions(ctx)
	if err != nil {
		decisions = map[string]recap.Decision{}
	}
	failures, err := store.PendingMap(ctx)
	if err != nil {
		failures = map[string]recap.Pending{}
	}
	out := make([]SessionRecapView, 0, len(records))
	listed := make(map[string]bool, len(records))
	for _, rec := range records {
		if !agent.IsVisibleSession(rec.Path) {
			continue
		}
		entries := make([]SessionRecapEntry, 0, len(rec.Entries))
		for _, entry := range rec.Entries {
			id := recap.HashEntry(entry.Kind, entry.Body)
			entries = append(entries, SessionRecapEntry{
				ID:       id,
				Kind:     entry.Kind,
				Body:     entry.Body,
				Evidence: entry.Evidence,
				Target:   recap.Sink(entry.Kind),
				Decision: decisions[id].Choice,
			})
		}
		view := SessionRecapView{
			Path:        rec.Path,
			State:       RecapStateStored,
			Entries:     entries,
			Model:       rec.Model,
			GeneratedAt: rec.GeneratedAt.Format(time.RFC3339),
		}
		if len(entries) == 0 {
			view.State = RecapStateEmpty
		}
		if pending, failed := failures[rec.Path]; failed {
			view.State = RecapStatePending
			view.Pending = recapPendingView(pending)
		}
		listed[rec.Path] = true
		out = append(out, view)
	}
	failed := make([]SessionRecapView, 0, len(failures))
	for path, pending := range failures {
		if listed[path] || !agent.IsVisibleSession(path) {
			continue
		}
		failed = append(failed, SessionRecapView{
			Path:    path,
			State:   RecapStatePending,
			Entries: []SessionRecapEntry{},
			Pending: recapPendingView(pending),
		})
	}
	// Newest attempt first, so a failure the page just caused is at the top.
	sort.Slice(failed, func(i, j int) bool { return failed[i].Pending.UpdatedAt > failed[j].Pending.UpdatedAt })
	return append(out, failed...)
}

// recapPendingView reports a failure. The reason quotes the model's answer, so it
// is redacted here rather than trusted to have been redacted on the way in.
func recapPendingView(pending recap.Pending) *SessionRecapPending {
	return &SessionRecapPending{
		Attempts:  pending.Attempts,
		Reason:    secrets.Redact(pending.Reason),
		UpdatedAt: pending.UpdatedAt.Format(time.RFC3339),
	}
}
