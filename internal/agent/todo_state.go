package agent

// The host's canonical task list: the state that outlives a turn because it
// never rides in the prompt, so a later turn still sees an unfinished plan.

import (
	"encoding/json"
	"strings"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

// SeedTodoState initializes the canonical task list from a host-generated
// starter list, such as an approved plan. A new host seed replaces stale state
// from earlier work so complete_step matches the plan the UI just displayed.
func (a *Agent) SeedTodoState(todos []evidence.TodoItem) {
	if len(todos) == 0 {
		return
	}
	a.setTodoState(todos)
}

// ReplayTodoHistory derives the canonical task list and the board from a
// transcript: the latest successful todo_write is the base, every complete_step
// after it advances an item, and every list the session carried folds into the
// board so unfinished work stays visible. Deterministic from persisted messages,
// which is what lets a host with no bound controller still show what a session
// owes (docs/40 S2).
func ReplayTodoHistory(msgs []provider.Message) ([]evidence.TodoItem, TodoBoard, bool) {
	successful := successfulToolCallIDs(msgs)
	var todos []evidence.TodoItem
	var lists [][]evidence.TodoItem
	baseIdx := -1
	for i, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			if tc.Name != "todo_write" || !successful[tc.ID] {
				continue
			}
			rec := evidence.ReceiptFromToolCall(tc.Name, json.RawMessage(tc.Arguments), true, true)
			// A successful empty todo_write is an explicit clear. Preserve it as the
			// latest base so history reloads do not resurrect an older non-empty list.
			todos = evidence.NormalizeSerialTodos(rec.Todos)
			lists = append(lists, todos)
			baseIdx = i
		}
	}
	board := TodoBoard{}
	for _, list := range lists {
		board = MergeTodoBoard(board, list)
	}
	if baseIdx < 0 {
		return nil, board, false
	}
	for i := baseIdx; i < len(msgs); i++ {
		for _, tc := range msgs[i].ToolCalls {
			if tc.Name != "complete_step" || !successful[tc.ID] {
				continue
			}
			rec := evidence.ReceiptFromToolCall(tc.Name, json.RawMessage(tc.Arguments), true, true)
			if m, ok := evidence.MatchStep(rec.Step, todos); ok {
				evidence.AdvanceSerialTodo(todos, m.Index-1)
			}
		}
	}
	return todos, board, true
}

// ReplaceTodoState mirrors a host-generated todo list into the canonical state.
// It is used when the host, rather than the model, owns the full state transition.
func (a *Agent) ReplaceTodoState(todos []evidence.TodoItem) {
	a.setTodoState(todos)
	a.recordTodoState(a.CanonicalTodoState())
}

// TodoBoardState returns a copy of the shelf's queue and archive: everything
// this session still owes, plus what finished.
func (a *Agent) TodoBoardState() TodoBoard {
	a.sess.todoMu.Lock()
	defer a.sess.todoMu.Unlock()
	return TodoBoard{
		Queue:   append([]evidence.TodoItem(nil), a.sess.todoBoard.Queue...),
		Archive: append([]evidence.TodoItem(nil), a.sess.todoBoard.Archive...),
	}
}

// CanonicalTodoState returns a copy of the host-reconstructed task list.
func (a *Agent) CanonicalTodoState() []evidence.TodoItem {
	a.sess.todoMu.Lock()
	defer a.sess.todoMu.Unlock()
	return append([]evidence.TodoItem(nil), a.sess.todoState...)
}

// CurrentTaskTodoState returns only the latest successful todo_write retained
// in the current evidence ledger. Unlike CanonicalTodoState, it never falls
// back to a prior user turn.
func (a *Agent) CurrentTaskTodoState() []evidence.TodoItem {
	if a == nil || a.task.ledger == nil {
		return nil
	}
	todos, ok := a.task.ledger.LatestTodos()
	if !ok {
		return nil
	}
	return append([]evidence.TodoItem(nil), todos...)
}

// consumeTodoOnlyReadinessMarkerIfResolved retires a pending final-readiness
// marker whose only gap was unfinished todos once the canonical list shows
// every item completed, so a reload no longer replays the stale wrap-up card.
// In-turn consumption stays with beginFinalReadinessRecovery (next user turn).
func (a *Agent) consumeTodoOnlyReadinessMarkerIfResolved() {
	if a == nil || a.sess.conversation == nil {
		return
	}
	a.sess.todoMu.Lock()
	state := append([]evidence.TodoItem(nil), a.sess.todoState...)
	a.sess.todoMu.Unlock()
	if len(state) == 0 || len(evidence.IncompleteTodos(state)) > 0 {
		return
	}
	marker := a.pendingFinalReadinessRecovery()
	if marker == nil || len(marker.Missing) == 0 {
		return
	}
	for _, id := range marker.Missing {
		if id != "todo" {
			return
		}
	}
	a.sess.conversation.ConsumeFinalReadinessRecovery()
}

func (a *Agent) incompleteCanonicalTodos() ([]evidence.TodoStepMatch, bool) {
	a.sess.todoMu.Lock()
	defer a.sess.todoMu.Unlock()
	if len(a.sess.todoState) == 0 {
		return nil, false
	}
	return evidence.IncompleteTodos(a.sess.todoState), true
}

func (a *Agent) hasIncompleteCanonicalCriteria() bool {
	a.sess.todoMu.Lock()
	defer a.sess.todoMu.Unlock()
	return len(a.sess.todoState) > 0 && len(evidence.IncompleteTodos(a.sess.todoState)) > 0
}

// hasCanonicalPlan reports whether this session has a canonical checklist at
// all. A completed list is still the plan that produced the current work, so
// continuing it must not cost a round trip to re-plan the same change.
func (a *Agent) hasCanonicalPlan() bool {
	a.sess.todoMu.Lock()
	defer a.sess.todoMu.Unlock()
	return len(a.sess.todoState) > 0
}

// recordTodoState logs the host-advanced list as a synthetic todo_write receipt
// so the per-turn final gate (which reads the ledger's latest todo_write) sees
// the advance — the model no longer has to re-send a todo_write to mark the
// completion. It bypasses the todo_write tool, so the completion-transition
// guard never runs on it.
func (a *Agent) recordTodoState(todos []evidence.TodoItem) {
	if a.task.ledger == nil {
		return
	}
	args, err := json.Marshal(map[string]any{"todos": todos})
	if err != nil {
		return
	}
	a.task.ledger.Record(evidence.ReceiptFromToolCall("todo_write", json.RawMessage(args), true, true))
}

func canonicalTodoStatus(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "pending"
	}
	return s
}
