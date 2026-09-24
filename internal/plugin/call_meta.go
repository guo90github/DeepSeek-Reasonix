package plugin

import (
	"context"

	"reasonix/internal/tool"
)

// callerSessionMetaKey is the request _meta a stdio server reads to learn which
// session is calling it: one host serves every tab of a workspace root, so the
// spawned child's own environment cannot name the caller.
const callerSessionMetaKey = "reasonix/sessionPath"

func callerSessionMeta(ctx context.Context) map[string]any {
	path := tool.CallerSessionPath(ctx)
	if path == "" {
		return nil
	}
	return map[string]any{callerSessionMetaKey: path}
}
