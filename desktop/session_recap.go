package main

import (
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/recap"
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

// SessionRecapView is one stored session recap as the read-only page consumes it.
type SessionRecapView struct {
	Path        string              `json:"path"`
	Entries     []SessionRecapEntry `json:"entries"`
	Model       string              `json:"model"`
	GeneratedAt string              `json:"generatedAt"`
}

// ListSessionRecaps returns the recaps the 会话回顾 page shows, newest first.
// Sessions that left the visible set (moved to trash, purged) are dropped here
// rather than in the UI, so the page lists exactly what retrieval still sees.
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
	out := make([]SessionRecapView, 0, len(records))
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
		out = append(out, SessionRecapView{
			Path:        rec.Path,
			Entries:     entries,
			Model:       rec.Model,
			GeneratedAt: rec.GeneratedAt.Format(time.RFC3339),
		})
	}
	return out
}
