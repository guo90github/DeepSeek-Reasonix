package main

import (
	"strings"
	"testing"

	"reasonix/internal/memory"
)

// An outcome only earns its place in memory if it reads as an answer: the person's
// own line about how the item ended, plus what it was answering. Without the item
// a resolution looks like a rule with no question behind it.
func TestResolvedItemOutcomeBecomesAProjectFact(t *testing.T) {
	fact := recapOutcomeFact(
		">6 条要点条没有 +N 提示",
		"desktop/frontend/src/components/AnswerKeyPoints.tsx L21",
		"加了 +N 提示：超过 6 条时显示剩余数量")

	if fact.Type != memory.TypeProject || fact.Scope != memory.FactScopeProject {
		t.Fatalf("an outcome is a project fact: %+v", fact)
	}
	if fact.Name == "" || fact.Title == "" || fact.Description != "加了 +N 提示：超过 6 条时显示剩余数量" {
		t.Fatalf("the fact needs a name, a title and the outcome as its description: %+v", fact)
	}
	for _, want := range []string{
		"加了 +N 提示：超过 6 条时显示剩余数量",
		"**Why:**",
		"**How to apply:**",
		"**Item:** >6 条要点条没有 +N 提示",
		"Evidence: desktop/frontend/src/components/AnswerKeyPoints.tsx L21",
	} {
		if !strings.Contains(fact.Body, want) {
			t.Fatalf("the outcome body must keep %q: %q", want, fact.Body)
		}
	}
}
