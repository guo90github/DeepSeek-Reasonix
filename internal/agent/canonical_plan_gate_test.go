package agent

import (
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

// A canonical checklist outlives its completion: continuing the same work must
// not demand a fresh plan before the next multi-file write.
func TestHasCanonicalPlanCountsCompletedLists(t *testing.T) {
	a := New(&scriptedProvider{name: "p"}, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	if a.hasCanonicalPlan() {
		t.Fatal("an empty session has no plan")
	}
	a.sess.todoState = []evidence.TodoItem{{Content: "wire the gate", Status: "completed"}}
	if a.hasActiveCanonicalTodo() {
		t.Fatal("a completed list is not active")
	}
	if !a.hasCanonicalPlan() {
		t.Fatal("a completed list is still the session's plan")
	}
}
