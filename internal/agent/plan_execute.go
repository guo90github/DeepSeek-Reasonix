package agent

import "strings"

// PlanExecuteTier gates the host-executed half of step three: a round may carry
// concrete read-only calls that the host runs itself, so N independent reads
// cost one model round instead of N. Off ships the tool surface unchanged —
// parallel_calls is not registered at all.
type PlanExecuteTier string

const (
	PlanExecuteOff PlanExecuteTier = ""
	PlanExecuteOn  PlanExecuteTier = "on"
)

// ParsePlanExecuteTier reads the configured tier. Anything unrecognized stays
// off, so a typo can never widen what the model may ask the host to run.
func ParsePlanExecuteTier(raw string) PlanExecuteTier {
	if strings.EqualFold(strings.TrimSpace(raw), string(PlanExecuteOn)) {
		return PlanExecuteOn
	}
	return PlanExecuteOff
}

// Enabled reports whether host-executed calls are available in this run.
func (t PlanExecuteTier) Enabled() bool { return t == PlanExecuteOn }
