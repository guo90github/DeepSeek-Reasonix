package control

import "time"

// promptTimeouts are the two waits a prompt can be bounded by — grouped so the
// approvalManager's struct-state ratchet grows by one field, not two.
type promptTimeouts struct {
	// approval bounds how long requestApproval/Ask block on a user decision; zero
	// waits indefinitely (right for an interactive terminal), while frontends with
	// nobody watching set it so the session cannot wedge forever (#4626, #4402).
	approval time.Duration
	// ask is the no-answer fallback for a displayed question: zero uses
	// defaultAskAutoAnswerTimeout, a negative value waits for the user instead
	// of answering with the recommended option.
	ask time.Duration
}

// askAutoAnswerWait resolves the fallback wait, or zero when it is disabled.
func (t promptTimeouts) askAutoAnswerWait() time.Duration {
	switch {
	case t.ask < 0:
		return 0
	case t.ask == 0:
		return defaultAskAutoAnswerTimeout
	default:
		return t.ask
	}
}
