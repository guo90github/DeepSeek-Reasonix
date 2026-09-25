package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

const inboxDispatchTestTimeout = 15 * time.Second

// The host knows why it did not publish; a receipt that only named the gate
// would leave a sender unable to tell "wait" from "someone has to act".
func TestReceiptLookupRelaysTheHostsOwnRefusal(t *testing.T) {
	c, _, _ := newInboxDispatchController(t)
	const hostSentence = "这个会话在桌面端已经没有标签页在托管它"
	c.SetBeforeInboxDispatch(func(*Controller) (func(), error) {
		return nil, &InboxDispatchRefusal{
			Reason:    hostSentence,
			Resumable: false,
			Err:       ErrInboxRuntimeUnpublished,
		}
	})
	c.inbox.mu.Lock()
	c.inbox.scheduleDispatchRetry = func(time.Duration, func()) {}
	c.inbox.mu.Unlock()

	enqueued, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentFollowup, Submit: "wake", Idempotency: "wake-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.maybeDispatchInbox()
	waitForHostRefusal(t, c, enqueued.ItemID)

	looked, found, err := c.LookupInboxReceipt("wake-1")
	if err != nil || !found {
		t.Fatalf("lookup = %+v found=%t err=%v", looked, found, err)
	}
	if looked.Gate != sessioninbox.GateHostDispatch {
		t.Fatalf("gate = %q, want %q", looked.Gate, sessioninbox.GateHostDispatch)
	}
	if looked.GateReason != hostSentence {
		t.Fatalf("gateReason = %q, want the host's own sentence", looked.GateReason)
	}
	if looked.Resumable == nil || *looked.Resumable {
		t.Fatalf("resumable = %v, want an explicit false", looked.Resumable)
	}
	if looked.State != sessioninbox.StateQueued || looked.Position != 1 {
		t.Fatalf("lookup = %+v, want the item queued first", looked)
	}
	if looked.Retryable != nil {
		t.Fatalf("retryable = %v before the defer ladder ran out", *looked.Retryable)
	}
}

// Once the defer ladder runs out, one more wake cannot change the answer. The
// receipt has to say so, or a sender keeps re-posting the same wake forever.
func TestExhaustedHostDeferLadderReportsUnretryableWake(t *testing.T) {
	c, _, _ := newInboxDispatchController(t)
	deferTimers := make(chan func(), 8)
	c.SetBeforeInboxDispatch(func(*Controller) (func(), error) { return nil, ErrInboxRuntimeUnpublished })
	c.inbox.mu.Lock()
	c.inbox.scheduleDispatchRetry = func(_ time.Duration, retry func()) { deferTimers <- retry }
	c.inbox.mu.Unlock()

	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentFollowup, Submit: "never admitted", Idempotency: "wake-2",
	}); err != nil {
		t.Fatal(err)
	}
	c.maybeDispatchInbox()
	for i := range inboxDispatchDeferDelays {
		select {
		case retry := <-deferTimers:
			retry()
		case <-time.After(inboxDispatchTestTimeout):
			t.Fatalf("deferral %d of %d was never scheduled", i+1, len(inboxDispatchDeferDelays))
		}
	}

	looked, found, err := c.LookupInboxReceipt("wake-2")
	if err != nil || !found {
		t.Fatalf("lookup = %+v found=%t err=%v", looked, found, err)
	}
	if looked.Retryable == nil || *looked.Retryable {
		t.Fatalf("retryable = %v, want an explicit false", looked.Retryable)
	}
	if looked.RetryReason == "" {
		t.Fatalf("a stopped ladder must say why: %+v", looked)
	}
	if want := sessioninbox.GateReasonText(sessioninbox.GateHostDispatch); looked.GateReason != want {
		t.Fatalf("gateReason = %q, want the gate template %q", looked.GateReason, want)
	}
	if looked.Resumable == nil || !*looked.Resumable {
		t.Fatalf("resumable = %v, want true: the host may still publish", looked.Resumable)
	}
}

func waitForHostRefusal(t *testing.T, c *Controller, itemID string) {
	t.Helper()
	deadline := time.Now().Add(inboxDispatchTestTimeout)
	for time.Now().Before(deadline) {
		if _, _, refused := c.inboxHostRefusalFor(itemID); refused {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	failInboxDispatchWait(t, c, "the host refusal to be recorded")
}

func TestClosedControllerCannotOpenInboxFromLateDispatch(t *testing.T) {
	dir := t.TempDir()
	c := New(Options{})
	// Model the dispatcher having a persisted path but no opened sidecar yet.
	c.mu.Lock()
	c.sessionPath = filepath.Join(dir, "session.jsonl")
	c.mu.Unlock()
	c.SetBeforeInboxDispatch(func(*Controller) (func(), error) { t.Error("closed controller entered admission"); return nil, nil })
	c.Close()
	c.NotifyInboxRuntimeReady()
	if _, err := c.ensureInbox(); err == nil {
		t.Fatal("closed controller opened an inbox")
	}
	c.rebindInbox()
	c.autosaveWG.Wait()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("late dispatch created sidecars: %v %v", entries, err)
	}
}

type inboxDispatchRunner struct {
	inputs chan string
}

func (r *inboxDispatchRunner) Run(_ context.Context, input string) error {
	r.inputs <- input
	return nil
}

func newInboxDispatchController(t *testing.T) (*Controller, *inboxDispatchRunner, <-chan struct{}) {
	t.Helper()
	dir := t.TempDir()
	runner := &inboxDispatchRunner{inputs: make(chan string, 8)}
	done := make(chan struct{}, 8)
	c := New(Options{
		Runner: runner,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.TurnDone {
				done <- struct{}{}
			}
		}),
		SessionDir:  dir,
		SessionPath: filepath.Join(dir, "session.jsonl"),
	})
	t.Cleanup(func() {
		c.Close()
		c.autosaveWG.Wait()
	})
	return c, runner, done
}

func failInboxDispatchWait(t *testing.T, c *Controller, waitingFor string) {
	t.Helper()
	c.inbox.mu.Lock()
	active := c.inbox.activeIDs()
	dispatching := c.inbox.dispatching
	dispatchPending := c.inbox.dispatchPending
	c.inbox.mu.Unlock()
	sort.Strings(active)
	t.Fatalf(
		"timed out after %s waiting for %s: runtime=%+v inbox=%+v active_items=%v dispatching=%t dispatch_pending=%t",
		inboxDispatchTestTimeout,
		waitingFor,
		c.RuntimeStatus(),
		c.InboxSnapshot(),
		active,
		dispatching,
		dispatchPending,
	)
}

func waitForInboxDispatch(t *testing.T, c *Controller, runner *inboxDispatchRunner) string {
	t.Helper()
	select {
	case input := <-runner.inputs:
		return input
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "inbox dispatch")
		return ""
	}
}

func waitForInboxTurnDone(t *testing.T, c *Controller, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "inbox turn completion")
	}
}

func TestEndRotationDispatchesQueuedInboxItem(t *testing.T) {
	c, runner, done := newInboxDispatchController(t)
	if err := c.beginRotation(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.TryEnqueueFollowup(InboxRequest{
		Intent: sessioninbox.IntentFollowup,
		Submit: "queued during rotation",
	}); err != nil {
		t.Fatal(err)
	}
	c.endRotation()

	if got := waitForInboxDispatch(t, c, runner); got != "queued during rotation" {
		t.Fatalf("dispatched input = %q", got)
	}
	waitForInboxTurnDone(t, c, done)
}

func TestRejectedIdleSteerDispatchesAsFollowup(t *testing.T) {
	c, runner, done := newInboxDispatchController(t)
	rec, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentSteer,
		Submit: "late steer becomes follow-up",
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := c.TrySteerInboxItem(rec.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != sessioninbox.DispositionQueuedFollowup {
		t.Fatalf("disposition = %q", receipt.Disposition)
	}

	if got := waitForInboxDispatch(t, c, runner); got != "late steer becomes follow-up" {
		t.Fatalf("dispatched input = %q", got)
	}
	waitForInboxTurnDone(t, c, done)
}

func TestInboxDispatchKickDuringEmptyScanIsNotLost(t *testing.T) {
	c, runner, done := newInboxDispatchController(t)
	scanReached := make(chan struct{})
	releaseScan := make(chan struct{})
	var once sync.Once
	c.inbox.mu.Lock()
	c.inbox.afterDispatchScan = func(found bool) {
		if found {
			return
		}
		once.Do(func() {
			close(scanReached)
			<-releaseScan
		})
	}
	c.inbox.mu.Unlock()

	dispatchReturned := make(chan struct{})
	go func() {
		c.maybeDispatchInbox()
		close(dispatchReturned)
	}()
	select {
	case <-scanReached:
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "dispatcher empty scan")
	}
	if _, err := c.EnqueueInbox(InboxRequest{Submit: "arrived during empty scan"}); err != nil {
		t.Fatal(err)
	}
	// This kick lands while the first dispatcher still owns the handoff. The
	// pending level must make that dispatcher scan again before it exits.
	c.maybeDispatchInbox()
	close(releaseScan)

	select {
	case <-dispatchReturned:
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "dispatcher return")
	}
	if got := waitForInboxDispatch(t, c, runner); got != "arrived during empty scan" {
		t.Fatalf("dispatched input = %q", got)
	}
	waitForInboxTurnDone(t, c, done)
}

func TestInboxDispatchRetriesTransientOwnerFailure(t *testing.T) {
	c, runner, done := newInboxDispatchController(t)
	retryReady := make(chan func(), 1)
	failedOnce := false
	c.inbox.mu.Lock()
	c.inbox.beforeDispatchSubmit = func(string) error {
		if failedOnce {
			return nil
		}
		failedOnce = true
		return errors.New("temporary dispatch failure")
	}
	c.inbox.scheduleDispatchRetry = func(_ time.Duration, retry func()) {
		retryReady <- retry
	}
	c.inbox.mu.Unlock()
	if _, err := c.EnqueueInbox(InboxRequest{Submit: "retry me"}); err != nil {
		t.Fatal(err)
	}
	c.maybeDispatchInbox()

	var retry func()
	select {
	case retry = <-retryReady:
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "transient failure retry")
	}
	select {
	case got := <-runner.inputs:
		t.Fatalf("item dispatched before scheduled retry: %q", got)
	default:
	}
	retry()
	if got := waitForInboxDispatch(t, c, runner); got != "retry me" {
		t.Fatalf("retried input = %q", got)
	}
	waitForInboxTurnDone(t, c, done)
}

type gatedInboxDispatchRunner struct {
	inputs       chan string
	firstStarted chan struct{}
	releaseFirst chan struct{}
	once         sync.Once
}

func (r *gatedInboxDispatchRunner) Run(ctx context.Context, input string) error {
	r.inputs <- input
	blocked := false
	r.once.Do(func() {
		blocked = true
		close(r.firstStarted)
	})
	if !blocked {
		return nil
	}
	select {
	case <-r.releaseFirst:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestNaturalCompletionAutoDispatchesDurableFIFO(t *testing.T) {
	dir := t.TempDir()
	runner := &gatedInboxDispatchRunner{
		inputs:       make(chan string, 8),
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	done := make(chan struct{}, 8)
	c := New(Options{
		Runner: runner,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.TurnDone {
				done <- struct{}{}
			}
		}),
		SessionDir:  dir,
		SessionPath: filepath.Join(dir, "session.jsonl"),
	})
	t.Cleanup(func() {
		c.Close()
		c.autosaveWG.Wait()
	})

	c.Submit("active turn")
	select {
	case <-runner.firstStarted:
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "active turn start")
	}
	if got := <-runner.inputs; got != "active turn" {
		t.Fatalf("initial input = %q", got)
	}
	for _, input := range []string{"queued one", "queued two"} {
		if _, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentFollowup, Submit: input}); err != nil {
			t.Fatal(err)
		}
	}
	close(runner.releaseFirst)
	waitForInboxTurnDone(t, c, done)
	for _, want := range []string{"queued one", "queued two"} {
		if got := waitForInboxDispatch(t, c, &inboxDispatchRunner{inputs: runner.inputs}); got != want {
			t.Fatalf("FIFO input = %q, want %q", got, want)
		}
		waitForInboxTurnDone(t, c, done)
	}
	if snap := c.InboxSnapshot(); len(snap.Items) != 0 || snap.Paused {
		t.Fatalf("completed FIFO left inbox state: %+v", snap)
	}
}

// The host's hook answers ErrInboxRuntimeUnpublished whenever it has not
// published this runtime, and it only kicks again on publication. A wake that
// lands in that gap must be re-attempted here or it waits for a kick that may
// never come.
func TestHostUnpublishedRuntimeIsDeferredThenAdmitted(t *testing.T) {
	c, runner, done := newInboxDispatchController(t)
	deferTimers := make(chan func(), 8)
	var mu sync.Mutex
	published := false
	c.SetBeforeInboxDispatch(func(*Controller) (func(), error) {
		mu.Lock()
		defer mu.Unlock()
		if !published {
			return nil, ErrInboxRuntimeUnpublished
		}
		return nil, nil
	})
	c.inbox.mu.Lock()
	c.inbox.scheduleDispatchRetry = func(_ time.Duration, retry func()) { deferTimers <- retry }
	c.inbox.mu.Unlock()
	if _, err := c.EnqueueInbox(InboxRequest{Submit: "wake during host publication"}); err != nil {
		t.Fatal(err)
	}
	c.maybeDispatchInbox()

	var retry func()
	select {
	case retry = <-deferTimers:
	case <-time.After(inboxDispatchTestTimeout):
		failInboxDispatchWait(t, c, "host deferral timer")
	}
	select {
	case got := <-runner.inputs:
		t.Fatalf("item dispatched while the host reported its runtime unpublished: %q", got)
	default:
	}

	mu.Lock()
	published = true
	mu.Unlock()
	retry()
	if got := waitForInboxDispatch(t, c, runner); got != "wake during host publication" {
		t.Fatalf("deferred input = %q", got)
	}
	waitForInboxTurnDone(t, c, done)
}

func TestHostUnpublishedRuntimeDeferralsAreBounded(t *testing.T) {
	c, _, _ := newInboxDispatchController(t)
	deferTimers := make(chan func(), 8)
	c.SetBeforeInboxDispatch(func(*Controller) (func(), error) { return nil, ErrInboxRuntimeUnpublished })
	c.inbox.mu.Lock()
	c.inbox.scheduleDispatchRetry = func(_ time.Duration, retry func()) { deferTimers <- retry }
	c.inbox.mu.Unlock()
	if _, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentFollowup, Submit: "never admitted"}); err != nil {
		t.Fatal(err)
	}
	c.maybeDispatchInbox()
	for i := range inboxDispatchDeferDelays {
		select {
		case retry := <-deferTimers:
			retry()
		case <-time.After(inboxDispatchTestTimeout):
			t.Fatalf("deferral %d of %d was never scheduled", i+1, len(inboxDispatchDeferDelays))
		}
	}
	select {
	case <-deferTimers:
		t.Fatal("deferrals continued past the ladder")
	case <-time.After(100 * time.Millisecond):
	}
}
