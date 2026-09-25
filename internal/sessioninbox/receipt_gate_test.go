package sessioninbox

import (
	"encoding/json"
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
	for _, key := range []string{"steerRejected", "gate", "gateReason", "pendingPrompt"} {
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
