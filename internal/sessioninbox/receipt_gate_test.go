package sessioninbox

import (
	"encoding/json"
	"strings"
	"testing"
)

// The two diagnostics are additive: a receipt that has nothing to say must not
// grow fields every existing reader would have to learn about.
func TestReceiptGateFieldsAreOmittedWhenEmpty(t *testing.T) {
	body, err := json.Marshal(InboxReceipt{ItemID: "i1", Disposition: DispositionStarted})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"steerRejected", "dispatchGate"} {
		if strings.Contains(string(body), key) {
			t.Fatalf("empty %s serialized: %s", key, body)
		}
	}
	filled, err := json.Marshal(InboxReceipt{
		ItemID:        "i1",
		Disposition:   DispositionQueuedFollowup,
		SteerRejected: SteerRejectedNoRunningTurn,
		DispatchGate:  DispatchGatePaused,
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded InboxReceipt
	if err := json.Unmarshal(filled, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SteerRejected != SteerRejectedNoRunningTurn || decoded.DispatchGate != DispatchGatePaused {
		t.Fatalf("round trip = %+v", decoded)
	}
}
