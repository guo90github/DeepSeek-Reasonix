package sessioninbox

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// The three diagnostics are additive: a receipt that has nothing to say must
// not grow fields every existing reader would have to learn about.
func TestReceiptGateFieldsAreOmittedWhenEmpty(t *testing.T) {
	body, err := json.Marshal(InboxReceipt{ItemID: "i1", Disposition: DispositionStarted})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"steerRejected", "gate", "gateReason", "pendingPrompt",
		"state", "resumable", "retryable", "retryReason",
	} {
		if strings.Contains(string(body), key) {
			t.Fatalf("empty %s serialized: %s", key, body)
		}
	}
	filled, err := json.Marshal(InboxReceipt{
		ItemID:        "i1",
		Disposition:   DispositionQueuedFollowup,
		SteerRejected: SteerRejectedNoRunningTurn,
		Gate:          GateAwaitingAnswer,
		GateReason:    GateReasonText(GateAwaitingAnswer),
		PendingPrompt: GateWaitsForUser(GateAwaitingAnswer),
		State:         StateQueued,
		Resumable:     new(bool),
		Retryable:     new(bool),
		RetryReason:   "一步都试过了",
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded InboxReceipt
	if err := json.Unmarshal(filled, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SteerRejected != SteerRejectedNoRunningTurn || decoded.Gate != GateAwaitingAnswer {
		t.Fatalf("round trip = %+v", decoded)
	}
	if decoded.GateReason == "" || !decoded.PendingPrompt {
		t.Fatalf("reason or pendingPrompt was lost: %+v", decoded)
	}
	// A false boolean is the signal a sender escalates on, so it must survive
	// the wire instead of being folded into "field absent".
	if decoded.Resumable == nil || *decoded.Resumable || decoded.Retryable == nil || *decoded.Retryable {
		t.Fatalf("a false diagnostic was lost in transit: %+v", decoded)
	}
	if decoded.State != StateQueued || decoded.RetryReason == "" {
		t.Fatalf("state or retry reason was lost: %+v", decoded)
	}
}

// A gate with no sentence would read as "no gate" to a sender, and a
// pendingPrompt pointing at any other gate would tell a human to answer a
// prompt that is not there.
func TestEveryGateHasASentenceAndOnlyTheAnswerGateWaitsOnTheUser(t *testing.T) {
	gates := []string{
		GateAwaitingAnswer, GateTurnRunning, GateTurnFinishing, GateRotating,
		GateClosed, GateNoSessionPath, GatePaused, GateReadonly, GateHostDispatch,
	}
	for _, gate := range gates {
		if GateReasonText(gate) == "" {
			t.Fatalf("gate %q has no sentence", gate)
		}
	}
	if GateReasonText("") != "" || GateReasonText("invented") != "" {
		t.Fatal("an undeclared gate must say nothing")
	}
	if !GateWaitsForUser(GateAwaitingAnswer) {
		t.Fatal("the answer gate must report pendingPrompt")
	}
	for _, gate := range gates[1:] {
		if GateWaitsForUser(gate) {
			t.Fatalf("gate %q claimed to wait on the user", gate)
		}
	}
}

// Resumable answers "would another wake lift this?", so it is false exactly for
// the gates a person clears. Anything else either opens on its own or is named
// by the host, whose answer overrides this default.
func TestOnlyHumanClearedGatesAreNotResumable(t *testing.T) {
	for _, gate := range []string{
		GateAwaitingAnswer, GateClosed, GateNoSessionPath, GatePaused, GateReadonly,
	} {
		if GateResumable(gate) {
			t.Fatalf("gate %q claimed another wake could lift it", gate)
		}
	}
	for _, gate := range []string{
		GateTurnRunning, GateTurnFinishing, GateRotating, GateHostDispatch, "", "invented",
	} {
		if !GateResumable(gate) {
			t.Fatalf("gate %q claimed only a human could clear it", gate)
		}
	}
}

// A queued wake is reported by its own place, not by how long the queue is, and
// the lookup reads the queue as it is now: a sender that re-asks has to see the
// item move, get claimed, and finally leave.
func TestReceiptPositionIsTheItemsOwnPlaceAndEmptiesWhenGone(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "s.jsonl"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "one"}, Idempotency: "msg-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "two"}, Idempotency: "msg-2"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Position != 1 || second.Position != 2 {
		t.Fatalf("enqueue positions = %d/%d, want 1/2", first.Position, second.Position)
	}
	if first.State != StateQueued {
		t.Fatalf("enqueue state = %q, want %q", first.State, StateQueued)
	}
	if err := s.MoveItem(second.ItemID, 0); err != nil {
		t.Fatal(err)
	}
	moved, found := s.LookupReceipt("msg-2")
	if !found || moved.Position != 1 || moved.State != StateQueued {
		t.Fatalf("lookup after move = %+v found=%t", moved, found)
	}
	displaced, found := s.LookupReceipt("msg-1")
	if !found || displaced.Position != 2 {
		t.Fatalf("lookup of the displaced item = %+v found=%t", displaced, found)
	}
	if err := s.ClaimItem(first.ItemID); err != nil {
		t.Fatal(err)
	}
	claimed, found := s.LookupReceipt("msg-1")
	if !found || claimed.State != StateRunning {
		t.Fatalf("claimed item lookup = %+v found=%t", claimed, found)
	}
	if err := s.AckDequeue(first.ItemID); err != nil {
		t.Fatal(err)
	}
	consumed, found := s.LookupReceipt("msg-1")
	if !found || consumed.Position != 0 || consumed.State != "" {
		t.Fatalf("consumed item lookup = %+v found=%t", consumed, found)
	}
}
