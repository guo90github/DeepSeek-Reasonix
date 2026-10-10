package agent

// batchNudgeState is the batching hint's per-turn memory: how many consecutive
// single read-only rounds the host has seen, and whether it already spoke. It is
// a named sub-state rather than two loose fields so the streak and the latch
// cannot drift apart — the struct-state ratchet exists to stop that growth.
type batchNudgeState struct {
	streak int
	nudged bool
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
