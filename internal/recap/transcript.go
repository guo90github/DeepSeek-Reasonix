package recap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// FileTranscript renders a session's transcript for the recap prompt. It keeps
// the read_session privacy posture: no system prompts, no reasoning, bounded
// messages, and tool results omitted.
type FileTranscript struct{}

// Read renders one session, preferring the transcript file when its recorded
// digest proves the file still describes the session: a full replay costs ~500 ms
// on a multi-megabyte session, against ~50 ms for the file.
func (FileTranscript) Read(ctx context.Context, sessionPath string) (string, error) {
	if text, ok := transcriptFileOrNothing(sessionPath); ok {
		return text, nil
	}
	return FileTranscript{}.ReadAuthoritative(ctx, sessionPath)
}

// ReadAuthoritative renders through the session loader: the replay owns branch,
// undo and pinned-context-revision semantics.
func (FileTranscript) ReadAuthoritative(_ context.Context, sessionPath string) (string, error) {
	ses, err := agent.LoadSession(sessionPath)
	if err != nil {
		return "", err
	}
	return renderMessages(ses.Snapshot()), nil
}

// transcriptFileOrNothing renders the compatibility transcript directly, but
// only on positive proof that the file is the session: its rows must hash to the
// digest the session sidecar recorded. A missing, empty, unparsable or
// digest-less file, and any file carrying a pinned context revision (which the
// replay may fold into state the file does not show), reports ok=false so the
// caller keeps the replay.
func transcriptFileOrNothing(sessionPath string) (string, bool) {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	key := fastReadKey(path, info)
	if fastReadUnproven(key) {
		return "", false
	}
	// The sidecar digest costs one small read and comes before the parse: with no
	// digest there is no proof to be had, so parsing could only waste time.
	digest, recorded, err := agent.SessionContentDigest(path)
	if err != nil || !recorded {
		return "", false
	}
	msgs, err := readTranscriptRows(path)
	if err != nil || len(msgs) == 0 {
		markFastReadUnproven(key)
		return "", false
	}
	if agent.TranscriptDigest(msgs) != digest {
		markFastReadUnproven(key)
		return "", false
	}
	return renderMessages(msgs), true
}

func readTranscriptRows(path string) ([]provider.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	msgs := make([]provider.Message, 0, 128)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m provider.Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			return nil, err
		}
		if agent.IsPinnedContextRevision(m) {
			return nil, errPinnedRevisionInFile
		}
		msgs = append(msgs, m)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// errPinnedRevisionInFile reports the one row kind that keeps the replay.
var errPinnedRevisionInFile = fmt.Errorf("recap: transcript file holds a pinned context revision")

// renderMessages is the single rendering policy, shared by both sources so they
// cannot drift apart.
func renderMessages(msgs []provider.Message) string {
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
	return b.String()
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
