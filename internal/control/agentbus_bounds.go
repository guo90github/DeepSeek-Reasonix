package control

import (
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/config"
)

// AgentBusTalkLimits maps the operator's talk knobs onto the kernel's bounds. Zero stays zero:
// a host nobody configured runs talk unbounded, which is what the kernel's own defaults say.
// The rate window only carries that knob's per-minute meaning, so it appears with the knob.
func AgentBusTalkLimits(cfg config.AgentBusConfig) agentbus.TalkLimits {
	limits := agentbus.TalkLimits{
		MaxRounds:     cfg.Talk.MaxRounds,
		SilenceWindow: time.Duration(cfg.Talk.SilenceMinutes) * time.Minute,
		RateMax:       cfg.Talk.RatePerMinute,
		AskTTL:        time.Duration(cfg.Talk.AskTTLMinutes) * time.Minute,
		MaxHop:        cfg.Talk.MaxHop,
	}
	if limits.RateMax > 0 {
		limits.RateWindow = time.Minute
	}
	return limits
}
