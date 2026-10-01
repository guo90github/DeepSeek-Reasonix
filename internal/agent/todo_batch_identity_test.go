package agent

import (
	"path/filepath"
	"testing"

	"reasonix/internal/evidence"
)

func todoItems(contents ...string) []evidence.TodoItem {
	out := make([]evidence.TodoItem, 0, len(contents))
	for i, content := range contents {
		status := "pending"
		if i == 0 {
			status = "in_progress"
		}
		out = append(out, evidence.TodoItem{Content: content, Status: status})
	}
	return out
}

func fixedBatchID(id string) func() string { return func() string { return id } }

// The transition table is the fix for "完成的会再次跳出": an edit to the same
// work keeps the batch, so a closed batch stays closed.
func TestResolveTodoBatchIdentityKeepsABatchAcrossEdits(t *testing.T) {
	first := ResolveTodoBatchIdentity(TodoBatchIdentity{}, TodoBatchContents(todoItems("第一步", "第二步")), fixedBatchID("todos-1"))
	if first.ID != "todos-1" || first.Closed {
		t.Fatalf("a new list must issue a fresh open batch: %+v", first)
	}

	closed := first
	closed.Closed = true

	extended := ResolveTodoBatchIdentity(closed, TodoBatchContents(todoItems("第一步", "第二步", "第三步")), fixedBatchID("todos-2"))
	if extended.ID != "todos-1" {
		t.Fatalf("adding a step must not mint a new batch: %+v", extended)
	}
	if !extended.Closed {
		t.Fatal("a closed batch must stay closed when its own work is edited")
	}

	reworded := ResolveTodoBatchIdentity(closed, TodoBatchContents(todoItems("第一步（改）", "第二步")), fixedBatchID("todos-3"))
	if reworded.ID != "todos-1" || !reworded.Closed {
		t.Fatalf("rewording one item must keep the closed batch: %+v", reworded)
	}

	fresh := ResolveTodoBatchIdentity(closed, TodoBatchContents(todoItems("另一件事")), fixedBatchID("todos-4"))
	if fresh.ID != "todos-4" || fresh.Closed {
		t.Fatalf("unrelated work is a new batch and shows: %+v", fresh)
	}
}

func TestResolveTodoBatchIdentityAdoptsEditsWhileOpen(t *testing.T) {
	open := ResolveTodoBatchIdentity(TodoBatchIdentity{}, TodoBatchContents(todoItems("甲")), fixedBatchID("todos-1"))
	edited := ResolveTodoBatchIdentity(open, TodoBatchContents(todoItems("甲", "乙")), fixedBatchID("todos-2"))
	if edited.ID != "todos-1" || edited.Closed {
		t.Fatalf("an open batch keeps its id through an edit: %+v", edited)
	}
	if len(edited.Contents) != 2 {
		t.Fatalf("the adopted contents must be the newest list: %+v", edited.Contents)
	}
}

func TestResolveTodoBatchIdentityLeavesAnEmptyListAlone(t *testing.T) {
	closed := TodoBatchIdentity{ID: "todos-1", Contents: []string{"甲"}, Closed: true}
	if got := ResolveTodoBatchIdentity(closed, nil, fixedBatchID("todos-2")); got.ID != "todos-1" || !got.Closed {
		t.Fatalf("no list says nothing about the batch: %+v", got)
	}
}

func TestNewTodoBatchIDIsUniqueAndPrefixed(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		id := NewTodoBatchID()
		if id == "" || seen[id] {
			t.Fatalf("unusable batch id %q", id)
		}
		seen[id] = true
	}
}

func TestTodoBatchIdentityRoundTripsThroughTheSidecar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := SaveTodoBatchIdentity(path, TodoBatchIdentity{ID: "todos-1", Contents: []string{"甲", "乙"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded := LoadTodoBatchIdentity(path)
	if loaded.ID != "todos-1" || len(loaded.Contents) != 2 || loaded.Closed {
		t.Fatalf("round trip: %+v", loaded)
	}

	if err := CloseTodoBatchIdentity(path, "todos-other"); err != nil {
		t.Fatalf("close other: %v", err)
	}
	if LoadTodoBatchIdentity(path).Closed {
		t.Fatal("closing another batch must not close this one")
	}
	if err := CloseTodoBatchIdentity(path, "todos-1"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !LoadTodoBatchIdentity(path).Closed {
		t.Fatal("the close must stick to the batch it names")
	}
}
