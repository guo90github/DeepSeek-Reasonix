package main

import (
	"reasonix/internal/agent"
	"reasonix/internal/config"
)

// desktopSubagentConcurrency and desktopParallelWriters resolve the stored
// concurrency knobs for the settings view. They live here beside the ask-timeout
// surface so settings_app.go's line budget does not grow.
func desktopSubagentConcurrency(n int) int {
	total, _ := agent.NormalizeConcurrencyLimits(n, 0)
	return total
}

func desktopParallelWriters(writers, total int) int {
	_, w := agent.NormalizeConcurrencyLimits(total, writers)
	return w
}

// SetAskTimeoutMinutes sets how long a displayed question waits before the host
// answers it with the recommended option: minutes, 0 keeps the default (10), and
// a negative value waits for the user instead.
func (a *App) SetAskTimeoutMinutes(v float64) error {
	return a.applyModelConfigChange(func(c *config.Config) error {
		return setAskTimeoutMinutesConfig(c, v)
	})
}
