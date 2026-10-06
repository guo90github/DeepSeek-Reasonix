package main

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/memory"
)

// docs/50 §2.2 全局级: across sessions, which fact was used how often, when it was
// last used, and whether the revision it used is still current.
func TestRecallUsageFoldsSessionsAndFlagsSuperseded(t *testing.T) {
	dir := t.TempDir()
	newest := filepath.Join(dir, "newest.jsonl")
	older := filepath.Join(dir, "older.jsonl")

	write := func(path string, turn agent.MemoryRecallTurn) {
		t.Helper()
		if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
			agent.AppendMemoryRecallTurn(meta, turn)
			return nil
		}); err != nil {
			t.Fatalf("UpdateBranchMeta %s: %v", path, err)
		}
	}
	write(newest, agent.MemoryRecallTurn{TurnSeq: 7, Hits: []agent.MemoryRecallTurnHit{
		{ID: "mem-a", Revision: 1, Injected: recallInjectedPtr(true)},
		{ID: "mem-b", Revision: 1, Injected: recallInjectedPtr(true)},
	}})
	write(older, agent.MemoryRecallTurn{TurnSeq: 4, Hits: []agent.MemoryRecallTurnHit{
		{ID: "mem-a", Revision: 1, Injected: recallInjectedPtr(false)},
		{ID: "mem-c", Revision: 1},
	}})

	// Sessions arrive newest-first; mem-a is now at revision 3 in the store.
	view := recallUsageForSessions(
		[]SessionMeta{{Path: newest}, {Path: older}},
		[]memory.Memory{{ID: "mem-a", Revision: 3, Name: "a-name", Description: "a-desc"}},
	)
	if !view.Available || view.Sessions != 2 {
		t.Fatalf("view = %+v, want two scanned sessions", view)
	}
	if len(view.Facts) != 3 {
		t.Fatalf("facts = %+v, want one row per recorded fact", view.Facts)
	}
	top := view.Facts[0]
	if top.ID != "mem-a" || top.Uses != 2 || top.Injected != 1 || top.Dropped != 1 {
		t.Fatalf("top fact = %+v, want mem-a used twice, injected once and dropped once", top)
	}
	if top.LastSession != newest || top.LastTurnSeq != 7 {
		t.Fatalf("top fact = %+v, want the newest session's use reported", top)
	}
	if !top.Live || top.CurrentRevision != 3 || !top.Superseded {
		t.Fatalf("top fact = %+v, want a live fact whose used revision 1 is superseded by 3", top)
	}
	if top.Name != "a-name" || top.Description != "a-desc" {
		t.Fatalf("top fact = %+v, want the store's current label", top)
	}
	for _, fact := range view.Facts[1:] {
		if fact.Live {
			t.Fatalf("fact = %+v, want facts absent from the store marked not live", fact)
		}
		if fact.ID == "mem-c" && fact.Unrecorded != 1 {
			t.Fatalf("fact = %+v, want an absent decision counted as unrecorded", fact)
		}
	}
}

func TestRecallUsageWithoutRecordsIsUnavailable(t *testing.T) {
	if view := recallUsageForSessions(nil, nil); view.Available || len(view.Facts) != 0 {
		t.Fatalf("view = %+v, want an unavailable aggregate", view)
	}
	if view := recallUsageForSessions([]SessionMeta{{Path: filepath.Join(t.TempDir(), "missing.jsonl")}}, nil); view.Available {
		t.Fatalf("view = %+v, want a session without records to contribute nothing", view)
	}
}
