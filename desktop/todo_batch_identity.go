package main

import (
	"slices"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/evidence"
)

// todoBatchIDForSessionPath resolves the batch the current task list belongs
// to and persists the record only when the identity actually changes: the meta
// view is rebuilt on every poll, so a read must never rewrite the sidecar.
func (a *App) todoBatchIDForSessionPath(sessionPath string, todos []evidence.TodoItem) string {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return ""
	}
	contents := agent.TodoBatchContents(todos)
	if len(contents) == 0 {
		return ""
	}
	previous := agent.LoadTodoBatchIdentity(sessionPath)
	next := agent.ResolveTodoBatchIdentity(previous, contents, agent.NewTodoBatchID)
	if next.ID != previous.ID || next.Closed != previous.Closed || !slices.Equal(next.Contents, previous.Contents) {
		// Best effort: a shelf that cannot record its batch still shows the list.
		_ = agent.SaveTodoBatchIdentity(sessionPath, next)
	}
	return next.ID
}

// todoBatchIDForTab resolves the batch for a tab that may not be bound yet.
func (a *App) todoBatchIDForTab(tabID string, todos []evidence.TodoItem) string {
	tab := a.tabByID(tabID)
	if tab == nil {
		return ""
	}
	return a.todoBatchIDForSessionPath(tab.currentSessionPath(), todos)
}

// todoItemsOf dereferences the optional list the meta view carries.
func todoItemsOf(todos *[]evidence.TodoItem) []evidence.TodoItem {
	if todos == nil {
		return nil
	}
	return *todos
}
