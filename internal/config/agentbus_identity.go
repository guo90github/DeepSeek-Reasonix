package config

// AgentBusIdentityConfig carries what a host calls the sessions it runs, so the roster can say
// what a participant is for. The words belong to the operator: the kernel has no role
// vocabulary and never invents one (F48, 2026-10-05).
type AgentBusIdentityConfig struct {
	Role string `toml:"role"`
}
