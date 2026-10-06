package control

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/memory"
	"reasonix/internal/sessioncontext"
)

// B1b (docs/50 §六): a remember call that cannot be auto-written must say why in
// a way the human (and, on denial, the model) can act on — including which
// existing fact to update instead of creating a near-duplicate.
func TestRememberWriteNoteNamesTheExistingFact(t *testing.T) {
	store := memory.Store{Dir: t.TempDir()}
	if _, err := store.Save(memory.Memory{
		Name: "release-target", Title: "Release target", Description: "Current release branch",
		Type: memory.TypeProject, Scope: memory.FactScopeProject, Body: "Use main-v2.",
	}); err != nil {
		t.Fatal(err)
	}
	existing, ok := store.Read("release-target")
	if !ok {
		t.Fatal("seed fact missing")
	}
	c := New(Options{Memory: &memory.Set{Store: store}})

	duplicate := json.RawMessage(`{"name":"release-target","description":"Changed release branch","type":"project","body":"Use release-v2."}`)
	note := c.rememberWriteNote(memoryRememberTool, duplicate, "")
	if !strings.Contains(note, "existing") || !strings.Contains(note, "update") || !strings.Contains(note, existing.ID) {
		t.Fatalf("note = %q, want the existing fact and the update path", note)
	}
	if combined := c.rememberWriteNote(memoryRememberTool, duplicate, "policy says ask"); !strings.Contains(combined, "policy says ask") || !strings.Contains(combined, existing.ID) {
		t.Fatalf("combined note = %q, want both the policy reason and the guidance", combined)
	}

	// A low-risk project create stays silent, and other tools are untouched.
	fresh := json.RawMessage(`{"name":"fresh-fact","description":"A new project fact","type":"project","body":"Body."}`)
	if got := c.rememberWriteNote(memoryRememberTool, fresh, "policy says ask"); got != "policy says ask" {
		t.Fatalf("a low-risk create must add no noise: %q", got)
	}
	if got := c.rememberWriteNote(memoryForgetTool, duplicate, "policy says ask"); got != "policy says ask" {
		t.Fatalf("other tools must be untouched: %q", got)
	}
}

// The human-facing seam: the guidance reaches the approval the user sees. The
// model gets the same wording from the remember tool's own result (see
// TestRememberToolRefusesAShadowingCreateAndPointsAtTheUpdate), because a denial
// exits through the gate without a reason on that path.
func TestRememberApprovalCarriesTheUpdateGuidance(t *testing.T) {
	store := memory.Store{Dir: t.TempDir()}
	if _, err := store.Save(memory.Memory{
		Name: "release-target", Title: "Release target", Description: "Current release branch",
		Type: memory.TypeProject, Scope: memory.FactScopeProject, Body: "Use main-v2.",
	}); err != nil {
		t.Fatal(err)
	}
	approvals := make(chan event.Approval, 1)
	c := New(Options{
		Memory: &memory.Set{Store: store},
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.ApprovalRequest {
				approvals <- e.Approval
			}
		}),
	})
	c.SetToolApprovalMode(ToolApprovalAsk)

	args := json.RawMessage(`{"name":"release-target","description":"Changed release branch","type":"project","body":"Use release-v2."}`)
	go func() {
		_, _, _, _ = gateApprover{c}.ApproveWithReason(context.Background(), memoryRememberTool, "", args)
	}()

	var approval event.Approval
	select {
	case approval = <-approvals:
	case <-time.After(30 * time.Second):
		t.Fatal("remember approval was not emitted")
	}
	if !strings.Contains(approval.Reason, "existing") || !strings.Contains(approval.Reason, "update") {
		t.Fatalf("approval reason = %q, want the update guidance", approval.Reason)
	}
	c.Approve(approval.ID, false, false, false)
}

// B2 (docs/50 §2.2): a turn's recall decision is recorded with its turn number,
// and each hit says whether it reached the model or was dropped.
func TestRecallAuditCarriesTurnAndInjected(t *testing.T) {
	result := memory.RecallResult{
		Query: "release branch", TurnSeq: 7, UsedChars: 120, Omitted: 1,
		Hits:    []memory.RecallHit{{Memory: memory.Memory{ID: "mem-a", Revision: 2, Scope: memory.FactScopeProject, Type: memory.TypeProject}, Score: 0.9, Freshness: "fresh"}},
		Dropped: []memory.RecallHit{{Memory: memory.Memory{ID: "mem-b", Revision: 1, Scope: memory.FactScopeProject, Type: memory.TypeProject}, Score: 0.4, Freshness: "stale"}},
	}
	audit := memoryRecallAudit(result)
	if audit.TurnSeq != 7 || audit.UsedChars != 120 || audit.Omitted != 1 {
		t.Fatalf("audit = %+v, want the turn and counters", audit)
	}
	if len(audit.Hits) != 2 {
		t.Fatalf("audit hits = %+v, want served and dropped fingerprints", audit.Hits)
	}
	if !audit.Hits[0].Injected || audit.Hits[0].ID != "mem-a" {
		t.Fatalf("first hit = %+v, want mem-a injected", audit.Hits[0])
	}
	if audit.Hits[1].Injected || audit.Hits[1].ID != "mem-b" {
		t.Fatalf("second hit = %+v, want mem-b recorded as dropped", audit.Hits[1])
	}
}

// The sidecar door: what the review page reads, without touching the trajectory.
func TestRecallTurnLandsOnTheSessionSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recall.jsonl")
	// A real turn always carries session-context sections, so seed one: the record
	// must fingerprint a non-empty snapshot rather than pass vacuously.
	c := New(Options{SessionDir: dir, SessionPath: path, Sink: event.Discard,
		SessionContextStatic: sessioncontext.Sections{Environment: "os=windows"}})

	c.recordMemoryRecallTurn(memory.RecallResult{
		Query: "release branch", TurnSeq: 3, UsedChars: 40,
		Hits: []memory.RecallHit{{Memory: memory.Memory{ID: "mem-a", Revision: 1}, Score: 0.8}},
	})
	// A second recall inside the same turn replaces the entry rather than adding one.
	c.recordMemoryRecallTurn(memory.RecallResult{
		Query: "release branch", TurnSeq: 3, UsedChars: 55, Suppressed: "budget",
		Dropped: []memory.RecallHit{{
			Memory: memory.Memory{
				ID: "mem-c", Revision: 1, Scope: memory.FactScopeProject, Type: memory.TypeProject,
				Description: "release target",
			},
			Score: 0.3, Freshness: "stale",
		}},
	})

	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	if len(meta.MemoryRecall) != 1 {
		t.Fatalf("records = %+v, want one entry for turn 3", meta.MemoryRecall)
	}
	turn, ok := agent.LatestMemoryRecallTurn(meta)
	if !ok || turn.TurnSeq != 3 || turn.UsedChars != 55 || turn.Suppressed != "budget" {
		t.Fatalf("turn = %+v, want the later decision for turn 3", turn)
	}
	if len(turn.Hits) != 1 || turn.Hits[0].ID != "mem-c" || turn.Hits[0].Injected == nil || *turn.Hits[0].Injected {
		t.Fatalf("hits = %+v, want the dropped fingerprint marked not injected", turn.Hits)
	}
	if hit := turn.Hits[0]; hit.Scope != "project" || hit.Type != "project" || hit.Freshness != "stale" {
		t.Fatalf("hit state = %+v, want the fact's own scope/type/freshness recorded", hit)
	}
	if turn.Hits[0].Description != "release target" {
		t.Fatalf("hit description = %q, want the fact's own one-line hook", turn.Hits[0].Description)
	}
	if turn.SnapshotDigest == "" {
		t.Fatal("a recorded turn must carry the session-context digest it ran against")
	}
	if turn.QueryHash == "" || strings.Contains(turn.QueryHash, "release") {
		t.Fatalf("query hash = %q, want a content-free hash", turn.QueryHash)
	}
}
