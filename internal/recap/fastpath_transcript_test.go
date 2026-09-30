package recap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

func savedSession(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "20260101-000000.000000000-test-model.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "please recap the thing"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "done: the thing is done"})
	if err := session.Save(path); err != nil {
		t.Fatalf("save session: %v", err)
	}
	return path
}

// The fast path is allowed only on the sidecar digest's proof, so a file the
// digest does not describe must keep the replay.
func TestTranscriptFileNeedsDigestProof(t *testing.T) {
	ctx := context.Background()
	path := savedSession(t, t.TempDir())

	text, ok := transcriptFileOrNothing(path)
	if !ok {
		t.Fatal("a freshly saved session should carry the digest that proves its file")
	}
	authoritative, err := FileTranscript{}.ReadAuthoritative(ctx, path)
	if err != nil {
		t.Fatalf("authoritative: %v", err)
	}
	if text != authoritative {
		t.Fatalf("fast path disagrees with the replay:\nfast: %q\nauth: %q", text, authoritative)
	}
	if read, err := (FileTranscript{}).Read(ctx, path); err != nil || read != authoritative {
		t.Fatalf("Read = %q, %v", read, err)
	}

	// An appended row changes the content without changing the recorded digest.
	appended := appendRow(t, path, provider.Message{Role: provider.RoleUser, Content: "unrecorded"})
	if appended == "" {
		t.Fatal("append failed")
	}
	if _, ok := transcriptFileOrNothing(path); ok {
		t.Fatal("a file whose rows no longer match the recorded digest must fall back")
	}

	// A pinned context revision is the one row kind the replay may turn into
	// content the file does not show.
	pinned := savedSession(t, t.TempDir())
	revision := provider.Message{
		Role:    provider.RoleUser,
		Origin:  provider.MessageOriginHost,
		Content: `<pinned_context_revision version="1"></pinned_context_revision>`,
	}
	appendRow(t, pinned, revision)
	if _, ok := transcriptFileOrNothing(pinned); ok {
		t.Fatal("a pinned context revision must keep the replay")
	}
	fallback, err := (FileTranscript{}).Read(ctx, pinned)
	if err != nil {
		t.Fatalf("Read with a pinned revision: %v", err)
	}
	if !strings.Contains(fallback, "please recap the thing") {
		t.Fatalf("fallback lost the transcript: %q", fallback)
	}

	// A row that does not parse keeps the replay as well.
	broken := savedSession(t, t.TempDir())
	f, err := os.OpenFile(broken, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, ok := transcriptFileOrNothing(broken); ok {
		t.Fatal("an unparsable file must fall back")
	}
}

func appendRow(t *testing.T, path string, m provider.Message) string {
	t.Helper()
	row, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(row, '\n')); err != nil {
		t.Fatalf("append: %v", err)
	}
	return path
}
