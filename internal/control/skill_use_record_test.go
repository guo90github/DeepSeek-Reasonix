package control

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

// B3 (docs/50 §2.2): a skill invocation lands on the session sidecar as a
// fingerprint — the turn, the skill's name, its content hash, and the catalog
// digest the turn's prompt carried.
func TestSkillUseLandsOnTheSessionSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills.jsonl")
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard})

	c.RecordSkillUse("hot", "abc123")
	c.RecordSkillUse("hot", "abc123") // same turn and skill replaces the entry
	c.RecordSkillUse("other", "def456")

	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	if len(meta.SkillUse) != 2 {
		t.Fatalf("records = %+v, want one entry per skill in the turn", meta.SkillUse)
	}
	last, ok := agent.LatestSkillUse(meta)
	if !ok || last.Name != "other" || last.ContentHash != "def456" {
		t.Fatalf("latest = %+v, want the second skill", last)
	}
	for _, use := range meta.SkillUse {
		if use.TurnSeq != c.Turn() {
			t.Fatalf("turn = %d, want the controller's current turn %d", use.TurnSeq, c.Turn())
		}
		if use.CatalogDigest == "" || strings.Contains(use.CatalogDigest, "hot") {
			t.Fatalf("catalog digest = %q, want a content-free fingerprint", use.CatalogDigest)
		}
	}

	// Without a session path there is nothing to record and nothing fails.
	bare := New(Options{Sink: event.Discard})
	bare.RecordSkillUse("hot", "abc123")
}
