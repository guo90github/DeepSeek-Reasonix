package memory

import (
	"fmt"
	"strings"
	"testing"
)

func TestStoreHonorsConfiguredFactCap(t *testing.T) {
	store := Store{Dir: t.TempDir(), MaxProjectFacts: 2}
	for i := range 2 {
		if _, err := store.Save(Memory{Name: fmt.Sprintf("f-%d", i), Body: "b"}); err != nil {
			t.Fatalf("fact %d within the configured cap was rejected: %v", i, err)
		}
	}
	_, err := store.Save(Memory{Name: "f-2", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "2-fact cap") {
		t.Fatalf("the configured cap must reject the third fact, got %v", err)
	}
	live, limit := store.FactCapUsage(FactScopeProject)
	if live != 2 || limit != 2 {
		t.Fatalf("FactCapUsage = %d/%d, want 2/2", live, limit)
	}
}

func TestStoreFallsBackToTheDefaultFactCap(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if got := store.FactCap(FactScopeProject); got != MaxProjectFacts {
		t.Fatalf("unconfigured project cap = %d, want %d", got, MaxProjectFacts)
	}
	if got := store.FactCap(FactScopeGlobal); got != MaxGlobalFacts {
		t.Fatalf("unconfigured global cap = %d, want %d", got, MaxGlobalFacts)
	}
}
