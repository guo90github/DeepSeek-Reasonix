package config

import "strings"

// ResponseLanguage normalizes the top-level language preference for final
// answers. Empty means auto: replies follow the current user turn.
func (c *Config) ResponseLanguage() string {
	if c == nil {
		return "auto"
	}
	return NormalizeLanguage(c.Language)
}

// NormalizeLanguage returns one of auto|zh|en for UI/default reply language settings.
func NormalizeLanguage(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "", "auto", "detect", "default":
		return "auto"
	case "zh", "cn", "chinese", "中文":
		return "zh"
	case "en", "english":
		return "en"
	default:
		return "auto"
	}
}
