package agent

// maxAnsweredAsksPerTurn caps how many questions one turn puts to the user. Each
// question stops the work until the user replies, so a turn that keeps asking
// turns the user into a form; past the cap the tool decides for itself instead of
// registering another prompt.
const maxAnsweredAsksPerTurn = 3

// askBudgetState is the per-turn count of questions this turn put to the user,
// counted whichever way each one resolved (an answer, or the host's no-answer
// fallback). It shares turnLoopState's lifetime and lock, like batchNudgeState.
type askBudgetState struct {
	asked int
}

func (s *turnLoopState) noteAnsweredAsk() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asks.asked++
}

func (s *turnLoopState) askBudgetSpent() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asks.asked >= maxAnsweredAsksPerTurn
}
