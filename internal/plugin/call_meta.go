package plugin

import (
	"context"
	"strings"

	"reasonix/internal/tool"
)

// callerSessionMetaKey is the request _meta a stdio server reads to learn which
// session is calling it: one host serves every tab of a workspace root, so the
// spawned child's own environment cannot name the caller.
const callerSessionMetaKey = "reasonix/sessionPath"

// callerSessionPath is the caller identity a call and its wake both route by.
func callerSessionPath(ctx context.Context) string {
	return strings.TrimSpace(tool.CallerSessionPath(ctx))
}

// callerSessionMeta is the per-call _meta for a known caller. A call with no
// caller stays untouched: the field is not a placeholder.
func callerSessionMeta(path string) map[string]any {
	return map[string]any{callerSessionMetaKey: path}
}

// noteCallerSession remembers the session that most recently called this server.
// A wake arrives with no request to attribute it to, so this is the only
// evidence a shared host has for where the wake belongs.
func (c *Client) noteCallerSession(path string) {
	if c == nil || path == "" {
		return
	}
	c.lastCallerSession.Store(&path)
}

// wakeCaller is the session a wake from this server belongs to, empty while the
// server has not been called. Empty must stay empty: a host serving several
// sessions cannot guess one, and a wrong inbox is worse than a visible drop.
func (c *Client) wakeCaller() string {
	if c == nil {
		return ""
	}
	if path := c.lastCallerSession.Load(); path != nil {
		return *path
	}
	return ""
}
