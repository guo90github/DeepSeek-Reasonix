package control

import _ "embed"

// auditSystemPromptContent is the verbatim reasoning-quality evaluator prompt
// (six failure classes + scoring formula + verdict JSON schema). It lives as a
// .md so the prompt is plain text and can be diffed/reviewed independently of
// Go source.
//
//go:embed audit_system_prompt.md
var auditSystemPromptContent string
