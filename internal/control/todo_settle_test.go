package control

import (
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

func settleTestController(t *testing.T) (*Controller, *agent.Agent, *[]event.Event) {
	t.Helper()
	events := &[]event.Event{}
	sink := event.FuncSink(func(e event.Event) { *events = append(*events, e) })
	executor := agent.New(nil, nil, agent.NewSession("test"), agent.Options{}, event.Discard)
	return &Controller{sink: sink, executor: executor}, executor, events
}

func todoWriteOutputs(events []event.Event) []string {
	var out []string
	for _, e := range events {
		if e.Kind == event.ToolResult && e.Tool.Name == "todo_write" {
			out = append(out, e.Tool.Output)
		}
	}
	return out
}

func TestSettleUnsupervisedTodosStatesWhatWasLeftBehind(t *testing.T) {
	c, executor, events := settleTestController(t)
	executor.ReplaceTodoState([]evidence.TodoItem{
		{Content: "清点中文字面量", Status: "completed"},
		{Content: "翻译模型可见文案", Status: "in_progress"},
		{Content: "跑四道闸门", Status: "pending"},
	})

	c.settleUnsupervisedTodos(c.sessionMessageCount())

	outputs := todoWriteOutputs(*events)
	if len(outputs) != 1 {
		t.Fatalf("todo_write results = %d, want 1: %v", len(outputs), outputs)
	}
	if !strings.Contains(outputs[0], "2 of 3") {
		t.Errorf("output %q does not state how much was left behind", outputs[0])
	}
	if state := executor.CanonicalTodoState(); len(evidence.IncompleteTodos(state)) != 2 {
		t.Errorf("the settle invented completions: %+v", state)
	}
}

func TestSettleUnsupervisedTodosStaysSilentWhenModelKeptTheList(t *testing.T) {
	c, executor, events := settleTestController(t)
	executor.ReplaceTodoState([]evidence.TodoItem{{Content: "still working", Status: "in_progress"}})
	executor.Session().Messages = append(executor.Session().Messages, provider.Message{
		ToolCalls: []provider.ToolCall{{Name: "todo_write", Arguments: `{"todos":[]}`}},
	})

	c.settleUnsupervisedTodos(0)

	if outputs := todoWriteOutputs(*events); len(outputs) != 0 {
		t.Errorf("a list the model advanced this turn needs no host statement: %v", outputs)
	}
}

func TestSettleUnsupervisedTodosStaysSilentWhenNothingIsUnfinished(t *testing.T) {
	c, executor, events := settleTestController(t)
	executor.ReplaceTodoState([]evidence.TodoItem{
		{Content: "done one", Status: "completed"},
		{Content: "done two", Status: "completed"},
	})
	c.settleUnsupervisedTodos(c.sessionMessageCount())
	if outputs := todoWriteOutputs(*events); len(outputs) != 0 {
		t.Errorf("a finished list needs no statement: %v", outputs)
	}

	executor.ReplaceTodoState(nil)
	c.settleUnsupervisedTodos(c.sessionMessageCount())
	if outputs := todoWriteOutputs(*events); len(outputs) != 0 {
		t.Errorf("an empty list needs no statement: %v", outputs)
	}
}

func TestSettleUnsupervisedTodosLeavesSupervisedListsAlone(t *testing.T) {
	c, executor, events := settleTestController(t)
	c.SetPlanMode(true)
	executor.ReplaceTodoState([]evidence.TodoItem{{Content: "planned step", Status: "in_progress"}})

	if !c.TodosSupervised() {
		t.Fatal("plan mode must count as supervision")
	}
	c.settleUnsupervisedTodos(c.sessionMessageCount())
	if outputs := todoWriteOutputs(*events); len(outputs) != 0 {
		t.Errorf("a supervised list is settled by its own plan path, not here: %v", outputs)
	}
}

func TestSettleUnsupervisedTodosLeavesGoalListsAlone(t *testing.T) {
	c, executor, events := settleTestController(t)
	c.goals.set("ship the fix", "", nil)
	executor.ReplaceTodoState([]evidence.TodoItem{{Content: "goal step", Status: "pending"}})

	if !c.TodosSupervised() {
		t.Fatal("a running goal must count as supervision")
	}
	c.settleUnsupervisedTodos(c.sessionMessageCount())
	if outputs := todoWriteOutputs(*events); len(outputs) != 0 {
		t.Errorf("a goal's list is settled by its own path, not here: %v", outputs)
	}
}

func TestSettleUnsupervisedTodosLeavesDeliveryFloorListsAlone(t *testing.T) {
	c, executor, events := settleTestController(t)
	if err := c.SetQualityFloor(QualityFloorDelivery); err != nil {
		t.Fatalf("SetQualityFloor: %v", err)
	}
	executor.ReplaceTodoState([]evidence.TodoItem{{Content: "delivery step", Status: "pending"}})

	if !c.TodosSupervised() {
		t.Fatal("the delivery floor must count as supervision: it arms the readiness gate")
	}
	c.settleUnsupervisedTodos(c.sessionMessageCount())
	if outputs := todoWriteOutputs(*events); len(outputs) != 0 {
		t.Errorf("the delivery floor's list is not a note: %v", outputs)
	}
}
