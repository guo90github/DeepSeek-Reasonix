package memory

import (
	"fmt"
	"strings"
	"testing"
)

func TestFactCapRejectsOverflowAndAllowsUpdates(t *testing.T) {
	store := recallTestStore(t)
	for i := range MaxProjectFacts {
		name := fmt.Sprintf("fact-%02d", i)
		if _, err := store.SaveWithOptions(Memory{
			Name: name, Description: "project fact " + name, Body: "body of " + name,
		}, SaveOptions{}); err != nil {
			t.Fatalf("fact %d within the cap was rejected: %v", i, err)
		}
	}

	if _, err := store.SaveWithOptions(Memory{
		Name: "one-too-many", Description: "beyond the cap", Body: "body",
	}, SaveOptions{}); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("a fact past the cap must be rejected with the cap message, got %v", err)
	}

	// Updating a fact that is already counted must not double-count itself.
	if _, err := store.SaveWithOptions(Memory{
		Name: "fact-00", Description: "project fact fact-00", Body: "updated body",
	}, SaveOptions{}); err != nil {
		t.Fatalf("update at the cap was rejected: %v", err)
	}

	// The cap is per scope: global memory keeps its own budget.
	if _, err := store.SaveWithOptions(Memory{
		Scope: FactScopeGlobal, Name: "global-fact", Description: "global fact", Body: "body",
	}, SaveOptions{}); err != nil {
		t.Fatalf("a global fact was rejected under the project cap: %v", err)
	}
}
