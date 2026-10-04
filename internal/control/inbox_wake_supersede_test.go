package control

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/sessioninbox"
)

// A generic wake names the whole work set it was derived from and the injector re-derives that set
// at turn end, so copies of it fold into one: stacked items can only deliver stale lists or
// retractions (2026-10-05: one drill left 11 queued on a single session).
func TestASecondGenericWakeIsFoldedIntoOne(t *testing.T) {
	st, err := sessioninbox.Open(filepath.Join(t.TempDir(), "s.jsonl"), sessioninbox.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	enqueue := func(key string) {
		t.Helper()
		if _, err := st.Enqueue(sessioninbox.EnqueueRequest{
			Intent:      sessioninbox.IntentFollowup,
			Envelope:    sessioninbox.PromptEnvelope{DisplayText: key, RawText: key, SubmitText: key},
			Source:      AgentBusWakeSource,
			Idempotency: key,
		}); err != nil {
			t.Fatal(err)
		}
	}

	enqueue("agentbus-wake:default/bob/aaaa")
	enqueue("agentbus-wake:default/bob/bbbb")
	if got := queuedInboxItems(t, st); got != 2 {
		t.Fatalf("queued = %d, want the two wakes stacked before any fold", got)
	}

	enqueue("agentbus-wake:default/bob/cccc")
	if keep := genericAgentBusWakeWaiting(st); keep == "" {
		t.Fatal("a generic wake is queued, so one of them has to be the survivor")
	}
	if got := queuedInboxItems(t, st); got != 1 {
		t.Fatalf("queued = %d, want exactly one generic wake left", got)
	}

	// A dispatch wake names one assignment and the injector never re-derives it, so folding must
	// leave it alone: dropping it would silently lose work addressed to this session.
	dispatch := agentbus.DispatchKey("default", "schema")
	enqueue(dispatch)
	if got := queuedInboxItems(t, st); got != 2 {
		t.Fatalf("queued = %d, want the dispatch wake kept alongside the generic one", got)
	}
	genericAgentBusWakeWaiting(st)
	kept := false
	for _, item := range st.Snapshot().Items {
		if item.State == sessioninbox.StateQueued && item.Idempotency == dispatch {
			kept = true
		}
	}
	if !kept {
		t.Fatal("folding dropped a dispatch wake: that loses work addressed to this session")
	}
	if got := queuedInboxItems(t, st); got != 2 {
		t.Fatalf("queued = %d, want the generic survivor kept beside the dispatch wake", got)
	}
}

func queuedInboxItems(t *testing.T, st *sessioninbox.Store) int {
	t.Helper()
	n := 0
	for _, item := range st.Snapshot().Items {
		if item.State == sessioninbox.StateQueued {
			n++
		}
	}
	return n
}
