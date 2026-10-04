package agentbus

import (
	"strings"
	"time"
)

// closedForSilence reports a silence closure whatever window it records: the reason now
// carries the measured silence and the threshold it fired on, so a test asserts on the
// prefix rather than on the whole string (2026-10-05).
func closedForSilence(reason string) bool {
	return strings.HasPrefix(reason, CloseSilence)
}

func namesSilenceWindow(reason string, window time.Duration) bool {
	return strings.Contains(reason, window.String())
}
