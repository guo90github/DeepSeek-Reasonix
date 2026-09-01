package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/evidence"
)

// recoveryPlanTransition detects structural rewrites of an active canonical
// task list. Initial plans and progress-only status updates stay on the fast
// path; changing step identity, order, or hierarchy while work remains is a
// semantic transition for the independent Auto reviewer.
func (a *Agent) recoveryPlanTransition(toolName string, args json.RawMessage) (bool, string, string, string) {
	if a == nil || toolName != "todo_write" || a.planMode.Load() {
		return false, "", "", ""
	}
	before := a.CanonicalTodoState()
	if len(before) == 0 || len(evidence.IncompleteTodos(before)) == 0 {
		return false, "", "", ""
	}
	after := evidence.ReceiptFromToolCall("todo_write", args, true, true).Todos
	if evidence.ValidateSerialTodos(after) != nil {
		return false, "", "", ""
	}
	if len(after) == 0 {
		return true, planReviewText(before), planReviewText(after), planTransitionDiff(before, after)
	}
	if !evidence.PreservesCompletedTodoPositions(before, after) {
		// Let todo_write report malformed or invalid state directly; an invalid
		// task list is not a meaningful plan proposal for the reviewer.
		return false, "", "", ""
	}
	if samePlanStructure(before, after) {
		return false, "", "", ""
	}
	if isPureCurrentItemSplit(before, after) {
		// Splitting the current item into a phase plus one in_progress
		// sub-step is the shape todo_write's continuity guard now permits;
		// it narrows work instead of restructuring the plan.
		return false, "", "", ""
	}
	return true, planReviewText(before), planReviewText(after), planTransitionDiff(before, after)
}

// isPureCurrentItemSplit reports whether after is exactly before with the
// current item expanded into a level-0 phase header whose first sub-step is
// the new current item. Anything more — a retitle, a reorder, an extra item —
// stays a plan transition for the independent reviewer.
func isPureCurrentItemSplit(before, after []evidence.TodoItem) bool {
	if len(after) != len(before)+1 {
		return false
	}
	h := -1
	for i := range before {
		if canonicalTodoStatus(before[i].Status) == "in_progress" {
			h = i
			break
		}
	}
	if h < 0 {
		return false
	}
	for i := range h {
		if before[i].Level != after[i].Level || normalizePlanStep(before[i].Content) != normalizePlanStep(after[i].Content) {
			return false
		}
	}
	if before[h].Level != after[h].Level || normalizePlanStep(before[h].Content) != normalizePlanStep(after[h].Content) {
		return false
	}
	if canonicalTodoStatus(after[h].Status) != "pending" {
		return false
	}
	if after[h+1].Level != 1 || canonicalTodoStatus(after[h+1].Status) != "in_progress" {
		return false
	}
	for i := h + 1; i < len(before); i++ {
		if before[i].Level != after[i+1].Level || normalizePlanStep(before[i].Content) != normalizePlanStep(after[i+1].Content) {
			return false
		}
	}
	return true
}

func samePlanStructure(a, b []evidence.TodoItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Level != b[i].Level || normalizePlanStep(a[i].Content) != normalizePlanStep(b[i].Content) {
			return false
		}
	}
	return true
}

func normalizePlanStep(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func planReviewText(todos []evidence.TodoItem) string {
	var b strings.Builder
	for i, todo := range todos {
		indent := ""
		if todo.Level == 1 {
			indent = "  "
		}
		fmt.Fprintf(&b, "%s%d. %s [%s]", indent, i+1, normalizePlanStep(todo.Content), canonicalTodoStatus(todo.Status))
		if i+1 < len(todos) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
