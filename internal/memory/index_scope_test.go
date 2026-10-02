package memory

import (
	"strings"
	"testing"
)

// #7995: the stable index and automatic recall must share one world view. On a
// name collision both scopes stay visible with qualified references, and the
// shadowed global entry says which project fact overrides it.
func TestIndexShowsBothScopesWithOverrideAnnotation(t *testing.T) {
	store := recallTestStore(t)
	recallTestWrite(t, store.Dir, Memory{
		ID: "mem-proj", Name: "deploy-region", Title: "Deploy region",
		Description: "This project deploys to eu-central-1", Type: TypeProject,
		Scope: FactScopeProject, Body: "eu-central-1",
	})
	recallTestWrite(t, store.GlobalDir, Memory{
		ID: "mem-glob", Name: "deploy-region", Title: "Deploy region",
		Description: "Default deploy region", Type: TypeProject,
		Scope: FactScopeGlobal, Body: "us-east-1",
	})

	index := store.Index()
	// Both scopes stay visible, each entry linked by its own stable id; only
	// the shadow annotation names the winning fact.
	for _, want := range []string{"](mem-proj)", "](mem-glob)", "(overridden by project/deploy-region.md)"} {
		if !strings.Contains(index, want) {
			t.Fatalf("index missing %q:\n%s", want, index)
		}
	}
	if strings.Contains(index, "](project/") || strings.Contains(index, "](global/") {
		t.Fatalf("provider index must link ids, never paths:\n%s", index)
	}
	// The project entry (the recall winner) must not carry the annotation.
	for line := range strings.SplitSeq(index, "\n") {
		if strings.Contains(line, "](mem-proj)") && strings.Contains(line, "overridden") {
			t.Fatalf("winning project entry wrongly annotated: %s", line)
		}
	}
}
