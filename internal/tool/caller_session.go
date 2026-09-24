package tool

import (
	"context"
	"strings"
)

type callerSessionPathKey struct{}

// WithCallerSessionPath marks which session's agent is calling a tool. One MCP
// child serves a whole plugin host, so a server that must address the session
// that asked reads the identity off each call instead of its own environment.
func WithCallerSessionPath(ctx context.Context, path string) context.Context {
	if strings.TrimSpace(path) == "" {
		return ctx
	}
	return context.WithValue(ctx, callerSessionPathKey{}, path)
}

func CallerSessionPath(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	path, _ := ctx.Value(callerSessionPathKey{}).(string)
	return strings.TrimSpace(path)
}
