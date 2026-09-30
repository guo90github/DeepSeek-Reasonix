package control

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/hook"
)

// A session restored from disk has content worth recapping but no turn in this
// process. Its close must still reach the end observer, while the user-facing
// hooks stay silent because SessionEnd pairs with a SessionStart that never ran.
func TestObserverFiresForASessionThatNeverRanHere(t *testing.T) {
	var sessionEnds atomic.Int32
	hooks := hook.NewRunner([]hook.ResolvedHook{{
		HookConfig: hook.HookConfig{Command: "session-end"},
		Event:      hook.SessionEnd,
		Scope:      hook.ScopeGlobal,
	}}, t.TempDir(), func(context.Context, hook.SpawnInput) hook.SpawnResult {
		sessionEnds.Add(1)
		return hook.SpawnResult{ExitCode: 0}
	}, nil)

	c := New(Options{Runner: &fakeTurnRunner{}, Hooks: hooks})
	sessionPath := filepath.Join(t.TempDir(), "restored.jsonl")
	c.SetSessionPath(sessionPath)

	ended := make(chan string, 4)
	c.SetSessionEndObserver(func(_, path string) { ended <- path })
	c.Close()

	select {
	case got := <-ended:
		if got != sessionPath {
			t.Fatalf("end observer saw %q, want %q", got, sessionPath)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closing a restored session never reached the end observer")
	}
	if got := sessionEnds.Load(); got != 0 {
		t.Fatalf("SessionEnd hooks fired %d times, want 0 for a session that never started", got)
	}
}
