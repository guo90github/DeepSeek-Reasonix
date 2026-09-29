package recap

import (
	"strings"

	"reasonix/internal/agent"
)

// Remover reports whether a session is inside a removal window, so a recap is
// never generated for a conversation the user just discarded. Callers pass the
// controller-side check; the kernel never reaches up to it.
type Remover func(sessionPath string) bool

// Admissible reports whether a session may be recapped: a normal visible
// transcript, not a recovery copy, not pending cleanup, and not mid-removal.
func Admissible(sessionPath string, removing Remover) bool {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return false
	}
	if agent.LooksLikeRecoveryFilename(path) {
		return false
	}
	if !agent.IsVisibleSession(path) {
		return false
	}
	if removing != nil && removing(path) {
		return false
	}
	return true
}
