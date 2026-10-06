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

// docs/50 §2.2 全局级: a query that keeps being asked is the other half of the
// record — the same question recalling the same fact and never injecting it says
// the fact's own words do not match how the question is asked.
func TestRecallUsageReportsRepeatedQueries(t *testing.T) {
	dir := t.TempDir()
	newer := filepath.Join(dir, "newer.jsonl")
	older := filepath.Join(dir, "older.jsonl")
	once := filepath.Join(dir, "once.jsonl")
	writeTurn := func(path, hash string, turnSeq int, injected bool) {
		t.Helper()
		if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
			agent.AppendMemoryRecallTurn(meta, agent.MemoryRecallTurn{
				TurnSeq: turnSeq, QueryHash: hash,
				Hits: []agent.MemoryRecallTurnHit{{ID: "mem-a", Revision: 1, Injected: recallInjectedPtr(injected)}},
			})
			return nil
		}); err != nil {
			t.Fatalf("UpdateBranchMeta %s: %v", path, err)
		}
	}
	writeTurn(older, "aaaa1111", 4, false)
	writeTurn(newer, "aaaa1111", 2, false)
	writeTurn(newer, "aaaa1111", 9, false) // the same session asks again later
	writeTurn(once, "bbbb2222", 5, true)

	view := recallUsageForSessions([]SessionMeta{{Path: newer}, {Path: older}, {Path: once}}, nil)
	if len(view.Queries) != 1 {
		t.Fatalf("queries = %+v, want only the repeated query reported", view.Queries)
	}
	query := view.Queries[0]
	if query.Hash != "aaaa1111" || query.Turns != 3 || query.Sessions != 2 {
		t.Fatalf("query = %+v, want three turns across two sessions", query)
	}
	if query.Dropped != 3 || query.Injected != 0 {
		t.Fatalf("query = %+v, want every hit dropped", query)
	}
	if query.LastTurnSeq != 9 || query.LastSession != newer {
		t.Fatalf("query = %+v, want the latest turn of the newest session", query)
	}
	if len(query.Facts) != 1 || query.Facts[0] != "mem-a" {
		t.Fatalf("query = %+v, want the fact it kept matching", query)
	}
}
