package agent

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// A state directory this host cannot write must not cost it the retry: recovery bookkeeping
// fails open, so the first incident still gets its one silent retry and the second stays quiet.
func TestMissingReasoningRecoveryIOFailureStillSuppressesLocally(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(statePath, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	prov := strictToolCallReasoningProvider{testutil.NewMock("deepseek-proxy")}
	a := New(prov, echoRegistry(), NewSession(""), Options{MissingReasoningWarnStateDir: statePath}, event.Discard)
	calls := []provider.ToolCall{{ID: "c1", Name: "echo", Arguments: `{"text":"hi"}`}}

	if missing, retry := a.observeMissingToolCallReasoning(calls, ""); !missing || !retry {
		t.Fatalf("initial observation = missing:%v retry:%v, want true/true", missing, retry)
	}
	if missing, retry := a.observeMissingToolCallReasoning(calls, ""); !missing || retry {
		t.Fatalf("repeated observation = missing:%v retry:%v, want true/false", missing, retry)
	}
}
