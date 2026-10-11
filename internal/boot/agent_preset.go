package boot

import (
	"reasonix/internal/agent"
	"reasonix/internal/agentpreset"
	"reasonix/internal/tool"
)

// Role vocabulary re-exported for old frontends. Runtime constraints live in
// internal/runtimepolicy; these names only parse compat inputs.

// AgentPreset label constants; light and its aliases fold to standard.
const (
	AgentPresetStandard = string(agentpreset.Standard)
	AgentPresetDelivery = string(agentpreset.Delivery)
)

// Deprecated compat aliases for one-version-old callers.
const (
	AgentPresetBalanced = AgentPresetStandard
	TokenModeFull       = "full"
	TokenModeDelivery   = "delivery"
)

// NormalizeAgentPreset maps free-form and legacy values to the canonical
// role label. Light folds to standard; unknown values return the input
// unchanged so callers can surface an error.
func NormalizeAgentPreset(raw string) string {
	if p, err := agentpreset.Normalize(raw); err == nil {
		return string(p)
	}
	return raw
}

// NormalizeAgentPresetErr is NormalizeAgentPreset reporting parse errors.
func NormalizeAgentPresetErr(raw string) (string, error) {
	p, err := agentpreset.Normalize(raw)
	return string(p), err
}

// NormalizeTokenMode is the deprecated alias that returns legacy tokenMode
// names (full/delivery) for dual-write and older clients.
func NormalizeTokenMode(mode string) string {
	return agentpreset.LegacyTokenMode(agentpreset.FromLegacyTokenMode(mode))
}

// AgentPresetFromTokenMode maps a legacy tokenMode onto a role setting.
func AgentPresetFromTokenMode(mode string) string {
	return string(agentpreset.FromLegacyTokenMode(mode))
}

// TokenModeFromAgentPreset maps a role setting onto the dual-write tokenMode.
func TokenModeFromAgentPreset(preset string) string {
	return agentpreset.LegacyTokenMode(agentpreset.FromLegacyTokenMode(preset))
}

// CoreProviderToolNames is the stable top-level tool surface shared by every
// Agent role setting under identical configuration. Host-control tools
// (ask, update_goal, todo_write, complete_step) are appended when enabled.
func CoreProviderToolNames() []string {
	return []string{
		"bash",
		"bash_output",
		"kill_shell",
		"wait",
		"read_file",
		"view_image",
		"edit_file",
		"write_file",
		"compress",
		"use_capability",
		"web_search",
	}
}

// HostControlToolNames are collaboration/contract tools that may appear in the
// provider schema independently of the Agent role setting.
func HostControlToolNames() []string {
	return []string{
		"ask",
		"update_goal",
		"todo_write",
		"complete_step",
		"agent_bus",
		// Only registered when plan_execute is on: applyUnifiedProviderToolSurface
		// drops allowlist names the registry does not hold, so the off surface is
		// unchanged by this line.
		"parallel_calls",
	}
}

// UnifiedProviderToolNames returns the provider-visible allowlist for a boot
// with host-control tools enabled.
func UnifiedProviderToolNames() []string {
	core := CoreProviderToolNames()
	host := HostControlToolNames()
	out := make([]string, 0, len(core)+len(host))
	out = append(out, core...)
	out = append(out, host...)
	return out
}

// applyUnifiedProviderToolSurface restricts Schemas/ContractEntries to the
// shared core + host-control tools. use_capability can still Get every
// registered tool, including those hidden from the provider schema.
func applyUnifiedProviderToolSurface(reg *tool.Registry, planExecute string) {
	if reg == nil {
		return
	}
	// The host-executed call surface is tier-gated: adding its name to the
	// allowlist below is harmless while it is unregistered, so the off surface
	// stays byte-identical.
	if tier := agent.ParsePlanExecuteTier(planExecute); tier.Enabled() {
		hostCalls := agent.NewParallelCallsTool(reg)
		hostCalls.SetPrefetch(tier.Prefetch())
		reg.Add(hostCalls)
	}
	// Fan-out tools are registered on every build yet stay off the surface; opting
	// into plan_execute is what puts them in front of the model, which is why
	// fan-out never fired before.
	candidates := UnifiedProviderToolNames()
	if agent.ParsePlanExecuteTier(planExecute).Enabled() {
		candidates = append(append([]string(nil), candidates...), tool.HostParallelTasks, tool.HostFleet)
	}
	allow := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if _, ok := reg.Get(name); ok {
			allow = append(allow, name)
		}
	}
	// Always keep use_capability if somehow only that remains.
	if len(allow) == 0 {
		if _, ok := reg.Get("use_capability"); ok {
			allow = []string{"use_capability"}
		}
	}
	reg.SetProviderVisibleTools(allow)
}
