package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/evidence"
)

// stubTodos answers only Todos; the /todos path reaches nothing else.
type stubTodos struct {
	control.SessionAPI
	todos []evidence.TodoItem
}

func (s stubTodos) Todos() []evidence.TodoItem { return s.todos }

// A remote client reads this endpoint, so an item's stable identity has to
// survive the projection: dropped, a retitle reaches the client as new work.
func TestTodosEndpointCarriesStepID(t *testing.T) {
	s := &Server{ctrl: stubTodos{todos: []evidence.TodoItem{
		{Content: "改两条描述", Status: "in_progress", StepID: "plan_step_01"},
	}}}
	rec := httptest.NewRecorder()
	s.todos(rec, httptest.NewRequest(http.MethodGet, "/todos", nil))

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /todos: %v", err)
	}
	if len(got) != 1 || got[0]["step_id"] != "plan_step_01" {
		t.Fatalf("/todos = %s, want one item carrying step_id", rec.Body.String())
	}
	if got[0]["content"] != "改两条描述" || got[0]["status"] != "in_progress" {
		t.Fatalf("/todos dropped the existing fields: %s", rec.Body.String())
	}
}
