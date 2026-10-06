package main

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
)

// B6 (docs/50 §2.2 / 第十四): the review page reads one session's fingerprints.
// The view must carry the decisions without carrying any memory or skill text.
func TestRecallRecordViewCarriesFingerprintsOnly(t *testing.T) {
	meta := agent.BranchMeta{
		MemoryRecall: []agent.MemoryRecallTurn{{
			TurnSeq: 4, QueryHash: "9f9f9f9f9f9f9f9f", UsedChars: 210, Omitted: 1, SnapshotDigest: "sha256:abc",
			Hits: []agent.MemoryRecallTurnHit{
				{ID: "mem-a", Revision: 2, Score: 0.9, Injected: recallInjectedPtr(true), Scope: "project", Type: "project", Freshness: "stale", Description: "release target"},
				{ID: "mem-b", Revision: 1, Score: 0.4, Injected: recallInjectedPtr(false)},
				{ID: "mem-c", Revision: 1, Score: 0.2},
			},
		}},
		MemoryRecallDropped: 7,
		SkillUse: []agent.SkillUseRecord{{
			TurnSeq: 4, Name: "hot", ContentHash: "abcdef0123456789", CatalogDigest: "0123456789abcdef",
		}},
	}

	view := recallRecordView(meta, "/tmp/session.jsonl")
	if !view.Available || view.SessionPath != "/tmp/session.jsonl" {
		t.Fatalf("view = %+v, want an available record with its path", view)
	}
	if len(view.Turns) != 1 || view.Turns[0].TurnSeq != 4 || view.Turns[0].UsedChars != 210 || view.Turns[0].Omitted != 1 {
		t.Fatalf("turns = %+v, want the turn decision", view.Turns)
	}
	if len(view.Turns[0].Hits) != 3 {
		t.Fatalf("hits = %+v, want served, dropped and unrecorded fingerprints", view.Turns[0].Hits)
	}
	hits := view.Turns[0].Hits
	if hits[0].Injected == nil || !*hits[0].Injected {
		t.Fatalf("hits = %+v, want the served fact marked injected", hits)
	}
	if hits[1].Injected == nil || *hits[1].Injected {
		t.Fatalf("hits = %+v, want the dropped fact marked not injected", hits)
	}
	if hits[2].Injected != nil {
		t.Fatalf("hits = %+v, want an unrecorded decision to stay unknown", hits)
	}
	if view.DroppedTurns != 7 {
		t.Fatalf("droppedTurns = %d, want the trimmed turns reported", view.DroppedTurns)
	}
	if hits[0].Scope != "project" || hits[0].Type != "project" || hits[0].Freshness != "stale" {
		t.Fatalf("hits = %+v, want the fact's scope/type/freshness carried through", hits[0])
	}
	if hits[0].Description != "release target" {
		t.Fatalf("hits = %+v, want the fact's description carried through", hits[0])
	}
	if view.Turns[0].SnapshotDigest != "sha256:abc" {
		t.Fatalf("snapshotDigest = %q, want the turn's context fingerprint", view.Turns[0].SnapshotDigest)
	}
	if len(view.Skills) != 1 || view.Skills[0].Name != "hot" || view.Skills[0].ContentHash == "" {
		t.Fatalf("skills = %+v, want the invocation fingerprint", view.Skills)
	}
	for _, probe := range []string{"description", "body", "Release target"} {
		if strings.Contains(rendered(view), probe) {
			t.Fatalf("view leaks text: %q", probe)
		}
	}
}

func rendered(view RecallRecordView) string {
	var b strings.Builder
	for _, turn := range view.Turns {
		b.WriteString(turn.Suppressed)
		for _, hit := range turn.Hits {
			b.WriteString(hit.ID)
		}
	}
	for _, use := range view.Skills {
		b.WriteString(use.Name + use.ContentHash + use.CatalogDigest)
	}
	return b.String()
}

// An empty session reports itself as unavailable rather than looking available.
func TestRecallRecordViewWithoutRecordsIsUnavailable(t *testing.T) {
	if view := recallRecordView(agent.BranchMeta{}, "/tmp/empty.jsonl"); view.Available || len(view.Turns) != 0 || len(view.Skills) != 0 {
		t.Fatalf("view = %+v, want an unavailable record", view)
	}
	if view := (NewApp()).RecallRecordForTab("missing-tab"); view.Available {
		t.Fatalf("an unknown tab must not borrow another session's record: %+v", view)
	}
}

// The recap page addresses sessions by transcript path, not by tab id, so the
// path entry point must read the sidecar the writer produced.
func TestRecallRecordForSessionReadsTheSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recall-view.jsonl")
	if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
		agent.AppendMemoryRecallTurn(meta, agent.MemoryRecallTurn{
			TurnSeq: 6, Hits: []agent.MemoryRecallTurnHit{{ID: "mem-z", Revision: 1, Score: 0.5, Injected: recallInjectedPtr(true)}},
		})
		agent.AppendSkillUse(meta, agent.SkillUseRecord{TurnSeq: 6, Name: "hot", ContentHash: "abc", CatalogDigest: "def"})
		return nil
	}); err != nil {
		t.Fatalf("UpdateBranchMeta: %v", err)
	}

	view := NewApp().RecallRecordForSession(path)
	if !view.Available || view.SessionPath != path {
		t.Fatalf("view = %+v, want the session's record", view)
	}
	if len(view.Turns) != 1 || view.Turns[0].TurnSeq != 6 || len(view.Turns[0].Hits) != 1 || view.Turns[0].Hits[0].ID != "mem-z" {
		t.Fatalf("turns = %+v, want the turn and its hit", view.Turns)
	}
	if len(view.Skills) != 1 || view.Skills[0].Name != "hot" {
		t.Fatalf("skills = %+v, want the invocation", view.Skills)
	}
	if empty := NewApp().RecallRecordForSession("   "); empty.Available {
		t.Fatalf("a blank path must not resolve: %+v", empty)
	}
}

func recallInjectedPtr(v bool) *bool { return &v }
