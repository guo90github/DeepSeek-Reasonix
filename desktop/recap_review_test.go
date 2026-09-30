package main

import (
	"strings"
	"testing"

	"reasonix/internal/memory"
	"reasonix/internal/recap"
)

func TestRecapMemoryFactCarriesTheNote(t *testing.T) {
	fact := recapMemoryFact(
		recap.Entry{Kind: recap.KindRootCause, Body: "the parser moved", Evidence: "internal/parser.go"},
		"the parser moved")
	if fact.Type != memory.TypeProject || fact.Scope != memory.FactScopeProject {
		t.Fatalf("a diagnosed root cause is a project fact: %+v", fact)
	}
	if fact.Name == "" || fact.Title == "" || fact.Description != "the parser moved" {
		t.Fatalf("the fact needs a name, a title, and a one-line description: %+v", fact)
	}
	if !strings.Contains(fact.Body, "the parser moved") || !strings.Contains(fact.Body, "internal/parser.go") {
		t.Fatalf("the fact body must keep the note and its evidence: %q", fact.Body)
	}
}

// Pointers and the tier travel into the fact, and the tier is only ever reported:
// memory is written on an explicit accept and an accept still lands in the current
// project, so a note cannot talk its way into the person's global memory.
func TestRecapMemoryFactKeepsPointersAndReportsTheTier(t *testing.T) {
	fact := recapMemoryFact(recap.Entry{
		Kind: recap.KindRefuted,
		Body: "只取分支统计未提交数会漏掉 CJK 路径",
		Refs: []recap.Ref{
			{Kind: recap.RefPath, Value: "desktop/gitstats.go", Detail: "L40"},
			{Kind: recap.RefCommand, Value: "go test ./desktop/"},
		},
		Scope: recap.Scope{Level: recap.ScopeGeneric, Reason: "两个项目都踩过"},
	}, "只取分支统计未提交数会漏掉 CJK 路径")

	if fact.Scope != memory.FactScopeProject {
		t.Fatalf("accepting must still land in the project, got %q", fact.Scope)
	}
	if !strings.Contains(fact.Body, "path desktop/gitstats.go L40") ||
		!strings.Contains(fact.Body, "command go test ./desktop/") {
		t.Fatalf("the pointers must reach the fact: %q", fact.Body)
	}
	if !strings.Contains(fact.Body, "**Scope proposed:** generic") ||
		!strings.Contains(fact.Body, "still writes to the current project") {
		t.Fatalf("the proposed tier is reported, and reported as not yet applied: %q", fact.Body)
	}
}

func TestRecapMemoryRefutedReadsAsClosedGuidance(t *testing.T) {
	fact := recapMemoryFact(
		recap.Entry{Kind: recap.KindRefuted, Body: "polishing the inline decoration again"},
		"polishing the inline decoration again")
	if fact.Type != memory.TypeFeedback {
		t.Fatalf("a ruled-out option is guidance about how to work: %+v", fact)
	}
	if !strings.Contains(fact.Body, "do not propose it again") {
		t.Fatalf("the body must say the option is closed: %q", fact.Body)
	}
}

func TestRecapMemoryKeepsDistinctNamesPerNote(t *testing.T) {
	first := recapMemoryFact(recap.Entry{Kind: recap.KindFact, Body: "第一件事"}, "第一件事")
	second := recapMemoryFact(recap.Entry{Kind: recap.KindFact, Body: "第二件事"}, "第二件事")
	if first.Name == second.Name {
		t.Fatalf("two notes must not collide on one memory name: %q", first.Name)
	}
}
