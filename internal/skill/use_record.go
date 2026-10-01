package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// UseRecorder receives a fingerprint of every skill invocation, so a session can
// say which skill a turn ran and whether that skill's bytes have since changed.
// It never receives the skill body (docs/50 §2.2).
type UseRecorder interface {
	RecordSkillUse(name, contentHash string)
}

type useRecorderKey struct{}

// WithUseRecorder stamps r onto ctx for the skill tools to find, the same way
// the memory queue is carried.
func WithUseRecorder(ctx context.Context, r UseRecorder) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, useRecorderKey{}, r)
}

// UseRecorderFromContext returns the recorder stamped by the agent, if any.
func UseRecorderFromContext(ctx context.Context) (UseRecorder, bool) {
	r, ok := ctx.Value(useRecorderKey{}).(UseRecorder)
	return r, ok && r != nil
}

// SkillContentHash fingerprints what a skill actually says. The body carries the
// instructions, so a hash over the body plus the description changes whenever the
// skill's behaviour would; the hash is truncated to 16 hex characters.
func SkillContentHash(sk Skill) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sk.Description) + "\x00" + strings.TrimSpace(sk.Body)))
	return hex.EncodeToString(sum[:8])
}
