package skill

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type recordingUseRecorder struct {
	names []string
	hash  []string
}

func (r *recordingUseRecorder) RecordSkillUse(name, contentHash string) {
	r.names = append(r.names, name)
	r.hash = append(r.hash, contentHash)
}

// B3 (docs/50 §2.2): running a skill fingerprints it — name plus a hash of what
// it says — and never hands the body to the record.
func TestRunSkillRecordsAFingerprintNotTheBody(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reasonix/skills/hot.md", "---\ndescription: hot skill\n---\nHOT-BODY-B3")
	store := New(Options{HomeDir: home, DisableBuiltins: true})
	tool := NewRunSkillTool(store, nil)
	recorder := &recordingUseRecorder{}
	ctx := WithUseRecorder(context.Background(), recorder)

	if _, err := tool.Execute(ctx, json.RawMessage(`{"name":"hot"}`)); err != nil {
		t.Fatalf("run_skill: %v", err)
	}
	if len(recorder.names) != 1 || recorder.names[0] != "hot" {
		t.Fatalf("recorded = %+v, want one use of 'hot'", recorder.names)
	}
	hash := recorder.hash[0]
	if hash == "" || strings.Contains(hash, "HOT") || strings.Contains(hash, "hot skill") {
		t.Fatalf("recorded hash = %q, want a content fingerprint only", hash)
	}

	sk, ok := store.Read("hot")
	if !ok {
		t.Fatal("skill missing from store")
	}
	if got := SkillContentHash(sk); got != hash {
		t.Fatalf("hash = %q, want the same fingerprint as the record %q", got, hash)
	}

	// The same skill hashes the same; an edit to its body changes the hash.
	writeSkill(t, home, ".reasonix/skills/quiet.md", "---\ndescription: hot skill\n---\nHOT-BODY-B3")
	quiet, ok := New(Options{HomeDir: home, DisableBuiltins: true}).Read("quiet")
	if !ok {
		t.Fatal("renamed skill missing")
	}
	if got := SkillContentHash(quiet); got != hash {
		t.Fatalf("identical body/description = %q, want the same fingerprint %q", got, hash)
	}
	writeSkill(t, home, ".reasonix/skills/hot.md", "---\ndescription: hot skill\n---\nHOT-BODY-CHANGED")
	edited, ok := store.Read("hot")
	if !ok {
		t.Fatal("edited skill missing")
	}
	if got := SkillContentHash(edited); got == hash {
		t.Fatalf("an edited body kept the fingerprint %q", got)
	}
}

// Without a recorder in context nothing is recorded and nothing fails.
func TestRunSkillWithoutRecorderIsSilent(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reasonix/skills/plain.md", "---\ndescription: plain\n---\nBody.")
	tool := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), nil)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"plain"}`)); err != nil {
		t.Fatalf("run_skill: %v", err)
	}
}
