package recap

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/provider"
)

// DefaultPath is the disposable recap projection under the cache root. An
// unresolvable cache root returns "" so callers fall back to memory.
func DefaultPath() string {
	dir := strings.TrimSpace(config.CacheDir())
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "session-recap", "v1.sqlite")
}

// FileTranscript renders a session's authoritative transcript for the recap
// prompt. It keeps the read_session privacy posture: no system prompts, no
// reasoning, bounded messages, and tool results omitted.
type FileTranscript struct{}

// Read renders one session.
func (FileTranscript) Read(_ context.Context, sessionPath string) (string, error) {
	ses, err := agent.LoadSession(sessionPath)
	if err != nil {
		return "", err
	}
	msgs := ses.Snapshot()
	var b strings.Builder
	turn := 0
	for _, m := range msgs {
		if agent.IsPinnedContextRevision(m) {
			continue
		}
		switch m.Role {
		case provider.RoleUser:
			turn++
			fmt.Fprintf(&b, "## User (turn %d)\n%s\n\n", turn, clipRunes(m.Content, 2000))
		case provider.RoleAssistant:
			label := turn
			if label < 1 {
				label = 1
			}
			if m.Content != "" {
				fmt.Fprintf(&b, "## Assistant (turn %d)\n%s\n\n", label, clipRunes(m.Content, 2000))
			}
			for _, call := range m.ToolCalls {
				fmt.Fprintf(&b, "- tool %s(%s)\n", call.Name, clipRunes(string(call.Arguments), 400))
			}
			if len(m.ToolCalls) > 0 {
				b.WriteString("\n")
			}
		}
	}
	return b.String(), nil
}

func clipRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
