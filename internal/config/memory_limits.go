package config

// MemoryFactCaps returns the user-global [memory] fact caps (0 when unset or
// unreadable). It decodes only that table: a typed field on Config would push
// ratcheted config.go/render.go over their repolint budgets, and the caps are
// consumed by internal/memory alone. A settings-UI save may drop the section.
func MemoryFactCaps() (project, global int) {
	path := userConfigLoadPath()
	if path == "" {
		return 0, 0
	}
	var partial struct {
		Memory struct {
			MaxProjectFacts int `toml:"max_project_facts"`
			MaxGlobalFacts  int `toml:"max_global_facts"`
		} `toml:"memory"`
	}
	if _, err := decodeTOMLFile(path, &partial); err != nil {
		return 0, 0
	}
	return partial.Memory.MaxProjectFacts, partial.Memory.MaxGlobalFacts
}

// MemoryAutoRecallEnabled reports whether a turn injects recalled facts by itself.
// Unset means on: turning it off makes retrieval on demand — pollution-free by
// construction, but a fact then reaches the model only when the model asks.
func MemoryAutoRecallEnabled() bool {
	path := userConfigLoadPath()
	if path == "" {
		return true
	}
	var partial struct {
		Memory struct {
			AutoRecall *bool `toml:"auto_recall"`
		} `toml:"memory"`
	}
	if _, err := decodeTOMLFile(path, &partial); err != nil {
		return true
	}
	return partial.Memory.AutoRecall == nil || *partial.Memory.AutoRecall
}
