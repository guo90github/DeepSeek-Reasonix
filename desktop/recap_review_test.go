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
