package agent

import "strings"

// PlanExecuteTier gates the host-executed half of step three: a round may carry
// concrete read-only calls that the host runs itself, so N independent reads
// cost one model round instead of N. Off ships the tool surface unchanged —
// parallel_calls is not registered at all.
type PlanExecuteTier string

const (
	PlanExecuteOff      PlanExecuteTier = ""
	PlanExecuteOn       PlanExecuteTier = "on"
	PlanExecutePrefetch PlanExecuteTier = "prefetch"
)

// ParsePlanExecuteTier reads the configured tier. Anything unrecognized stays
// off, so a typo can never widen what the model may ask the host to run.
func ParsePlanExecuteTier(raw string) PlanExecuteTier {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(PlanExecuteOn):
		return PlanExecuteOn
	case string(PlanExecutePrefetch):
		return PlanExecutePrefetch
	}
	return PlanExecuteOff
}

// Enabled reports whether the host-executed call surface is registered.
func (t PlanExecuteTier) Enabled() bool { return t == PlanExecuteOn || t == PlanExecutePrefetch }

// Prefetch reports whether the host also reads the paths the model named. It
// implies Enabled: the surface and the prefetch ride one opt-in.
func (t PlanExecuteTier) Prefetch() bool { return t == PlanExecutePrefetch }
