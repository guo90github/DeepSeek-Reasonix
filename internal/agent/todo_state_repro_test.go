package agent

import (
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

// Requirement 17 reproductions at the kernel seam. The session keeps exactly one
// list: whatever the newest todo_write said. Nothing records what a later list
// dropped, so an unfinished item from an earlier list cannot be shown again.

func todoReproAgent(t *testing.T) *Agent {
	t.Helper()
	a := New(&scriptedProvider{}, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	return a
}

func todoContents(todos []evidence.TodoItem) []string {
	out := make([]string, 0, len(todos))
	for _, todo := range todos {
		out = append(out, todo.Content+":"+todo.Status)
	}
	return out
}

// Flipped by the board (docs/40 §5 step 2): the canonical list is still only
// the newest word, but the unfinished half of the earlier list stays owed on
// the board instead of disappearing.
func TestTodoWriteReplacesTheCanonicalListButTheBoardKeepsTheUnfinishedHalf(t *testing.T) {
	a := todoReproAgent(t)

	a.setTodoState([]evidence.TodoItem{
		{Content: "第一步", Status: "completed"},
		{Content: "第二步", Status: "in_progress"},
		{Content: "第三步", Status: "pending"},
	})
	before := todoContents(a.sess.todoState)
	if len(before) != 3 {
		t.Fatalf("fixture: first list = %v", before)
	}

	// The model re-plans: a fresh list that never mentions the unfinished work.
	a.setTodoState([]evidence.TodoItem{{Content: "另起一批", Status: "in_progress"}})
	after := todoContents(a.sess.todoState)

	if len(after) != 1 || after[0] != "另起一批:in_progress" {
		t.Fatalf("the canonical list must stay the newest word: %v", after)
	}
	board := a.TodoBoardState()
	if got := todoContents(board.Queue); len(got) != 3 ||
		got[0] != "第二步:in_progress" || got[1] != "第三步:pending" || got[2] != "另起一批:in_progress" {
		t.Fatalf("the board must still owe the earlier unfinished items: %v", got)
	}
	if got := todoContents(board.Archive); len(got) != 1 || got[0] != "第一步:completed" {
		t.Fatalf("the board must remember what finished: %v", got)
	}
}

func TestTodoStateKeepsOnlyTheNewestStatuses(t *testing.T) {
	a := todoReproAgent(t)
	a.setTodoState([]evidence.TodoItem{
		{Content: "写方案", Status: "completed"},
		{Content: "做实现", Status: "in_progress"},
	})
	// Re-sending the same items with new statuses is how completion advances.
	a.setTodoState([]evidence.TodoItem{
		{Content: "写方案", Status: "completed"},
		{Content: "做实现", Status: "completed"},
	})
	if got := todoContents(a.sess.todoState); len(got) != 2 || got[1] != "做实现:completed" {
		t.Fatalf("statuses must follow the newest write: %v", got)
	}
}
