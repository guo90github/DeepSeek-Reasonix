package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAssessRememberWriteAutoAllowsOnlyLowRiskProjectCreates(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	safe := json.RawMessage(`{"name":"release-target","description":"Release target for this project","type":"project","body":"Release artifacts are published from main-v2."}`)
	assessment := AssessRememberWrite(store, safe)
	if !assessment.AutoAllow || assessment.Name != "release-target" || assessment.Reason == "" {
		t.Fatalf("safe project create assessment = %+v", assessment)
	}

	cases := map[string]json.RawMessage{
		"implicit type":    json.RawMessage(`{"name":"release-target","description":"Release target","body":"Use main-v2."}`),
		"global":           json.RawMessage(`{"name":"release-target","description":"Release target","type":"project","scope":"global","body":"Use main-v2."}`),
		"global reference": json.RawMessage(`{"name":"global/release-target.md","description":"Release target","type":"project","body":"Use main-v2."}`),
		"user preference":  json.RawMessage(`{"name":"prefers-go","description":"Preferred language","type":"user","body":"Prefer Go."}`),
		"feedback":         json.RawMessage(`{"name":"concise","description":"Response style","type":"feedback","body":"Keep answers concise."}`),
		"stable id update": json.RawMessage(`{"id":"mem-existing","expected_revision":1,"description":"Update","type":"project","body":"Updated body."}`),
		"credential":       json.RawMessage(`{"name":"deploy-key","description":"Deploy credential","type":"project","body":"DEPLOY_API_KEY=sk-example-secret-value-123456"}`),
		"email":            json.RawMessage(`{"name":"release-owner","description":"Release owner","type":"project","body":"Contact release-owner@example.test."}`),
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if got := AssessRememberWrite(store, args); got.AutoAllow || got.Reason == "" {
				t.Fatalf("assessment = %+v, want approval with reason", got)
			}
		})
	}
}

func TestAssessRememberWriteRejectsConflictingReferenceScope(t *testing.T) {
	store := Store{Dir: t.TempDir(), GlobalDir: t.TempDir()}
	args := json.RawMessage(`{"name":"global/release-target.md","description":"Release target","type":"project","scope":"project","body":"Use main-v2."}`)
	got := AssessRememberWrite(store, args)
	if got.AutoAllow || !strings.Contains(got.Reason, "conflicts") {
		t.Fatalf("conflicting reference assessment = %+v", got)
	}
}

func TestAssessRememberWriteRequiresApprovalForExistingOrSemanticDuplicate(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if _, err := store.Save(Memory{
		Name: "release-target", Title: "Release target", Description: "Current release branch",
		Type: TypeProject, Scope: FactScopeProject, Body: "Use main-v2.",
	}); err != nil {
		t.Fatal(err)
	}

	for _, args := range []json.RawMessage{
		json.RawMessage(`{"name":"release-target","description":"Changed release branch","type":"project","body":"Use release-v2."}`),
		json.RawMessage(`{"name":"another-name","title":"Release target","description":"Current release branch","type":"project","body":"Use main-v2."}`),
	} {
		if got := AssessRememberWrite(store, args); got.AutoAllow || !strings.Contains(got.Reason, "existing") {
			t.Fatalf("duplicate assessment = %+v", got)
		}
	}
}

func TestRememberAutoWriteClaimRemainsCreateOnlyAtExecution(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	args := json.RawMessage(`{"name":"release-target","description":"Release target","type":"project","body":"Use main-v2."}`)
	claim := &fakeAutoWriteQueue{claim: true}
	ctx := WithQueue(context.Background(), claim)

	// Simulate another writer creating the same name after approval assessment but
	// before the remember tool executes.
	if _, err := store.Save(Memory{Name: "release-target", Description: "concurrent", Body: "Do not overwrite."}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRememberTool(store).Execute(ctx, args); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("auto-approved create became an overwrite: %v", err)
	}
	got, ok := store.Read("release-target")
	if !ok || got.Body != "Do not overwrite." {
		t.Fatalf("concurrent memory was overwritten: %+v, ok=%v", got, ok)
	}
}

type fakeAutoWriteQueue struct {
	notes []string
	claim bool
}

func (q *fakeAutoWriteQueue) QueueMemory(note string) { q.notes = append(q.notes, note) }
func (q *fakeAutoWriteQueue) ClaimAutoMemoryWrite(json.RawMessage) bool {
	claimed := q.claim
	q.claim = false
	return claimed
}

// B1 (docs/50 A-32): where a fact lands when the caller names no scope.
//
// Guards first: the three thresholds below already hold, so they are pinned here
// rather than presented as new behavior.
func TestAssessRememberWriteRejectsNonProjectTypesRegardlessOfScope(t *testing.T) {
	store := Store{Dir: t.TempDir(), GlobalDir: t.TempDir()}
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"name":"prefers-go","description":"Preferred language","type":"user","scope":"project","body":"Prefer Go."}`),
		json.RawMessage(`{"name":"concise","description":"Response style","type":"feedback","scope":"project","body":"Keep answers concise."}`),
	} {
		got := AssessRememberWrite(store, args)
		if got.AutoAllow || !strings.Contains(got.Reason, "project/reference") {
			t.Fatalf("non-project type assessment = %+v", got)
		}
	}
}

// The flip: a user/feedback fact without an explicit scope is not a project fact
// silently downgraded to retrieval; it is global by default, so an automatic
// write must be refused *because the effective scope is global*.
func TestAssessRememberWriteTreatsUnscopedUserFactAsGlobal(t *testing.T) {
	store := Store{Dir: t.TempDir(), GlobalDir: t.TempDir()}
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"name":"prefers-go","description":"Preferred language","type":"user","body":"Prefer Go."}`),
		json.RawMessage(`{"name":"concise","description":"Response style","type":"feedback","body":"Keep answers concise."}`),
	} {
		got := AssessRememberWrite(store, args)
		if got.AutoAllow {
			t.Fatalf("unscoped global-by-default fact was auto-allowed: %+v", got)
		}
		if got.Scope != FactScopeGlobal {
			t.Fatalf("effective scope = %q, want global", got.Scope)
		}
		if !strings.Contains(got.Reason, "global") {
			t.Fatalf("reason must name the effective scope: %+v", got)
		}
	}
	// An explicit project scope keeps the same fact project-local (still not an
	// automatic write: the type is what the low-risk path refuses).
	if got := AssessRememberWrite(store, json.RawMessage(`{"name":"prefers-go","description":"Preferred language","type":"user","scope":"project","body":"Prefer Go."}`)); got.Scope != FactScopeProject {
		t.Fatalf("explicit project scope = %q, want project", got.Scope)
	}
}

// The flip: a semantic duplicate is not a dead end — the assessment names the
// fact that already covers it, so the caller can steer to an update.
func TestAssessRememberWriteNamesTheExistingFactToUpdate(t *testing.T) {
	store := Store{Dir: t.TempDir(), GlobalDir: t.TempDir()}
	if _, err := store.Save(Memory{
		Name: "release-target", Title: "Release target", Description: "Current release branch",
		Type: TypeProject, Scope: FactScopeProject, Body: "Use main-v2.",
	}); err != nil {
		t.Fatal(err)
	}
	saved, ok := store.Read("release-target")
	if !ok {
		t.Fatal("seed fact missing")
	}
	got := AssessRememberWrite(store, json.RawMessage(`{"name":"release-target","description":"Changed release branch","type":"project","body":"Use release-v2."}`))
	if got.AutoAllow || !strings.Contains(got.Reason, "existing") {
		t.Fatalf("duplicate assessment = %+v", got)
	}
	if got.OverlapID != saved.ID || got.OverlapRevision != saved.Revision || got.OverlapName != saved.Name {
		t.Fatalf("assessment must name the existing fact: got %+v, want id=%s revision=%d name=%s",
			got, saved.ID, saved.Revision, saved.Name)
	}
	if !strings.Contains(got.Reason, saved.ID) || !strings.Contains(got.Reason, "update") {
		t.Fatalf("reason must point at the update path: %+v", got)
	}
}
