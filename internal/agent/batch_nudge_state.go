package agent

// batchNudgeState is the batching and fan-out hints' per-turn memory: how many
// consecutive single read-only rounds the host has seen, and whether each hint
// already spoke. They live together because they share one lifetime — the
// struct-state ratchet exists to stop loose fields growing one per case.
type batchNudgeState struct {
	streak  int
	nudged  bool
	batched bool
	fanout  bool
}

func (s *turnLoopState) bumpBatchingStreak() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batching.streak++
	return s.batching.streak
}

func (s *turnLoopState) resetBatchingStreak() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batching.streak = 0
}

func (s *turnLoopState) markBatchingNudged() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.batching.nudged {
		return false
	}
	s.batching.nudged = true
	return true
}

func (s *turnLoopState) markFanoutNudged() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.batching.fanout {
		return false
	}
	s.batching.fanout = true
	return true
}

// markBatchingInUse records that a round already carried several calls, so the
// expensive hint may follow it without the cheap one having to fire first.
func (s *turnLoopState) markBatchingInUse() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batching.batched = true
}

// batchingAddressed reports whether the cheaper fix is already suggested (the
// hint spoke) or already in use (a round carried several calls).
func (s *turnLoopState) batchingAddressed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batching.nudged || s.batching.batched
}
