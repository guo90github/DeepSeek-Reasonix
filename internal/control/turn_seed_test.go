package control

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

// Two record-identity guarantees the review surfaces depend on: a record says who
// decided the hand-over, and a restarted process must not re-issue a turn number a
// previous run already wrote (the write keys on turn_seq and source).
func TestRecallRecordStampsSourceAndResumesTurnNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})

	c.recordSuppressedRecall("automatic recall is off; retrieve on demand")
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta(%q) = %v, %v; want a sidecar", path, ok, err)
	}
	if len(meta.MemoryRecall) != 1 {
		t.Fatalf("records = %d, want 1", len(meta.MemoryRecall))
	}
	rec := meta.MemoryRecall[0]
	if rec.Source != agent.MemoryRecallSourceAuto {
		t.Fatalf("source = %q, want %q", rec.Source, agent.MemoryRecallSourceAuto)
	}
	if rec.QueryExcerpt != "" || rec.QueryHash != "" {
		t.Fatalf("a suppressed turn must not claim a query: excerpt=%q hash=%q", rec.QueryExcerpt, rec.QueryHash)
	}
	if rec.Suppressed != "automatic recall is off; retrieve on demand" {
		t.Fatalf("suppressed = %q, want the reason", rec.Suppressed)
	}

	if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
		agent.AppendMemoryRecallTurn(meta, agent.MemoryRecallTurn{TurnSeq: 7, Source: agent.MemoryRecallSourceAuto})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := lastRecordedTurnSeq(filepath.Join(dir, "missing.jsonl")); got != 0 {
		t.Fatalf("lastRecordedTurnSeq on a session with no sidecar = %d, want 0", got)
	}
	if got := lastRecordedTurnSeq(path); got != 7 {
		t.Fatalf("lastRecordedTurnSeq = %d, want 7", got)
	}
	if got := c.nextTurn(); got != 8 {
		t.Fatalf("nextTurn after resume = %d, want 8: a restart must not re-issue 7", got)
	}
	if got := c.nextTurn(); got != 9 {
		t.Fatalf("nextTurn twice = %d, want 9", got)
	}
}
