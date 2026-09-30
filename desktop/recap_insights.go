package main

import (
	"time"

	"reasonix/internal/recap"
	"reasonix/internal/secrets"
)

// SessionRecapInsight is one conclusion more than one project reached on its own.
// It is what turns the projection from "notes about this project" into "what keeps
// being true across projects" — and every line names the projects, so the claim is
// checkable instead of asserted.
type SessionRecapInsight struct {
	Kind     string   `json:"kind"`
	Body     string   `json:"body"`
	Evidence string   `json:"evidence,omitempty"`
	Projects []string `json:"projects"`
	// Occurrences counts how many records of one project reached it: the second way
	// a conclusion earns a report, for work that keeps coming back to one codebase.
	Occurrences int    `json:"occurrences"`
	SeenAt      string `json:"seenAt"`
}

// insightWindow is how far back the report looks: a period, not a dump of
// everything ever recorded.
const insightWindow = 30 * 24 * time.Hour

// insightLimit caps one report; the projection is small, but a report nobody can
// read through is not a report.
const insightLimit = 20

// ListRecapInsights reports the conclusions at least two projects reached
// independently in the last 30 days. Read-only and model-free: it is a read of
// the projection, computed the same way the tier proposal's evidence is.
func (a *App) ListRecapInsights() []SessionRecapInsight {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return []SessionRecapInsight{}
	}
	defer func() { _ = store.Close() }()
	insights, err := store.Insights(ctx, time.Now().Add(-insightWindow), insightLimit)
	if err != nil {
		return []SessionRecapInsight{}
	}
	out := make([]SessionRecapInsight, 0, len(insights))
	for _, insight := range insights {
		out = append(out, SessionRecapInsight{
			Kind:        insight.Kind,
			Body:        secrets.Redact(insight.Body),
			Evidence:    secrets.Redact(insight.Evidence),
			Projects:    append([]string(nil), insight.Projects...),
			Occurrences: insight.Occurrences,
			SeenAt:      insight.SeenAt.Format(time.RFC3339),
		})
	}
	return out
}
