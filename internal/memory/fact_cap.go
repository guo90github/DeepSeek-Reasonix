package memory

import (
	"time"
)

// MaxProjectFacts and MaxGlobalFacts cap active facts per scope. The whole
// index loads into session-context at every session start, so the cap is
// enforced at write time: growth becomes curation instead.
const (
	MaxProjectFacts = 40
	MaxGlobalFacts  = 15
)

// FactCap returns the active-fact cap for a scope: the configured value when
// the store carries one, otherwise the package default. Zero means uncapped.
func (s Store) FactCap(scope FactScope) int {
	configured, fallback := s.MaxProjectFacts, MaxProjectFacts
	if NormalizeFactScope(string(scope)) == FactScopeGlobal {
		configured, fallback = s.MaxGlobalFacts, MaxGlobalFacts
	}
	if configured > 0 {
		return configured
	}
	return fallback
}

// FactCapUsage reports the live fact count and the cap for a scope, so
// management surfaces can show how close the store is to needing curation.
func (s Store) FactCapUsage(scope FactScope) (live, cap int) {
	now := time.Now().UTC()
	want := NormalizeFactScope(string(scope))
	for _, fact := range s.ListAll() {
		if NormalizeFactScope(string(fact.Scope)) != want {
			continue
		}
		if memoryFreshness(fact, now) == FreshnessExpired {
			continue
		}
		live++
	}
	return live, s.FactCap(scope)
}
