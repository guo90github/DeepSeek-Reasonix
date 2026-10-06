package main

import (
	"strings"

	"reasonix/internal/agent"
)

// RecallRecordView is one session's recorded fingerprints: what each turn asked
// memory for, which facts reached the model, and which skill it ran (docs/50 §2.2).
// It carries each hit's id, counters, digests and short label — never a fact's
// body — so the review page can render it without a live controller.
type RecallRecordView struct {
	Available    bool             `json:"available"`
	SessionPath  string           `json:"sessionPath,omitempty"`
	DroppedTurns int              `json:"droppedTurns,omitempty"`
	Turns        []RecallTurnView `json:"turns,omitempty"`
	Skills       []SkillUseView   `json:"skills,omitempty"`
}

// RecallTurnView is one turn's recall decision.
type RecallTurnView struct {
	TurnSeq int `json:"turnSeq"`
	// Source separates the automatic turn-tail injection ("") from a retrieval the
	// model asked for through the memory tool.
	Source    string `json:"source,omitempty"`
	QueryHash string `json:"queryHash,omitempty"`
	// SnapshotDigest is the session-context fingerprint the turn ran against.
	SnapshotDigest string `json:"snapshotDigest,omitempty"`
	// QueryExcerpt is the ask this turn recalled for, one line at most.
	QueryExcerpt string          `json:"queryExcerpt,omitempty"`
	UsedChars    int             `json:"usedChars,omitempty"`
	Omitted      int             `json:"omitted,omitempty"`
	Suppressed   string          `json:"suppressed,omitempty"`
	Hits         []RecallHitView `json:"hits,omitempty"`
}

// RecallHitView is one fact's fingerprint in a turn; Injected separates the facts
// the model saw from the ones that were dropped, and Name/Title are the fact's
// short label so a reader can tell the ids apart. Records written before the label
// existed carry neither, so the page falls back to the id.
type RecallHitView struct {
	ID          string  `json:"id"`
	Name        string  `json:"name,omitempty"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Reason      string  `json:"reason,omitempty"`
	Scope       string  `json:"scope,omitempty"`
	Type        string  `json:"type,omitempty"`
	Freshness   string  `json:"freshness,omitempty"`
	Revision    int     `json:"revision,omitempty"`
	Score       float64 `json:"score,omitempty"`
	Injected    *bool   `json:"injected,omitempty"`
}

// SkillUseView is one skill invocation's fingerprint.
type SkillUseView struct {
	TurnSeq       int    `json:"turnSeq"`
	Name          string `json:"name"`
	ContentHash   string `json:"contentHash,omitempty"`
	CatalogDigest string `json:"catalogDigest,omitempty"`
}

// RecallRecordForTab returns the recall and skill-use records a tab's session has
// written to its sidecar. An empty tabID means the active tab; an unknown tab or a
// session with no records returns an empty view rather than another tab's data.
func (a *App) RecallRecordForTab(tabID string) RecallRecordView {
	tab := a.tabByID(tabID)
	if tab == nil {
		if tabID != "" {
			return RecallRecordView{}
		}
		tab = a.activeTab()
	}
	if tab == nil {
		return RecallRecordView{}
	}
	path := tab.currentSessionPath()
	if path == "" {
		return RecallRecordView{}
	}
	return a.recallRecordForSessionPath(path)
}

// RecallRecordForSession returns the same view for a session the caller names by
// transcript path. The recap page lists sessions that are not open tabs, so it
// cannot address them by tab id.
func (a *App) RecallRecordForSession(sessionPath string) RecallRecordView {
	return a.recallRecordForSessionPath(sessionPath)
}

func (a *App) recallRecordForSessionPath(path string) RecallRecordView {
	if strings.TrimSpace(path) == "" {
		return RecallRecordView{}
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		return RecallRecordView{}
	}
	return recallRecordView(meta, path)
}

// recallRecordView converts one session's sidecar records for the review page.
func recallRecordView(meta agent.BranchMeta, path string) RecallRecordView {
	view := RecallRecordView{SessionPath: path, DroppedTurns: meta.MemoryRecallDropped}
	for _, turn := range meta.MemoryRecall {
		out := RecallTurnView{
			TurnSeq: turn.TurnSeq, QueryHash: turn.QueryHash, QueryExcerpt: turn.QueryExcerpt,
			UsedChars: turn.UsedChars,
			Omitted:   turn.Omitted, Suppressed: turn.Suppressed, SnapshotDigest: turn.SnapshotDigest,
			Source: turn.Source,
		}
		for _, hit := range turn.Hits {
			out.Hits = append(out.Hits, RecallHitView{
				ID: hit.ID, Name: hit.Name, Title: hit.Title, Description: hit.Description, Reason: hit.Reason,
				Scope: hit.Scope, Type: hit.Type, Freshness: hit.Freshness,
				Revision: hit.Revision, Score: hit.Score, Injected: hit.Injected,
			})
		}
		view.Turns = append(view.Turns, out)
	}
	for _, use := range meta.SkillUse {
		view.Skills = append(view.Skills, SkillUseView{
			TurnSeq: use.TurnSeq, Name: use.Name,
			ContentHash: use.ContentHash, CatalogDigest: use.CatalogDigest,
		})
	}
	view.Available = len(view.Turns) > 0 || len(view.Skills) > 0
	return view
}
