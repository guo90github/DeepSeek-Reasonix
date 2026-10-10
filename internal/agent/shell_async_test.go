package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// countingShell stands in for bash so the promotion rule can be measured
// without launching a process. It records the arguments it was handed and
// whether the host moved the call off the critical path, mirroring the real
// tool: the branch is taken for the model's own request or for the mark.
type countingShell struct {
	mu      sync.Mutex
	runs    int
	offPath bool
	raw     string
	lastArg map[string]any
}

func (*countingShell) Name() string            { return "bash" }
func (*countingShell) Description() string     { return "fake shell" }
func (*countingShell) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*countingShell) ReadOnly() bool          { return false }

func (s *countingShell) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed map[string]any
	_ = json.Unmarshal(args, &parsed)
	offPath := tool.OffPathLaunchFrom(ctx)
	s.mu.Lock()
	s.runs++
	s.offPath = offPath
	s.raw = string(args)
	s.lastArg = parsed
	s.mu.Unlock()
	if background, _ := parsed["run_in_background"].(bool); background || offPath {
		return `Started background job "job-1". It keeps running across turns.`, nil
	}
	return "command output", nil
}

func (s *countingShell) calls() (int, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs, s.lastArg
}

func (s *countingShell) offPathLaunch() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offPath
}

func (s *countingShell) rawArguments() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw
}

// countingReader stands in for a later call in the same batch: what matters is
// whether it ran at all.
type countingReader struct {
	mu   sync.Mutex
	runs int
}

func (*countingReader) Name() string            { return "read_file" }
func (*countingReader) Description() string     { return "fake reader" }
func (*countingReader) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*countingReader) ReadOnly() bool          { return true }

func (r *countingReader) Execute(context.Context, json.RawMessage) (string, error) {
	r.mu.Lock()
	r.runs++
	r.mu.Unlock()
	return "   1→note", nil
}

func (r *countingReader) executions() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs
}

// TestShellAsyncRunsFollowersAfterAProvenlyInertCall pins the narrow case: a
// lifted command the host can prove writes nothing (go vet) cannot make a
// trailing call observe a half-applied change, so the trailing call runs
// instead of paying a round to be sent again. The deferral side is covered by
// TestShellAsyncPromotesTheSlowCallAndDefersTheRest, whose command (go test) is
// not statically known and therefore may write.
func TestShellAsyncRunsFollowersAfterAProvenlyInertCall(t *testing.T) {
	shell, reader := &countingShell{}, &countingReader{}
	a := shellAsyncAgent(t, ShellAsyncFast, shell, reader)

	results := runBatch(t, a,
		provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"go vet ./..."}`},
		provider.ToolCall{ID: "r1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
	)

	if !shell.offPathLaunch() {
		t.Fatal("the inert check was not moved off the critical path")
	}
	if got := reader.executions(); got != 1 {
		t.Fatalf("the trailing read ran %d times, want 1: an inert command cannot make it stale", got)
	}
	if strings.Contains(results[1], "moved to the background") {
		t.Fatalf("the trailing read was deferred behind an inert command: %q", results[1])
	}
}

func shellAsyncAgent(t *testing.T, tier ShellAsyncTier, shell tool.Tool, reader tool.Tool) *Agent {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(shell)
	reg.Add(reader)
	return New(&scriptedProvider{}, reg, NewSession("sys"), Options{ShellAsync: tier, ContextWindow: 64_000}, event.Discard)
}

func runBatch(t *testing.T, a *Agent, calls ...provider.ToolCall) []string {
	t.Helper()
	b := a.executeBatch(context.Background(), &a.turn, calls)
	if len(b.results) != len(calls) {
		t.Fatalf("results = %v, want one per call", b.results)
	}
	return b.results
}

// TestShellAsyncPromotesTheSlowCallAndDefersTheRest pins the whole rule: with a
// later call pending, the shell call starts in the background and the calls
// behind it do not race the job.
func TestShellAsyncPromotesTheSlowCallAndDefersTheRest(t *testing.T) {
	shell, reader := &countingShell{}, &countingReader{}
	a := shellAsyncAgent(t, ShellAsyncFast, shell, reader)

	results := runBatch(t, a,
		provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"go test ./..."}`},
		provider.ToolCall{ID: "r1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
	)

	runs, args := shell.calls()
	if runs != 1 {
		t.Fatalf("shell ran %d times", runs)
	}
	if !shell.offPathLaunch() {
		t.Fatalf("the shell call was not moved off the critical path: %v", args)
	}
	if !strings.Contains(results[0], "Started background job") {
		t.Fatalf("the promoted call's result must say so: %q", results[0])
	}
	if !strings.Contains(results[1], "moved to the background") || !strings.Contains(results[1], "Collect it first") {
		t.Fatalf("the later call must be deferred with the collection step: %q", results[1])
	}
	if got := reader.executions(); got != 0 {
		t.Fatalf("a call behind a background job ran %d times; it must wait", got)
	}
}

// TestShellAsyncPromotesWithoutRewritingTheCall pins the channel: the host
// carries its decision out of band, so the tool sees the arguments the model
// wrote, byte for byte, even when the call is lifted.
func TestShellAsyncPromotesWithoutRewritingTheCall(t *testing.T) {
	shell, reader := &countingShell{}, &countingReader{}
	a := shellAsyncAgent(t, ShellAsyncFast, shell, reader)

	written := `{"command":"go test ./...","timeout":30}`
	runBatch(t, a,
		provider.ToolCall{ID: "s1", Name: "bash", Arguments: written},
		provider.ToolCall{ID: "r1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
	)

	if !shell.offPathLaunch() {
		t.Fatal("the call was not moved off the critical path")
	}
	if got := shell.rawArguments(); got != written {
		t.Fatalf("the host edited the call the model wrote: %q, want %q", got, written)
	}
}

// TestShellAsyncOffLeavesTheBatchAlone is the default: nothing is promoted and
// the batch runs in order.
func TestShellAsyncOffLeavesTheBatchAlone(t *testing.T) {
	shell, reader := &countingShell{}, &countingReader{}
	a := shellAsyncAgent(t, ShellAsyncOff, shell, reader)

	results := runBatch(t, a,
		provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"go test ./..."}`},
		provider.ToolCall{ID: "r1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
	)

	if _, args := shell.calls(); args["run_in_background"] != nil {
		t.Fatalf("the tier is off but the call was rewritten: %v", args)
	}
	if shell.offPathLaunch() {
		t.Fatal("the tier is off but the call was moved off the critical path")
	}
	if got := reader.executions(); got != 1 {
		t.Fatalf("the later call ran %d times, want 1", got)
	}
	if strings.Contains(results[1], "moved to the background") {
		t.Fatalf("the later call was deferred with the tier off: %q", results[1])
	}
}

// TestShellAsyncBalancedOnlyPromotesChecks pins the middle tier: a check goes
// to the background, an ordinary command keeps its place in the foreground.
func TestShellAsyncBalancedOnlyPromotesChecks(t *testing.T) {
	for _, tc := range []struct {
		command  string
		promoted bool
		why      string
	}{
		{command: "go test ./internal/agent/", promoted: true, why: "a check is the long call a batch rarely needs at once"},
		{command: "cat note.txt", promoted: false, why: "an ordinary command stays in the foreground"},
	} {
		shell, reader := &countingShell{}, &countingReader{}
		a := shellAsyncAgent(t, ShellAsyncBalanced, shell, reader)
		runBatch(t, a,
			provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"` + tc.command + `"}`},
			provider.ToolCall{ID: "r1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
		)
		if got := shell.offPathLaunch(); got != tc.promoted {
			t.Errorf("%q: promoted=%v, want %v (%s)", tc.command, got, tc.promoted, tc.why)
		}
	}
}

// TestShellAsyncNeverRewritesAnExplicitBackgroundCall keeps the model's own
// choice intact: it asked for the background, so the host neither edits the call
// nor marks it as its own decision.
func TestShellAsyncNeverRewritesAnExplicitBackgroundCall(t *testing.T) {
	shell, reader := &countingShell{}, &countingReader{}
	a := shellAsyncAgent(t, ShellAsyncFast, shell, reader)

	runBatch(t, a,
		provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"sleep 30","run_in_background":true}`},
		provider.ToolCall{ID: "r1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
	)

	if _, args := shell.calls(); args["run_in_background"] != true || len(args) != 2 {
		t.Fatalf("an explicit background call was rewritten: %v", args)
	}
	if shell.offPathLaunch() {
		t.Fatal("the host marked the model's own background call as its decision")
	}
	if got := reader.executions(); got != 1 {
		t.Fatalf("the later call ran %d times, want 1: an explicit background call is the model's own parallelism", got)
	}
}

// TestShellAsyncLeavesALoneCallAlone: with nothing after it, waiting is the
// honest thing to do — there is no work to continue with.
func TestShellAsyncLeavesALoneCallAlone(t *testing.T) {
	shell, reader := &countingShell{}, &countingReader{}
	a := shellAsyncAgent(t, ShellAsyncFast, shell, reader)

	runBatch(t, a, provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"go test ./..."}`})

	if _, args := shell.calls(); args["run_in_background"] != nil {
		t.Fatalf("a lone call was promoted: %v", args)
	}
	if shell.offPathLaunch() {
		t.Fatal("a lone call was moved off the critical path")
	}
}
