package plugin

import (
	"context"

	"reasonix/internal/secrets"
)

// hostProcessEnvKey marks the host-owned values a spawn must give its stdio
// child. They ride the lifetime context rather than the Spec: the Spec feeds
// SchemaCacheKey, where a per-session input would churn the schema cache.
type hostProcessEnvKey struct{}

// withHostProcessEnv returns ctx carrying the values every stdio child spawned
// under it inherits. Empty values leave ctx unchanged.
func withHostProcessEnv(ctx context.Context, values map[string]string) context.Context {
	if ctx == nil || len(values) == 0 {
		return ctx
	}
	return context.WithValue(ctx, hostProcessEnvKey{}, values)
}

func hostProcessEnvFrom(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	values, _ := ctx.Value(hostProcessEnvKey{}).(map[string]string)
	return values
}

// childEnv composes one stdio child's environment: the process env, then the
// host-owned values, then the server's own Env, which stays the last word.
func childEnv(ctx context.Context, s Spec) []string {
	return mergeEnv(mergeEnv(secrets.ProcessEnv(), hostProcessEnvFrom(ctx)), s.Env)
}

// SetProcessEnvProvider installs the resolver a Host calls once per stdio
// spawn, so each child learns its own session-scoped serve address. Call it
// before the first connect: spawns read the field without a lock, like profile.
func (h *Host) SetProcessEnvProvider(f func() map[string]string) {
	h.processEnv = f
}

// processEnvValues resolves the host-owned values for one spawn.
func (h *Host) processEnvValues() map[string]string {
	if h == nil || h.processEnv == nil {
		return nil
	}
	return h.processEnv()
}
