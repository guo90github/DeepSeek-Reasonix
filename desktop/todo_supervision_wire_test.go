package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/evidence"
)

// A note (the model's own list) omits the key so every older frontend keeps the
// pinned panel; a commitment the host enforces reports true.
func TestTodosSupervisionMetaWireContract(t *testing.T) {
	note := &todoMetaController{todos: []evidence.TodoItem{{Content: "Ship", Status: "in_progress"}}}
	commitment := &todoMetaController{supervised: true, todos: note.todos}
	for _, ctrl := range []*todoMetaController{note, commitment} {
		raw, err := json.Marshal(Meta{CanonicalTodos: ctrlTodos(ctrl), TodosSupervised: ctrl.TodosSupervised()})
		if err != nil {
			t.Fatalf("marshal todos meta: %v", err)
		}
		if got := strings.Contains(string(raw), `"todosSupervised":true`); got != ctrl.supervised {
			t.Fatalf("supervised=%v meta = %s", ctrl.supervised, raw)
		}
	}
}

// MetaForTab is the only path a frontend reads, so the flag must come off the
// live controller rather than the encoder alone.
func TestMetaForTabReportsTodoSupervisionFromController(t *testing.T) {
	isolateDesktopUserDirs(t)

	app := NewApp()
	app.ctx = context.Background()
	app.readyHook = func() {}
	ctrl := control.New(control.Options{Label: "test/todo-supervision"})
	app.setTestCtrl(ctrl, "test/todo-supervision")
	defer func() {
		if c := app.activeCtrl(); c != nil {
			c.Close()
		}
	}()

	if app.Meta().TodosSupervised {
		t.Fatal("an open turn reported its task list as a commitment")
	}
	ctrl.SetPlanMode(true)
	if !app.Meta().TodosSupervised {
		t.Fatal("plan mode did not report its task list as a commitment")
	}
	ctrl.SetPlanMode(false)
	if app.Meta().TodosSupervised {
		t.Fatal("leaving plan mode kept the commitment flag")
	}
}
