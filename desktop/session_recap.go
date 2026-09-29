package main

import (
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/recap"
)

// SessionRecapView is one stored session recap as the read-only page consumes it.
type SessionRecapView struct {
	Path        string `json:"path"`
	Goal        string `json:"goal"`
	Actions     string `json:"actions"`
	Conclusion  string `json:"conclusion"`
	Todos       string `json:"todos,omitempty"`
	Model       string `json:"model"`
	GeneratedAt string `json:"generatedAt"`
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
	out := make([]SessionRecapView, 0, len(records))
	for _, rec := range records {
		if !agent.IsVisibleSession(rec.Path) {
			continue
		}
		out = append(out, SessionRecapView{
			Path:        rec.Path,
			Goal:        rec.Goal,
			Actions:     rec.Actions,
			Conclusion:  rec.Conclusion,
			Todos:       rec.FollowUps,
			Model:       rec.Model,
			GeneratedAt: rec.GeneratedAt.Format(time.RFC3339),
		})
	}
	return out
}
