package tool

import "context"

type offPathLaunchKey struct{}

// WithOffPathLaunch stamps one call's execution context with the host's
// off-path decision: start this call so the turn does not wait for it. The
// decision travels here rather than in the call's arguments, so the transcript
// keeps exactly what the model wrote while bash still takes its job branch.
func WithOffPathLaunch(ctx context.Context) context.Context {
	return context.WithValue(ctx, offPathLaunchKey{}, true)
}

// OffPathLaunchFrom reports whether the host moved this call off the critical
// path. False for a plain context: headless runs and direct tool calls never
// promote, and the model's own run_in_background stays its own decision.
func OffPathLaunchFrom(ctx context.Context) bool {
	off, _ := ctx.Value(offPathLaunchKey{}).(bool)
	return off
}
