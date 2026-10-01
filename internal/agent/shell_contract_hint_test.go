package agent

import (
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

// The refusal the model actually receives has to carry the split, not just the
// advice: naming the calls is what turns one refused compound command into one
// correct retry, and the calls named must be the model's own segments.
func TestShellContractRefusalCarriesTheSplitToTheModel(t *testing.T) {
	bash, ok := tool.LookupBuiltin("bash")
	if !ok {
		t.Fatal("bash builtin is not registered")
	}
	a := newIncompleteReadTestAgent(&scriptedProvider{}, incompleteReadBuiltin(t), NewSession("sys"), event.Discard, bash)
	a.task.ledger = evidence.NewLedger()

	// Mixed in ordinary and delivery modes alike, so the refusal is the same
	// whichever classifier this turn runs.
	command := `go generate ./... ; go test ./...`
	out, _ := roundOutcome(t, a, "b1", "bash", map[string]any{"command": command})
	if !strings.Contains(out, "blocked:") {
		t.Fatalf("fixture: this shape must be refused before launch: %q", out)
	}
	if !strings.Contains(out, "send each as its own call") {
		t.Fatalf("the refusal must name the calls to send: %q", out)
	}
	for _, segment := range []string{"1) go generate ./...", "2) go test ./..."} {
		if !strings.Contains(out, segment) {
			t.Fatalf("the refusal must carry %q: %q", segment, out)
		}
	}
}
