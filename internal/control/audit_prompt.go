package control

import _ "embed"

// auditSystemPromptContent is the verbatim reasoning-quality evaluator prompt
// (six failure classes + scoring formula + verdict JSON schema) for one turn.
// It lives as a .md so the prompt is plain text and can be diffed/reviewed
// independently of Go source.
//
//go:embed audit_system_prompt.md
var auditSystemPromptContent string

// auditSegmentPromptContent scores a batch of consecutive turns in one call.
// It reuses the single-turn class definitions and formula verbatim so a
// per-turn score in a session audit stays comparable with the single-turn
// audit, and adds the conclusion/prior_conflict fields the cross-turn pass
// consumes.
//
//go:embed audit_segment_prompt.md
var auditSegmentPromptContent string

// auditSessionPromptContent is the cross-turn pass: it reviews the per-turn
// results (never the reasoning text) for contradictions, drift, repeated dead
// ends, unmet commitments, and error propagation, then scores the session.
//
//go:embed audit_session_prompt.md
var auditSessionPromptContent string
