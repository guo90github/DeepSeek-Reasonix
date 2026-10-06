package control

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/memory"
)

// A tool-driven fetch is a second record for the same turn, not a replacement for the
// automatic one: they answer different questions ("what was pushed" vs "what was
// asked for") and the review page needs both.
func TestToolFetchLandsBesideTheAutomaticRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fetch.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})

	c.nextTurn()
	c.Compose("把支付部署到绿色集群")
	c.RecordMemoryFetch("green", []memory.Memory{{
		ID: "mem-green", Name: "green-cluster", Description: "支付部署到绿色集群",
	}})

	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	var automatic, toolDriven int
	for _, turn := range meta.MemoryRecall {
		switch turn.Source {
		case agent.MemoryRecallSourceAuto:
			automatic++
		case agent.MemoryRecallSourceTool:
			toolDriven++
			if len(turn.Hits) != 1 || turn.Hits[0].ID != "mem-green" {
				t.Fatalf("tool record = %+v, want the fetched fact on it", turn)
			}
			if turn.QueryExcerpt != "green" || turn.QueryHash == "" {
				t.Fatalf("tool record = %+v, want the ask recorded content-free", turn)
			}
		default:
			t.Fatalf("unexpected source %q", turn.Source)
		}
	}
	if automatic != 1 || toolDriven != 1 {
		t.Fatalf("records = %+v, want one automatic and one tool record", meta.MemoryRecall)
	}
}

// A fetch with no session or nothing behind it must not write a record.
func TestToolFetchWithoutHitsWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})

	c.nextTurn()
	c.RecordMemoryFetch("nothing matched", nil)

	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if ok && len(meta.MemoryRecall) != 0 {
		t.Fatalf("records = %+v, want an empty fetch left unrecorded", meta.MemoryRecall)
	}
}
