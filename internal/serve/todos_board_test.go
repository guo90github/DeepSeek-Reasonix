package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/evidence"
)

// stubBoard answers only TodoBoard; the /todos/board path reaches nothing else.
type stubBoard struct {
	control.SessionAPI
	board agent.TodoBoard
}

func (s stubBoard) TodoBoard() agent.TodoBoard { return s.board }

// A remote tab reads this endpoint to show work a later list left owed, so both
// sides have to survive the projection — including the empty one, which must
// encode as [] rather than null.
func TestTodosBoardEndpointCarriesQueueAndArchive(t *testing.T) {
	s := &Server{ctrl: stubBoard{board: agent.TodoBoard{
		Queue: []evidence.TodoItem{{Content: "串行验证并提交", Status: "in_progress", StepID: "p0_serve_board"}},
	}}}
	rec := httptest.NewRecorder()
	s.todosBoard(rec, httptest.NewRequest(http.MethodGet, "/todos/board", nil))

	var got struct {
		Queue   []map[string]any `json:"queue"`
		Archive []map[string]any `json:"archive"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /todos/board: %v", err)
	}
	if len(got.Queue) != 1 || got.Queue[0]["step_id"] != "p0_serve_board" {
		t.Fatalf("queue = %s, want one item carrying step_id", rec.Body.String())
	}
	if got.Queue[0]["content"] != "串行验证并提交" || got.Queue[0]["status"] != "in_progress" {
		t.Fatalf("queue dropped the existing fields: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"archive":[]`) {
		t.Fatalf("an empty archive must encode as [], not null: %s", rec.Body.String())
	}
}
