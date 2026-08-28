package openai

import (
	"maps"

	"reasonix/internal/provider"
)

// thinkingOffExtraBody forces enable_thinking=false into the wire body when
// the request turns thinking off. The field must beat any extra_body value the
// provider configured, so the copy wins in chatRequest.MarshalJSON (ExtraBody
// merges last). The caller has already matched the DashScope/MaaS endpoint.
func (c *client) thinkingOffExtraBody(extra map[string]any, req provider.Request) map[string]any {
	if !req.ThinkingDisabled() {
		return extra
	}
	eb := make(map[string]any, len(extra)+1)
	maps.Copy(eb, extra)
	eb["enable_thinking"] = false
	return eb
}
