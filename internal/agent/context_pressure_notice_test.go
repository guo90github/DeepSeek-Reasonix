package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/tool"
)

// noticesOf collects the user-visible notices an agent emits.
func noticesOf(a *Agent) *[]event.Event {
	var out []event.Event
	a.svc.sink = event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice {
			out = append(out, e)
		}
	})
	return &out
}

// A compaction failure used to reach the log and nothing else, so a session that
// had stopped starting turns still looked idle. It has to reach the surface, at a
// level that means something, with the way out in the sentence.
func TestFailedCompactionIsVisibleWithAnAction(t *testing.T) {
	sess := foldableSessionOverForce(6)
	a := agentOverForce(t, &fakeProvider{streamErr: errors.New("provider down")}, sess)
	notices := noticesOf(a)

	if err := prepareContext(context.Background(), a, CompactionTriggerOverflow); err != nil {
		t.Fatalf("prepare = %v", err)
	}
	var found bool
	for _, e := range *notices {
		if e.Text != i18n.M.ContextCompactionFailed {
			continue
		}
		found = true
		if e.Level != event.LevelWarn {
			t.Fatalf("level = %v, want a warning", e.Level)
		}
		if !strings.Contains(e.Detail, "err_type=provider down") {
			t.Fatalf("detail = %q, want the cause the user can search for", e.Detail)
		}
	}
	if !found {
		t.Fatalf("notices = %+v, want the compaction failure on screen", *notices)
	}
	if !strings.Contains(i18n.M.ContextCompactionFailed, "new session") {
		t.Fatalf("message %q names no action", i18n.M.ContextCompactionFailed)
	}
}

// Advice that repeats every round is noise. The drift prompt fires once, when
// the session first crosses the threshold — and never before it.
func TestNearingWindowPromptFiresOnceWithTheWayOut(t *testing.T) {
	const window = 30_000
	sess := windowStrictSession(78_000)
	prov := &windowStrictProvider{window: window}
	a := New(prov, tool.NewRegistry(), sess, Options{
		ContextWindow: window, CompactRatio: 0.8, RecentKeep: 2, ArchiveDir: t.TempDir(),
	}, event.Discard)
	est := a.estimatedVisibleRequestTokens(sess.Snapshot())
	if ratio := float64(est) / float64(window); ratio <= nearingWindowRatio || est >= a.compactTrigger() {
		t.Fatalf("fixture sits at %.2f of the window (%d tokens); the prompt is not exercised", ratio, est)
	}
	notices := noticesOf(a)

	for range 2 {
		if _, err := a.contextManager().Prepare(context.Background(), ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
			t.Fatalf("prepare = %v", err)
		}
	}
	drift := 0
	for _, e := range *notices {
		if e.Text == i18n.M.ContextNearingWindow {
			drift++
			if !strings.Contains(e.Detail, "window=30000") {
				t.Fatalf("detail = %q, want the numbers behind the prompt", e.Detail)
			}
		}
	}
	if drift != 1 {
		t.Fatalf("drift prompts = %d after two rounds, want exactly one", drift)
	}
	if !strings.Contains(i18n.M.ContextNearingWindow, "new session") {
		t.Fatalf("message %q names no action", i18n.M.ContextNearingWindow)
	}
}
