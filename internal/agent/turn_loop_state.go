package agent

import (
	"sync"

	"reasonix/internal/tool"
)

// turnLoopState groups per-turn loop-guard maps so parallel tool goroutines
// share one lock instead of unsynchronized maps on turnRuntime.
type turnLoopState struct {
	mu                      sync.Mutex
	dispatchClasses         map[string]tool.CallClass
	resultFingerprints      map[string]string
	acceptedDecisions       map[string]acceptedDecision
	previousErrorCategories map[string]struct{}
	softBudgetNudged        bool
	softBudgetNudgeRound    int
	batching                batchNudgeState
	asks                    askBudgetState
}

func (s *turnLoopState) setDispatchClasses(classes map[string]tool.CallClass) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatchClasses = classes
}

func (s *turnLoopState) dispatchClass(id string) (tool.CallClass, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	class, ok := s.dispatchClasses[id]
	return class, ok
}

func (s *turnLoopState) rememberFingerprint(fp, callID string) (prev string, seen bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resultFingerprints == nil {
		s.resultFingerprints = map[string]string{}
	}
	prev, seen = s.resultFingerprints[fp]
	if !seen {
		s.resultFingerprints[fp] = callID
	}
	return prev, seen
}

func (s *turnLoopState) advanceErrorCategories(current map[string]int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	hit := false
	next := make(map[string]struct{}, len(current))
	for category, count := range current {
		if count >= 2 {
			hit = true
		}
		if _, repeated := s.previousErrorCategories[category]; repeated {
			hit = true
		}
		next[category] = struct{}{}
	}
	s.previousErrorCategories = next
	return hit
}

func (s *turnLoopState) markSoftBudgetNudged(round int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.softBudgetNudged {
		return false
	}
	s.softBudgetNudged = true
	s.softBudgetNudgeRound = round
	return true
}

func (s *turnLoopState) softBudgetNudgedAt() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.softBudgetNudgeRound
}
