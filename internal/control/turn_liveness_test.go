package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// A running turn that produces no events for the stall threshold gets exactly
// one warning per silent stretch; any progress re-arms it.
func TestStalledTurnWarnsOncePerSilence(t *testing.T) {
	oldInterval, oldThreshold := midTurnSnapshotInterval.Load(), turnStallThreshold.Load()
	midTurnSnapshotInterval.Store(int64(10 * time.Millisecond))
	turnStallThreshold.Store(int64(80 * time.Millisecond))
	t.Cleanup(func() {
		midTurnSnapshotInterval.Store(oldInterval)
		turnStallThreshold.Store(oldThreshold)
	})

	notices := make(chan event.Event, 8)
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice && e.Code == event.NoticeCodeTurnStalled {
			notices <- e
		}
	})})
	t.Cleanup(c.Close)

	started := make(chan struct{})
	c.runGuarded(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started

	select {
	case n := <-notices:
		if n.Level != event.LevelWarn || n.Text == "" {
			t.Fatalf("stall notice = %+v, want a warn-level explanation", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("silent running turn never produced a stall notice")
	}
	select {
	case <-notices:
		t.Fatal("stall notice repeated without any progress")
	case <-time.After(300 * time.Millisecond):
	}

	c.sink.Emit(event.Event{Kind: event.Text, Text: "still working"})
	select {
	case <-notices:
	case <-time.After(5 * time.Second):
		t.Fatal("renewed silence after progress did not warn again")
	}
	c.Cancel()
}

func TestIdleControllerNeverWarnsAboutStalls(t *testing.T) {
	notices := 0
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice && e.Code == event.NoticeCodeTurnStalled {
			notices++
		}
	})})
	t.Cleanup(c.Close)
	c.liveness.reset(time.Now().Add(-time.Hour))
	c.warnIfTurnStalled(time.Now())
	if notices != 0 {
		t.Fatalf("idle controller emitted %d stall notices", notices)
	}
}

// The driver outside this package reads two things: how silent the running turn
// is, and whether the last turn already died on the window. Reading them adds no
// event of its own.
func TestTurnSilenceAndContextExhaustionAreReadableFromOutside(t *testing.T) {
	oldThreshold := turnStallThreshold.Load()
	turnStallThreshold.Store(int64(time.Minute))
	t.Cleanup(func() { turnStallThreshold.Store(oldThreshold) })

	stalled := 0
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice && e.Code == event.NoticeCodeTurnStalled {
			stalled++
		}
	})})
	t.Cleanup(c.Close)

	if _, ok := c.TurnSilence(time.Now()); ok {
		t.Fatal("an idle controller must not report a running turn")
	}
	if c.ContextExhausted() {
		t.Fatal("a fresh controller must not report a spent window")
	}
	if got := TurnStallThreshold(); got != time.Minute {
		t.Fatalf("TurnStallThreshold() = %s, want the stored bound", got)
	}

	started := make(chan struct{})
	c.runGuarded(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started

	silence, ok := c.TurnSilence(time.Now())
	if !ok {
		t.Fatal("a running turn must report its silence")
	}
	if silence > time.Minute {
		t.Fatalf("silence = %s, want a turn that just started", silence)
	}
	if _, ok := c.TurnSilence(time.Now().Add(2 * time.Minute)); !ok {
		t.Fatal("silence must stay readable while the turn runs")
	}
	c.Cancel()
	if stalled != 0 {
		t.Fatalf("reading the liveness produced %d stall notices, want none", stalled)
	}
}

// A turn that ended on the provider's window must be reported as exhausted by
// the very path that finishes a turn; a new turn clears it.
func TestTurnThatDiedOnTheWindowIsReportedAsExhausted(t *testing.T) {
	c := New(Options{})
	t.Cleanup(c.Close)

	started := make(chan struct{})
	c.runGuarded(func(context.Context) error {
		close(started)
		return contextLimitFailure()
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the turn body never started")
	}
	if !waitFor(time.Second, c.ContextExhausted) {
		t.Fatal("a turn that ended on the provider's window was not reported")
	}

	resumed := make(chan struct{})
	c.runGuarded(func(ctx context.Context) error {
		close(resumed)
		<-ctx.Done()
		return ctx.Err()
	})
	<-resumed
	if c.ContextExhausted() {
		t.Fatal("a new turn must clear the exhausted signal")
	}
	c.Cancel()
}

func TestOrdinaryTurnFailureIsNotAContextExhaustion(t *testing.T) {
	c := New(Options{})
	t.Cleanup(c.Close)

	started := make(chan struct{})
	c.runGuarded(func(context.Context) error {
		close(started)
		return errors.New("boom")
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the turn body never started")
	}
	if !waitFor(time.Second, func() bool { _, running := c.TurnSilence(time.Now()); return !running }) {
		t.Fatal("the failed turn never finished")
	}
	if c.ContextExhausted() {
		t.Fatal("an ordinary failure must not mark the window as spent")
	}
}

// contextLimitFailure is a provider overflow error shaped the way the parser
// builds one: the typed cause always carries the HTTP error it came from.
func contextLimitFailure() error {
	return &provider.ContextLimitError{
		APIError: &provider.APIError{Status: 400, Body: "maximum context length is 65536 tokens"},
	}
}

func waitFor(timeout time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return ok()
}
