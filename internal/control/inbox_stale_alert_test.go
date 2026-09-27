package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/sessioninbox"
)

// staleAlertController builds a controller whose stall timer never fires on its
// own, so a test decides when the check runs.
func staleAlertController(t *testing.T, notices *[]event.Event) *Controller {
	t.Helper()
	restore := inboxStaleAlertDelay
	inboxStaleAlertDelay = 0
	t.Cleanup(func() { inboxStaleAlertDelay = restore })

	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{
		SessionPath: session, SessionDir: dir,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice {
				*notices = append(*notices, e)
			}
		}),
	})
	t.Cleanup(c.Close)
	c.inbox.mu.Lock()
	c.inbox.scheduleStaleAlert = func(time.Duration, func()) {}
	c.inbox.mu.Unlock()
	return c
}

func enqueueStaleLine(t *testing.T, c *Controller) {
	t.Helper()
	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentFollowup, Submit: "还在等", Idempotency: "stale-1",
	}); err != nil {
		t.Fatal(err)
	}
}

// isStaleAlarm matches either alarm by its template's fixed prefix, so the test
// does not depend on the language the catalogue is in.
func isStaleAlarm(e event.Event) bool {
	for _, tmpl := range []string{i18n.M.InboxStalePausedFmt, i18n.M.InboxStaleStartFailedFmt} {
		if prefix, _, _ := strings.Cut(tmpl, "%"); prefix != "" && strings.HasPrefix(e.Text, prefix) {
			return true
		}
	}
	return false
}

func staleNoticeOf(t *testing.T, notices []event.Event) event.Event {
	t.Helper()
	for _, e := range notices {
		if isStaleAlarm(e) {
			return e
		}
	}
	t.Fatalf("notices = %+v, want the queue-stall alarm", notices)
	return event.Event{}
}

// The 2026-09-27 queue sat for five hours with nobody told. A line that waits
// past the alert delay now says so, and says which of the two situations it is:
// a paused queue is cleared by pressing resume, a failed start is not.
func TestStaleQueueAlertNamesWhichOfTheTwoItIs(t *testing.T) {
	var paused []event.Event
	c := staleAlertController(t, &paused)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	enqueueStaleLine(t, c)
	c.checkStaleQueue()

	pausedNotice := staleNoticeOf(t, paused)
	if pausedNotice.Level != event.LevelWarn {
		t.Fatalf("level = %v, want a warning", pausedNotice.Level)
	}
	if !strings.Contains(pausedNotice.Detail, string(sessioninbox.GatePaused)) {
		t.Fatalf("detail = %q, want the gate named", pausedNotice.Detail)
	}

	var failed []event.Event
	c2 := staleAlertController(t, &failed)
	const cause = "context exceeds provider limit and compaction failed"
	c2.noteInboxStartFailure(errors.New(cause))
	enqueueStaleLine(t, c2)
	c2.checkStaleQueue()

	failedNotice := staleNoticeOf(t, failed)
	if !strings.Contains(failedNotice.Text, cause) {
		t.Fatalf("text = %q, want the failure a reader can act on", failedNotice.Text)
	}
	if pausedNotice.Text == failedNotice.Text {
		t.Fatal("the two stalls share one sentence; the human action differs")
	}
	if !strings.Contains(failedNotice.Detail, string(sessioninbox.GateStartFailed)) {
		t.Fatalf("detail = %q, want the start-failed gate", failedNotice.Detail)
	}
}

// One line gets one sentence: a session that stays stuck must not repeat itself
// every alert interval.
func TestStaleQueueAlertSpeaksOncePerLine(t *testing.T) {
	var notices []event.Event
	c := staleAlertController(t, &notices)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	enqueueStaleLine(t, c)

	for range 3 {
		c.checkStaleQueue()
	}
	count := 0
	for _, e := range notices {
		if isStaleAlarm(e) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("alarms = %d after three checks, want one per waiting line", count)
	}
}
