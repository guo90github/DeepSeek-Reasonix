package config

// AgentBusTalkConfig bounds free talk (AGENT_BUS §3.2). Zero leaves that bound off, and zero is
// what an absent section means: a host nobody configured runs talk unbounded.
//
// SilenceMinutes is what makes a lapsed topic closable at all. With no window, silence is never
// counted, so no topic closes itself and the closure's own reason never appears — which is why
// the kernel treats a zero window as "no clock" rather than "close at once".
type AgentBusTalkConfig struct {
	SilenceMinutes int `toml:"silence_minutes"`
	// MaxRounds is how many times each participant in a chain may be asked.
	MaxRounds int `toml:"max_rounds"`
	// RatePerMinute caps how many lines one topic may take in a minute; zero leaves it off.
	RatePerMinute int `toml:"rate_per_minute"`
	// AskTTLMinutes is how long a question stays answerable before the chain refuses.
	AskTTLMinutes int `toml:"ask_ttl_minutes"`
	// MaxHop caps how many turns one correlation may take (ask, answer, ask, ...).
	MaxHop int `toml:"max_hop"`
}
