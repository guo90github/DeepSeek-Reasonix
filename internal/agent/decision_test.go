package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
)

func TestAskRejectsMoreThanThreeQuestions(t *testing.T) {
	_, err := NewAskTool().Execute(context.Background(), []byte(`{
		"questions":[
			{"header":"A","question":"a?","options":[{"label":"1"},{"label":"2"}]},
			{"header":"B","question":"b?","options":[{"label":"1"},{"label":"2"}]},
			{"header":"C","question":"c?","options":[{"label":"1"},{"label":"2"}]},
			{"header":"D","question":"d?","options":[{"label":"1"},{"label":"2"}]}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "at most 3") {
		t.Fatalf("error = %v", err)
	}
}

func decidedMemory(decisions ...acceptedDecision) *decisionMemory {
	memory := newDecisionMemory()
	for _, decision := range decisions {
		memory.remember(decision)
	}
	return memory
}

func TestAskReusesAcceptedDecisionWithoutNewEvidence(t *testing.T) {
	ctx := withDecisionMemory(context.Background(), decidedMemory(acceptedDecision{ID: "dec-1", Question: "Which path?", Answer: "Keep going"}))
	out, err := NewAskTool().Execute(ctx, []byte(`{
		"decision_id":"dec-1",
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "reused accepted decision") {
		t.Fatalf("got %q", out)
	}
}

func TestAskAllowsReopenWithNewEvidence(t *testing.T) {
	memory := decidedMemory(acceptedDecision{ID: "dec-1", Question: "Which path?", Answer: "Keep going"})
	asker := &recordingAsker{}
	ctx := withDecisionMemory(withCallContext(context.Background(), "c1", event.Discard, asker, false), memory)
	out, err := NewAskTool().Execute(ctx, []byte(`{
		"decision_id":"dec-1",
		"new_evidence":"new metric from the latest run",
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "decision_id") {
		t.Fatalf("got %q", out)
	}
	if len(asker.questions) != 1 {
		t.Fatalf("asker questions = %d", len(asker.questions))
	}
}

// The memory belongs to the conversation, so a question the user already settled
// in an earlier turn never reaches them again.
func TestAskReusesDecisionSettledInAnEarlierTurn(t *testing.T) {
	memory := decidedMemory()
	first := &recordingAsker{}
	firstTurn := withDecisionMemory(withCallContext(context.Background(), "call-1", event.Discard, first, false), memory)
	opened, err := NewAskTool().Execute(firstTurn, []byte(`{
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err != nil {
		t.Fatalf("first ask: %v", err)
	}
	if first.calls != 1 {
		t.Fatalf("first asker calls = %d", first.calls)
	}

	second := &recordingAsker{}
	nextTurn := withDecisionMemory(withCallContext(context.Background(), "call-2", event.Discard, second, false), memory)
	again, err := NewAskTool().Execute(nextTurn, []byte(`{
		"decision_id":"`+decisionIDFromResult(t, opened)+`",
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err != nil {
		t.Fatalf("next turn: %v", err)
	}
	if second.calls != 0 {
		t.Fatalf("the settled question reached the user again: asker calls = %d", second.calls)
	}
	if !strings.Contains(again, "reused accepted decision") {
		t.Fatalf("next turn result = %q", again)
	}
}

func TestAskUnknownDecisionIDNamesTheKnownOnes(t *testing.T) {
	ctx := withDecisionMemory(context.Background(), decidedMemory(acceptedDecision{ID: "dec-1", Question: "Which path?", Answer: "Keep going"}))
	_, err := NewAskTool().Execute(ctx, []byte(`{
		"decision_id":"dec-typo",
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err == nil {
		t.Fatal("expected the spoofed decision id to be rejected")
	}
	if !strings.Contains(err.Error(), "dec-typo") || !strings.Contains(err.Error(), "dec-1") {
		t.Fatalf("error = %v, want it to name the decision id the model meant", err)
	}
}

func TestAskReportsReusedAutoSelectionAsHostFallback(t *testing.T) {
	ctx := withDecisionMemory(context.Background(), decidedMemory(acceptedDecision{ID: "dec-1", Question: "Which path?", Answer: "Keep going", Auto: true}))
	out, err := NewAskTool().Execute(ctx, []byte(`{
		"decision_id":"dec-1",
		"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "host auto-selected") || !strings.Contains(out, "the user did not choose it") {
		t.Fatalf("reuse of an auto-selected decision reads as the user's own choice: %q", out)
	}
}

func decisionIDFromResult(t *testing.T, result string) string {
	t.Helper()
	_, id, ok := strings.Cut(result, "decision_id: ")
	if !ok {
		t.Fatalf("ask result carries no decision_id: %q", result)
	}
	return strings.TrimSpace(id)
}
