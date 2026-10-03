package recap

import (
	"bytes"
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

// transcriptFileRows decodes the compatibility transcript directly, but only on
// positive proof that the file is the session: its rows must hash to the digest
// the session sidecar recorded. A missing, empty, unparsable or digest-less
// file, and any file carrying a pinned context revision (which the replay may
// fold into state the file does not show), reports ok=false so the caller keeps
// the replay. covered counts the bytes of the rows it decoded, and content is the
// file as read, so a caller may record what it has covered.
func transcriptFileRows(sessionPath string) (rows []provider.Message, covered int64, content []byte, ok bool) {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return nil, 0, nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, nil, false
	}
	key := fastReadKey(path, info)
	if fastReadUnproven(key) {
		return nil, 0, nil, false
	}
	// The sidecar digest costs one small read and comes before the parse: with no
	// digest there is no proof to be had, so parsing could only waste time.
	digest, recorded, err := agent.SessionContentDigest(path)
	if err != nil || !recorded {
		return nil, 0, nil, false
	}
	content, err = os.ReadFile(path)
	if err != nil {
		return nil, 0, nil, false
	}
	msgs, covered, err := parseTranscriptRows(content)
	if err != nil || len(msgs) == 0 {
		markFastReadUnproven(key)
		return nil, 0, nil, false
	}
	if agent.TranscriptDigest(msgs) != digest {
		markFastReadUnproven(key)
		return nil, 0, nil, false
	}
	return msgs, covered, content, true
}

// transcriptFileOrNothing is the rendered form of transcriptFileRows.
func transcriptFileOrNothing(sessionPath string) (string, bool) {
	rows, _, _, ok := transcriptFileRows(sessionPath)
	if !ok {
		return "", false
	}
	return renderMessages(rows), true
}

// parseTranscriptRows decodes the rows of a transcript file and reports how many
// bytes its complete rows cover. A partial tail line is left out of both, so the
// next read starts on a line boundary.
func parseTranscriptRows(content []byte) ([]provider.Message, int64, error) {
	msgs := make([]provider.Message, 0, 128)
	var covered int64
	for len(content) > 0 {
		nl := bytes.IndexByte(content, '\n')
		if nl < 0 {
			break
		}
		line := bytes.TrimSpace(content[:nl])
		content = content[nl+1:]
		covered += int64(nl) + 1
		if len(line) == 0 {
			continue
		}
		var m provider.Message
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, 0, err
		}
		if agent.IsPinnedContextRevision(m) {
			return nil, 0, errPinnedRevisionInFile
		}
		msgs = append(msgs, m)
	}
	return msgs, covered, nil
}

// errPinnedRevisionInFile reports the one row kind that keeps the replay.
var errPinnedRevisionInFile = fmt.Errorf("recap: transcript file holds a pinned context revision")

// renderMessages is the single rendering policy, shared by both sources so they
// cannot drift apart.
func renderMessages(msgs []provider.Message) string {
	return renderMessagesFrom(msgs, 0)
}

// renderMessagesFrom numbers turns from turnBase, so a reader that renders only
// the part a previous read did not cover keeps one numbering across the session.
func renderMessagesFrom(msgs []provider.Message, turnBase int) string {
	var b strings.Builder
	turn := turnBase
	for _, m := range msgs {
		if agent.IsPinnedContextRevision(m) {
			continue
		}
		switch m.Role {
		case provider.RoleUser:
			turn++
			fmt.Fprintf(&b, "## User (turn %d)\n%s\n\n", turn, clipRunes(m.Content, 2000))
		case provider.RoleAssistant:
			label := max(turn, 1)
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
