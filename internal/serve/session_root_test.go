package serve

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

func rootedServeController(t *testing.T, root, sessionPath string) (*control.Controller, *sessionTagSink) {
	t.Helper()
	saveServeTestSession(t, sessionPath)
	bc := NewBroadcaster()
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, bc)
	ctrl := control.New(control.Options{
		Executor:      exec,
		Sink:          bc,
		SessionDir:    filepath.Dir(sessionPath),
		SessionPath:   sessionPath,
		WorkspaceRoot: root,
	})
	t.Cleanup(ctrl.Close)
	tag := NewSessionTagSink(bc)
	tag.SetPath(sessionPath)
	return ctrl, tag
}

// TestSessionPathForRootAnswersOnlyUnambiguousRoots pins the rule an addressed
// remote wake depends on: a caller that needs precision is never handed a guess.
func TestSessionPathForRootAnswersOnlyUnambiguousRoots(t *testing.T) {
	dir := t.TempDir()
	rootA, rootB := filepath.Join(dir, "alpha"), filepath.Join(dir, "beta")

	first, firstTag := rootedServeController(t, rootA, filepath.Join(dir, "a-first.jsonl"))
	srv := New(first, NewBroadcaster(), config.ServeConfig{})
	srv.RegisterSessionTag(first, firstTag)

	if got := srv.SessionPathForRoot(rootB); got != "" {
		t.Fatalf("root without a session = %q, want empty", got)
	}
	if got := srv.SessionPathForRoot(""); got != "" {
		t.Fatalf("empty root = %q, want empty", got)
	}
	want := agent.CanonicalSessionPath(first.SessionPath())
	if got := srv.SessionPathForRoot(rootA); got != want {
		t.Fatalf("single session for root = %q, want %q", got, want)
	}
	if got := srv.SessionPathForRoot(filepath.Join(rootA, ".")); got != want {
		t.Fatalf("unfolded root spelling = %q, want %q", got, want)
	}
	if runtime.GOOS == "windows" {
		if got := srv.SessionPathForRoot(strings.ToUpper(rootA)); got != want {
			t.Fatalf("Windows root spelling = %q, want %q", got, want)
		}
	}

	second, secondTag := rootedServeController(t, rootA, filepath.Join(dir, "a-second.jsonl"))
	srv.RegisterSessionTag(second, secondTag)
	if got := srv.SessionPathForRoot(rootA); got != "" {
		t.Fatalf("two sessions for one root = %q, want empty (ambiguous)", got)
	}

	third, thirdTag := rootedServeController(t, rootB, filepath.Join(dir, "b-first.jsonl"))
	srv.RegisterSessionTag(third, thirdTag)
	if got := srv.SessionPathForRoot(rootB); got != agent.CanonicalSessionPath(third.SessionPath()) {
		t.Fatalf("the other root must still answer, got %q", got)
	}
}
