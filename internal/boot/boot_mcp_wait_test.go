package boot

import (
	"testing"
	"time"

	"reasonix/internal/plugin"
)

func waitForMCPFailure(t *testing.T, h *plugin.Host, name string, timeout time.Duration) []plugin.Failure {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		failures := h.Failures()
		for _, f := range failures {
			if f.Name == name {
				return failures
			}
		}
		if time.Now().After(deadline) {
			return failures
		}
		time.Sleep(10 * time.Millisecond)
	}
}
