package control

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/memory"
)

// The turn ordinal must advance without hooks. It used to move only inside the hook
// block, so a hook-free run left every turn-keyed record at 0 and each sidecar kept
// one entry — "the last recall of the session" — instead of one per turn. The old
// skill-use test could not catch that: it compared the record against the same
// counter, so 0 == 0 passed.
func TestTurnOrdinalAdvancesWithoutHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "turns.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})
	if got := c.Turn(); got != 0 {
		t.Fatalf("turn = %d before any submit, want 0", got)
	}

	c.nextTurn()
	c.RecordSkillUse("hot", "abc123")
	c.nextTurn()
	c.RecordSkillUse("hot", "abc123")

	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	if len(meta.SkillUse) != 2 {
		t.Fatalf("skill records = %+v, want one per turn, not one per skill name", meta.SkillUse)
	}
	turns := map[int]bool{}
	for _, use := range meta.SkillUse {
		if use.TurnSeq < 1 {
			t.Fatalf("skill record = %+v, want a real turn number", use)
		}
		turns[use.TurnSeq] = true
	}
	if len(turns) != 2 {
		t.Fatalf("turns = %v, want the two turns kept apart", turns)
	}

	// The recall record travels on the same ordinal: two turns, two records.
	c.nextTurn()
	c.recordMemoryRecallTurn(memory.RecallResult{
		TurnSeq: c.Turn(),
		Hits:    []memory.RecallHit{{Memory: memory.Memory{ID: "mem-a", Revision: 1}}},
	})
	c.nextTurn()
	c.recordMemoryRecallTurn(memory.RecallResult{
		TurnSeq: c.Turn(),
		Hits:    []memory.RecallHit{{Memory: memory.Memory{ID: "mem-b", Revision: 1}}},
	})
	recalled, _, err := agent.LoadBranchMeta(path)
	if err != nil {
		t.Fatalf("LoadBranchMeta: %v", err)
	}
	if len(recalled.MemoryRecall) != 2 {
		t.Fatalf("recall records = %d, want one per turn", len(recalled.MemoryRecall))
	}
	for _, turn := range recalled.MemoryRecall {
		if turn.TurnSeq < 1 {
			t.Fatalf("recall record = %+v, want a real turn number", turn)
		}
	}
}
