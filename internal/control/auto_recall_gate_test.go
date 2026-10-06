package control

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

// Step 4 (docs/50 §2.2): with automatic recall off a turn injects nothing — and says
// so, both to the model and on the sidecar, so an empty turn is explained rather
// than looking broken.
func TestAutoRecallOffLeavesTheTurnExplained(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ondemand.jsonl")
	off := false
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard, MemoryAutoRecall: &off})

	text := c.Compose("把支付部署到绿色集群")
	if !strings.Contains(text, "memory-on-demand") {
		t.Fatalf("turn = %q, want the on-demand notice", text)
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	if len(meta.MemoryRecall) != 1 {
		t.Fatalf("records = %+v, want the turn recorded anyway", meta.MemoryRecall)
	}
	turn := meta.MemoryRecall[0]
	if turn.Suppressed == "" || len(turn.Hits) != 0 {
		t.Fatalf("turn = %+v, want a suppression reason and nothing injected", turn)
	}
	if turn.QueryExcerpt == "" {
		t.Fatalf("turn = %+v, want the ask and its turn recorded", turn)
	}
}

// On stays the default: the switch must not change a session that never set it.
func TestAutoRecallOffOnlyWhenAskedFor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auto.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})

	text := c.Compose("把支付部署到绿色集群")
	if strings.Contains(text, "memory-on-demand") {
		t.Fatalf("turn = %q, want the default to keep recalling automatically", text)
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	if len(meta.MemoryRecall) != 1 || meta.MemoryRecall[0].Suppressed == "" {
		t.Fatalf("records = %+v, want the automatic path to have run and explained itself", meta.MemoryRecall)
	}
}
