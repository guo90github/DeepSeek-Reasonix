package main

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// S2 (docs/40 §5 step 3): with no bound controller the shelf used to render
// nothing until the tab was focused. The transcript already holds the list.
func TestReplayedTodosForSessionReadsAnUnboundSessionsShelf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.jsonl")
	session := agent.NewSession("")
	session.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		ID:        "c1",
		Name:      "todo_write",
		Arguments: `{"todos":[{"content":"第一步","status":"completed","level":0},{"content":"第二步","status":"in_progress","level":0},{"content":"第三步","status":"pending","level":0}]}`,
	}}})
	session.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "c1", Content: "ok"})
	session.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		ID:        "c2",
		Name:      "todo_write",
		Arguments: `{"todos":[{"content":"另起一批","status":"in_progress","level":0}]}`,
	}}})
	session.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "c2", Content: "ok"})
	if err := session.Save(path); err != nil {
		t.Fatalf("save session: %v", err)
	}

	resetTodoHistoryReplays()
	todos, board, has := replayedTodosForSession(path, "digest-1", 1)
	if !has {
		t.Fatalf("the transcript records two task lists")
	}
	if len(todos) != 1 || todos[0].Content != "另起一批" {
		t.Fatalf("canonical list = %v, want only the latest list", todos)
	}
	if len(board.Queue) != 3 || board.Queue[0].Content != "第二步" || board.Queue[2].Content != "另起一批" {
		t.Fatalf("board queue = %v, want the unfinished half of both lists", board.Queue)
	}
	if len(board.Archive) != 1 || board.Archive[0].Content != "第一步" {
		t.Fatalf("board archive = %v, want the finished item", board.Archive)
	}

	// Memoized by (path, digest, revision): the old key keeps answering from the
	// memo, while a changed key re-reads the transcript.
	extended := agent.NewSession("")
	extended.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		ID:        "c3",
		Name:      "todo_write",
		Arguments: `{"todos":[{"content":"再一批","status":"in_progress","level":0}]}`,
	}}})
	extended.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "c3", Content: "ok"})
	if err := extended.Save(path); err != nil {
		t.Fatalf("re-save session: %v", err)
	}

	if todos, _, _ := replayedTodosForSession(path, "digest-1", 1); len(todos) != 1 || todos[0].Content != "另起一批" {
		t.Fatalf("the memoized (path, digest, revision) must still answer: %v", todos)
	}
	todos, _, has = replayedTodosForSession(path, "digest-2", 2)
	if !has || len(todos) != 1 || todos[0].Content != "再一批" {
		t.Fatalf("a changed revision must re-read the transcript: %v", todos)
	}
}

func TestReplayedTodosForSessionSkipsAPathlessSession(t *testing.T) {
	resetTodoHistoryReplays()
	_, board, has := replayedTodosForSession("  ", "", 0)
	if has || len(board.Queue) != 0 || len(board.Archive) != 0 {
		t.Fatalf("an unbound tab without a transcript owes nothing")
	}
}
