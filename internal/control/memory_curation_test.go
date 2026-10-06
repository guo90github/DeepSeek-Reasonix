package control

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

// The curation view must name the fact the session kept being handed and never used,
// and stay quiet about one whose words did show up in a reply.
func TestMemoryCurationNamesTheFactsNeverUsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "curation.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})

	yes, no := true, false
	used, unused := true, false
	write := func(turn agent.MemoryRecallTurn) {
		if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
			agent.AppendMemoryRecallTurn(meta, turn)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for turn := 1; turn <= 2; turn++ {
		write(agent.MemoryRecallTurn{
			TurnSeq: turn, QueryExcerpt: "把支付部署到绿色集群",
			Hits: []agent.MemoryRecallTurnHit{
				{ID: "mem-hot", Name: "hot-fact", Injected: &yes, LikelyUsed: &unused},
				{ID: "mem-cool", Name: "cool-fact", Injected: &yes, LikelyUsed: &used},
				{ID: "mem-dropped", Name: "dropped-fact", Injected: &no},
			},
		})
	}

	out := renderMemoryCuration(c)
	if !strings.Contains(out, "2 turn(s)") {
		t.Fatalf("out = %q, want the session summary", out)
	}
	if !strings.Contains(out, "hot-fact  injected 2") {
		t.Fatalf("out = %q, want the repeated hand-over counted", out)
	}
	at := strings.Index(out, "rewrite the keywords")
	if at < 0 {
		t.Fatalf("out = %q, want a rewrite hint for the never-used fact", out)
	}
	tail := out[at:]
	if !strings.Contains(tail, "hot-fact") {
		t.Fatalf("tail = %q, want the never-used fact named", tail)
	}
	if strings.Contains(tail, "cool-fact") {
		t.Fatalf("tail = %q, want the used fact left out of the rewrite hint", tail)
	}
	if !strings.Contains(out, "dropped-fact") {
		t.Fatalf("out = %q, want a dropped hit still listed", out)
	}
}
