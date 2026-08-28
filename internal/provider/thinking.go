package provider

import "strings"

// ThinkingDisabled reports whether the caller explicitly turned thinking off
// for this one request (EffortOverride "disabled"). Adapters map it onto their
// endpoint's reasoning-off knob; the session never sets it, so the session's
// thinking behavior and provider-visible prefix stay byte-identical.
func (r Request) ThinkingDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(r.EffortOverride), "disabled")
}
