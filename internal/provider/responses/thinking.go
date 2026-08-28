package responses

import (
	"strings"

	"reasonix/internal/provider"
)

// requestEffort folds a per-request thinking-off override into the configured
// effort: "disabled" maps to "none", the Responses API's reasoning-off knob.
// The session never sets the override, so its configured effort is untouched.
func requestEffort(configured string, req provider.Request) string {
	if req.ThinkingDisabled() {
		return "none"
	}
	return strings.ToLower(strings.TrimSpace(configured))
}
