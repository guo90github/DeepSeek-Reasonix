package agent

import (
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func boardContents(items []evidence.TodoItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Content+":"+item.Status)
	}
	return out
}

func eqBoard(t *testing.T, got []evidence.TodoItem, want []string, label string) {
	t.Helper()
	contents := boardContents(got)
	if len(contents) != len(want) {
		t.Fatalf("%s: got %v, want %v", label, contents, want)
	}
	for i := range want {
		if contents[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", label, contents, want)
		}
	}
}

// MergeTodoBoard keeps what a later list leaves unfinished, which is the fix for
// "最新待办顶掉历史未完成待办后历史不可见".
func TestMergeTodoBoardKeepsUnfinishedWorkAcrossLists(t *testing.T) {
	board := MergeTodoBoard(TodoBoard{}, []evidence.TodoItem{
		{Content: "第一步", Status: "completed"},
		{Content: "第二步", Status: "in_progress"},
		{Content: "第三步", Status: "pending"},
	})
	eqBoard(t, board.Queue, []string{"第二步:in_progress", "第三步:pending"}, "the queue holds the unfinished items")
	eqBoard(t, board.Archive, []string{"第一步:completed"}, "the archive holds what finished")

	// A replacement list that never mentions the previous work.
	board = MergeTodoBoard(board, []evidence.TodoItem{{Content: "另起一批", Status: "in_progress"}})
	eqBoard(t, board.Queue, []string{"第二步:in_progress", "第三步:pending", "另起一批:in_progress"}, "the earlier unfinished items stay in the queue")
	eqBoard(t, board.Archive, []string{"第一步:completed"}, "the archive is untouched")
}

func TestMergeTodoBoardAdvancesStatusesInPlace(t *testing.T) {
	board := MergeTodoBoard(TodoBoard{}, []evidence.TodoItem{
		{Content: "甲", Status: "in_progress"},
		{Content: "乙", Status: "pending"},
	})
	board = MergeTodoBoard(board, []evidence.TodoItem{
		{Content: "甲", Status: "completed"},
		{Content: "乙", Status: "in_progress"},
	})
	eqBoard(t, board.Queue, []string{"乙:in_progress"}, "a finished item leaves the queue")
	eqBoard(t, board.Archive, []string{"甲:completed"}, "and joins the archive in order")

	// Resuming an archived item brings it back.
	board = MergeTodoBoard(board, []evidence.TodoItem{{Content: "甲", Status: "pending"}})
	eqBoard(t, board.Queue, []string{"乙:in_progress", "甲:pending"}, "resumed work returns to the queue")
}

func TestMergeTodoBoardIgnoresAnEmptyList(t *testing.T) {
	board := MergeTodoBoard(TodoBoard{}, []evidence.TodoItem{{Content: "甲", Status: "in_progress"}})
	if got := MergeTodoBoard(board, nil); len(got.Queue) != 1 || len(got.Archive) != 0 {
		t.Fatalf("an empty list says nothing about the board: %+v", got)
	}
}

func TestMergeTodoBoardBoundsBothSides(t *testing.T) {
	board := TodoBoard{}
	for i := range todoBoardLimit + 40 {
		board = MergeTodoBoard(board, []evidence.TodoItem{{Content: string(rune('a'+i%26)) + itoa(i), Status: "in_progress"}})
	}
	if len(board.Queue) > todoBoardLimit {
		t.Fatalf("queue grew to %d", len(board.Queue))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

// The agent's own seam: two todo_writes in one session, the second dropping the
// first's unfinished work — the board still owes it.
func TestSharedTodoStateKeepsTheUnfinishedHalfOnTheBoard(t *testing.T) {
	a := New(&scriptedProvider{}, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	a.setTodoState([]evidence.TodoItem{
		{Content: "第一步", Status: "completed"},
		{Content: "第二步", Status: "in_progress"},
		{Content: "第三步", Status: "pending"},
	})
	a.setTodoState([]evidence.TodoItem{{Content: "另起一批", Status: "in_progress"}})

	eqBoard(t, a.CanonicalTodoState(), []string{"另起一批:in_progress"}, "the canonical list is still only the newest word")
	board := a.TodoBoardState()
	eqBoard(t, board.Queue, []string{"第二步:in_progress", "第三步:pending", "另起一批:in_progress"}, "the board still owes the earlier unfinished items")
	eqBoard(t, board.Archive, []string{"第一步:completed"}, "and remembers what finished")
}

// A transcript replay is what a host with no bound controller shows (docs/40
// S2): the canonical list is the latest word, the board still owes the rest.
func TestReplayTodoHistoryRebuildsTheBoardFromATranscript(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c1", Name: "todo_write", Arguments: `{"todos":[{"content":"第一步","status":"completed","level":0},{"content":"第二步","status":"in_progress","level":0},{"content":"第三步","status":"pending","level":0}]}`}}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: "ok"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c2", Name: "todo_write", Arguments: `{"todos":[{"content":"另起一批","status":"in_progress","level":0}]}`}}},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: "ok"},
	}
	todos, board, hasList := ReplayTodoHistory(msgs)
	if !hasList {
		t.Fatalf("the transcript carries two lists")
	}
	eqBoard(t, todos, []string{"另起一批:in_progress"}, "the canonical list is the latest list")
	eqBoard(t, board.Queue, []string{"第二步:in_progress", "第三步:pending", "另起一批:in_progress"}, "the board owes the unfinished half of both lists")
	eqBoard(t, board.Archive, []string{"第一步:completed"}, "and archives what finished")

	if _, _, hasList := ReplayTodoHistory(nil); hasList {
		t.Fatalf("a transcript with no list reports none")
	}
}
