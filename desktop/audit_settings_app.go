package main

import "reasonix/internal/config"

// The audit settings are user-triggered configuration only; they do not rebuild
// the controller (the audit model is resolved lazily at AuditTurn time, and the
// input ceiling is read from the controller built for the tab). All setters
// persist through the shared config-edit path.

// SetAuditModel configures the standalone model used for manual reasoning audits.
func (a *App) SetAuditModel(name string) error {
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetAuditModel(name) })
}

// SetAuditThreshold sets the quality score below which an audit result is
// flagged for attention.
func (a *App) SetAuditThreshold(threshold float64) error {
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetAuditThreshold(threshold) })
}

// SetAuditEffort sets the reasoning depth the audit model uses when scoring
// (off|low|medium|high); empty means auto/provider default.
func (a *App) SetAuditEffort(effort string) error {
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetAuditEffort(effort) })
}

// SetAuditMaxChars caps the audited reasoning excerpt in runes; 0 means the
// built-in default.
func (a *App) SetAuditMaxChars(maxChars int) error {
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetAuditMaxChars(maxChars) })
}

// GetAuditModel returns the configured audit model ref ("" when unset).
func (a *App) GetAuditModel() string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	return cfg.Agent.AuditModel
}

// GetAuditThreshold returns the audit attention threshold.
func (a *App) GetAuditThreshold() float64 {
	cfg, err := config.Load()
	if err != nil || cfg.Agent.AuditThreshold <= 0 {
		return defaultAuditThreshold
	}
	return cfg.Agent.AuditThreshold
}

// GetAuditEffort returns the audit model's reasoning depth ("" = auto).
func (a *App) GetAuditEffort() string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	return cfg.Agent.AuditEffort
}

// GetAuditMaxChars returns the audited reasoning cap in runes (default when unset).
func (a *App) GetAuditMaxChars() int {
	cfg, err := config.Load()
	if err != nil || cfg.Agent.AuditMaxChars <= 0 {
		return defaultAuditMaxChars
	}
	return cfg.Agent.AuditMaxChars
}
