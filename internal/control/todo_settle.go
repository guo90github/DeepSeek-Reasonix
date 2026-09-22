package control

// Unsupervised task lists: a turn the host never promised to deliver leaves its
// list a note, not a commitment. The panel pins incomplete lists, so an open
// turn must state plainly what it left behind instead of hanging silently.

import (
	"fmt"

	"reasonix/internal/evidence"
)

// settleUnsupervisedTodos states what an open turn left behind.
func (c *Controller) settleUnsupervisedTodos(startMessages int) {
	if c.executor == nil || c.TodosSupervised() {
		return
	}
	todos := c.executor.CanonicalTodoState()
	if len(todos) == 0 {
		return
	}
	left := len(evidence.IncompleteTodos(todos))
	if left == 0 {
		return
	}
	// A list the model advanced this turn is its own current statement; the host
	// only speaks for a list the model walked away from.
	if c.hasTodoUpdateSince(startMessages) {
		return
	}
	// The list is left untouched: the host holds no receipt for work it never
	// promised, and inventing completions would make a stale list false. The
	// notice states it now because the pinned panel shows items, not this text.
	message := fmt.Sprintf("task list left with %d of %d items unfinished: this turn was not a supervised delivery", left, len(todos))
	c.emitTodoState("turn-settle", todos, message)
	c.notice(message)
}

// TodosSupervised reports whether an unfinished item is a commitment the host
// enforces (an active goal or plan mode) rather than the model's own note. It
// reads active() rather than deliveryScope(): frontends call this on the meta
// path, and a display read must not assign a goal scope id.
func (c *Controller) TodosSupervised() bool {
	if c.goals.active() {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionSettings.planMode
}
