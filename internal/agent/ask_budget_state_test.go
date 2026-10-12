package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
)

// askProbes carry disjoint wording: two probes that share most of their terms
// would read as the same ambiguity, and the second would be reused instead of
// asked — which is not what a budget test measures.
var askProbes = []string{
	`{"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]}`,
	`{"questions":[{"header":"Release","question":"What should happen next?","options":[{"label":"Deploy"},{"label":"Hold"}]}]}`,
	`{"questions":[{"header":"Notify","question":"Tell the team about it?","options":[{"label":"Yes"},{"label":"No"}]}]}`,
	`{"questions":[{"header":"Scope","question":"Widen the scope?","options":[{"label":"Widen"},{"label":"Narrow"}]}]}`,
}

func TestAskRefusesAfterThePerTurnBudget(t *testing.T) {
	asker := &recordingAsker{}
	ctx := withTurnState(withCallContext(context.Background(), "call", event.Discard, asker, false), &turnRuntime{})
	for i := 0; i < maxAnsweredAsksPerTurn; i++ {
		if _, err := NewAskTool().Execute(ctx, []byte(askProbes[i])); err != nil {
			t.Fatalf("ask %d: %v", i, err)
		}
	}
	if asker.calls != maxAnsweredAsksPerTurn {
		t.Fatalf("asker calls = %d, want %d", asker.calls, maxAnsweredAsksPerTurn)
	}
	out, err := NewAskTool().Execute(ctx, []byte(askProbes[maxAnsweredAsksPerTurn]))
	if err != nil {
		t.Fatalf("the refusal must be a result the model can act on, not an error: %v", err)
	}
	if !strings.Contains(out, "already asked") {
		t.Fatalf("result = %q", out)
	}
	if asker.calls != maxAnsweredAsksPerTurn {
		t.Fatalf("the refused question still reached the user: asker calls = %d", asker.calls)
	}
}

// Spending the prompt budget must not close off the reuse path: a settled
// decision still answers the model without troubling the user.
func TestAskBudgetDoesNotCountReusedDecisions(t *testing.T) {
	memory := decidedMemory()
	asker := &recordingAsker{}
	ctx := withDecisionMemory(withTurnState(withCallContext(context.Background(), "call", event.Discard, asker, false), &turnRuntime{}), memory)
	ids := make([]string, 0, maxAnsweredAsksPerTurn)
	for i := 0; i < maxAnsweredAsksPerTurn; i++ {
		out, err := NewAskTool().Execute(ctx, []byte(askProbes[i]))
		if err != nil {
			t.Fatalf("ask %d: %v", i, err)
		}
		ids = append(ids, decisionIDFromResult(t, out))
	}
	reused, err := NewAskTool().Execute(ctx, []byte(`{
		"decision_id":"`+ids[0]+`",
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err != nil {
		t.Fatalf("reuse after the budget: %v", err)
	}
	if !strings.Contains(reused, "reused accepted decision") {
		t.Fatalf("result = %q", reused)
	}
	if asker.calls != maxAnsweredAsksPerTurn {
		t.Fatalf("reuse reached the user: asker calls = %d", asker.calls)
	}
}

func TestAskReportsHostAutoSelectionAsFallback(t *testing.T) {
	ctx := withCallContext(context.Background(), "call", event.Discard,
		fixedAsker{answers: []event.AskAnswer{{QuestionID: "q1", Selected: []string{"Keep going"}, Auto: true}}}, false)
	out, err := NewAskTool().Execute(ctx, []byte(askProbes[0]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "The user answered") {
		t.Fatalf("an auto-selection is reported as the user's answer: %q", out)
	}
	for _, want := range []string{"host fallback, not a user choice", "auto-selected after no answer"} {
		if !strings.Contains(out, want) {
			t.Fatalf("result = %q, want it to contain %q", out, want)
		}
	}
}
