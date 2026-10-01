package control

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/store"
	"reasonix/internal/tool"
)

func readinessAuditResultFor(result string) evidence.ReadinessAuditResult {
	return evidence.ReadinessAuditResult(result)
}

func readinessAuditForCounts(projectChecks, todos int) evidence.ReadinessAudit {
	return evidence.ReadinessAudit{Result: evidence.ReadinessBlocked, MissingProjectChecks: projectChecks, IncompleteTodos: todos}
}

// BA1b (docs/70 §2.1): the readiness verdict mapping and the counters the record
// carries — counts and ids only, never the obligations' text.
func TestTurnVerdictAndMissingCount(t *testing.T) {
	cases := map[string]string{
		"allowed": agent.TurnVerdictDelivered,
		"blocked": agent.TurnVerdictBlocked,
		"errored": agent.TurnVerdictAborted,
		"":        agent.TurnVerdictDelivered,
	}
	for result, want := range cases {
		if got := turnVerdict(readinessAuditResultFor(result)); got != want {
			t.Fatalf("turnVerdict(%q) = %q, want %q", result, got, want)
		}
	}
	audit := readinessAuditForCounts(2, 1)
	if got := missingObligationCount(audit); got != 3 {
		t.Fatalf("missing count = %d, want the counters summed", got)
	}
	var state readinessState
	if _, ok := state.last(); ok {
		t.Fatal("a fresh state has no verdict")
	}
	state.record(audit)
	if _, ok := state.last(); !ok {
		t.Fatal("a recorded verdict is visible")
	}
	state.clear()
	if _, ok := state.last(); ok {
		t.Fatal("clearing drops the previous turn's verdict")
	}
}

// A finished turn writes its outcome to the session sidecar, content-free.
func TestFinishedTurnRecordsItsOutcome(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "turn-outcome.jsonl")
	const sentinel = "SENTINEL-PROMPT-TEXT"
	prov := testutil.NewMock("turn-outcome-mock", testutil.Turn{Text: "done"})
	exec := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.Discard)
	c := New(Options{Executor: exec, SessionDir: dir, SessionPath: path, Label: "test", Sink: event.Discard})

	if err := c.Run(context.Background(), sentinel); err != nil {
		t.Fatalf("Run: %v", err)
	}

	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta: ok=%v err=%v", ok, err)
	}
	outcome, ok := agent.LatestTurnOutcome(meta)
	if !ok {
		t.Fatal("a finished turn must leave an outcome record")
	}
	if outcome.TurnSeq < 1 {
		t.Fatalf("turn = %d, want the session's turn number", outcome.TurnSeq)
	}
	if outcome.Verdict != agent.TurnVerdictDelivered {
		t.Fatalf("verdict = %q, want delivered for a turn the gate never blocked", outcome.Verdict)
	}
	if outcome.MissingCount != 0 || len(outcome.MissingIDs) != 0 {
		t.Fatalf("outcome = %+v, want no obligations for a plain turn", outcome)
	}

	// The record is content-free: no prompt text, no answer text. Read the meta
	// sidecar itself, which is where the record lands.
	data, readErr := os.ReadFile(store.SessionMeta(path))
	if readErr != nil {
		t.Fatalf("read sidecar: %v", readErr)
	}
	if strings.Contains(string(data), sentinel) || strings.Contains(string(data), "done") {
		t.Fatalf("the outcome record leaked turn text:\n%s", data)
	}
}
