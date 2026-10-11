package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/tool"
)

type stubReadTool struct {
	name     string
	output   string
	readOnly bool
	runs     *atomic.Int32
}

func (s stubReadTool) Name() string           { return s.name }
func (s stubReadTool) Description() string    { return "stub" }
func (s stubReadTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s stubReadTool) ReadOnly() bool         { return s.readOnly }
func (s stubReadTool) PlanModeSafe() bool     { return true }

func (s stubReadTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	if s.runs != nil {
		s.runs.Add(1)
	}
	return fmt.Sprintf("%s:%s", s.output, string(args)), nil
}

func callNodeRegistry(t *testing.T, runs *atomic.Int32) *tool.Registry {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(stubReadTool{name: "read_file", output: "alpha", readOnly: true, runs: runs})
	reg.Add(stubReadTool{name: "bash", output: "boom", readOnly: false, runs: runs})
	return reg
}

// TestParallelCallsRunsReadOnlyNodesInOneRound: one tool call, every node run by
// the host, every answer in the merged result the model gets back.
func TestParallelCallsRunsReadOnlyNodesInOneRound(t *testing.T) {
	var runs atomic.Int32
	host := NewParallelCallsTool(callNodeRegistry(t, &runs))
	out, err := host.Execute(context.Background(), json.RawMessage(`{"calls":[
		{"tool":"read_file","arguments":{"path":"a.txt"},"description":"a"},
		{"tool":"read_file","arguments":{"path":"b.txt"},"description":"b"}]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := runs.Load(); got != 2 {
		t.Fatalf("ran %d nodes, want 2", got)
	}
	for _, want := range []string{"── a ──", "── b ──", `alpha:{"path":"a.txt"}`, `alpha:{"path":"b.txt"}`} {
		if !strings.Contains(out, want) {
			t.Fatalf("aggregate %q is missing %q", out, want)
		}
	}
}

// TestParallelCallsRefusesAnythingThatCanTouchState: the node that could mutate
// is refused before it runs, while its read-only sibling still completes.
func TestParallelCallsRefusesAnythingThatCanTouchState(t *testing.T) {
	var runs atomic.Int32
	host := NewParallelCallsTool(callNodeRegistry(t, &runs))
	out, err := host.Execute(context.Background(), json.RawMessage(`{"calls":[
		{"tool":"read_file","arguments":{"path":"a.txt"}},
		{"tool":"bash","arguments":{"command":"rm -rf /"}}]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("ran %d nodes, want only the read-only one", got)
	}
	if !strings.Contains(out, "statically read-only") {
		t.Fatalf("a stateful node was not refused: %q", out)
	}
}

// TestParallelCallsRejectsNestingAndBadRequests keeps the surface fail-closed:
// a worker-spawning node, a degenerate single node, an unknown field and
// non-object arguments are all refused before anything runs.
func TestParallelCallsRejectsNestingAndBadRequests(t *testing.T) {
	host := NewParallelCallsTool(tool.NewRegistry())
	for _, tc := range []struct{ name, args, want string }{
		{"nested worker", `{"calls":[{"tool":"task"},{"tool":"read_file"}]}`, "runs its own worker"},
		{"self nesting", `{"calls":[{"tool":"parallel_calls"},{"tool":"read_file"}]}`, "cannot nest itself"},
		{"single call", `{"calls":[{"tool":"read_file"}]}`, "issue it directly"},
		{"unknown field", `{"calls":[],"extra":1}`, "invalid args"},
		{"non-object arguments", `{"calls":[{"tool":"read_file","arguments":["a"]},{"tool":"read_file"}]}`, "JSON object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := host.Execute(context.Background(), json.RawMessage(tc.args)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestParallelCallsReportsUnknownToolWithoutRunning checks the registry miss is
// a node failure, not a group failure: the other node still returns its answer.
func TestParallelCallsReportsUnknownToolWithoutRunning(t *testing.T) {
	var runs atomic.Int32
	host := NewParallelCallsTool(callNodeRegistry(t, &runs))
	out, err := host.Execute(context.Background(), json.RawMessage(`{"calls":[
		{"tool":"read_file","arguments":{"path":"a.txt"}},
		{"tool":"nope","arguments":{}}]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "unknown tool") || !strings.Contains(out, "status: completed") {
		t.Fatalf("aggregate %q should carry one failure and one answer", out)
	}
}

func TestPlanExecuteTierParsing(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want PlanExecuteTier
	}{
		{raw: "on", want: PlanExecuteOn},
		{raw: "ON", want: PlanExecuteOn},
		{raw: " on ", want: PlanExecuteOn},
		{raw: "", want: PlanExecuteOff},
		{raw: "off", want: PlanExecuteOff},
		{raw: "auto", want: PlanExecuteOff},
	} {
		if got := ParsePlanExecuteTier(tc.raw); got != tc.want {
			t.Fatalf("ParsePlanExecuteTier(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
	if PlanExecuteOff.Enabled() || !PlanExecuteOn.Enabled() {
		t.Fatal("tier gating reads the wrong way")
	}
}
