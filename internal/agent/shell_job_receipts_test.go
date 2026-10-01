package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// jobShellTool stands in for bash starting a background job: it reports the
// job's id, and the state a collection should then report.
type jobShellTool struct {
	mu    sync.Mutex
	runs  int
	jobID string
}

func (*jobShellTool) Name() string            { return "bash" }
func (*jobShellTool) Description() string     { return "fake job shell" }
func (*jobShellTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*jobShellTool) ReadOnly() bool          { return false }

func (s *jobShellTool) ExecutionDescriptor(json.RawMessage) *tool.ShellExecution {
	return &tool.ShellExecution{Kind: "shell", State: tool.ShellStateRunning}
}

func (s *jobShellTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	res, err := s.ExecuteDetailed(ctx, args)
	return res.Output, err
}

func (s *jobShellTool) ExecuteDetailed(context.Context, json.RawMessage) (tool.DetailedResult, error) {
	s.mu.Lock()
	s.runs++
	s.mu.Unlock()
	return tool.DetailedResult{
		Output:    `Started background job "bash-9". It keeps running across turns.`,
		Execution: &tool.ShellExecution{Kind: "shell", JobID: s.jobID, State: tool.ShellStateBackgroundStarted},
	}, nil
}

// collectTool stands in for bash_output/wait.
type collectTool struct{ name, state string }

func (c collectTool) Name() string          { return c.name }
func (collectTool) Description() string     { return "fake collector" }
func (collectTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (collectTool) ReadOnly() bool          { return true }

func (c collectTool) ExecutionDescriptor(json.RawMessage) *tool.ShellExecution {
	return &tool.ShellExecution{Kind: "shell", JobID: "bash-9", State: c.state}
}

func (c collectTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	res, err := c.ExecuteDetailed(ctx, args)
	return res.Output, err
}

func (c collectTool) ExecuteDetailed(context.Context, json.RawMessage) (tool.DetailedResult, error) {
	return tool.DetailedResult{
		Output:    "[bash-9] done",
		Execution: &tool.ShellExecution{Kind: "shell", JobID: "bash-9", State: c.state},
	}, nil
}

func jobReceiptAgent(t *testing.T, shell tool.Tool, collector tool.Tool) *Agent {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(shell)
	reg.Add(collector)
	a := New(&scriptedProvider{}, reg, NewSession("sys"), Options{}, event.Discard)
	a.task.ledger = evidence.NewLedger()
	return a
}

func startBackgroundCheck(t *testing.T, a *Agent, command string) {
	t.Helper()
	runBatch(t, a, provider.ToolCall{
		ID:        "s1",
		Name:      "bash",
		Arguments: `{"command":` + strconvQuote(command) + `,"run_in_background":true}`,
	})
}

// TestCollectedBackgroundCheckCreditsTheCommand is the closure the async tier
// needs: the check ran in the background, the model collected it, and the
// command it launched holds a successful receipt it can be signed off with.
func TestCollectedBackgroundCheckCreditsTheCommand(t *testing.T) {
	command := "go test ./internal/agent/"
	a := jobReceiptAgent(t, &jobShellTool{jobID: "bash-9"}, collectTool{name: "bash_output", state: tool.ShellStateCompleted})

	startBackgroundCheck(t, a, command)
	if a.task.ledger.HasSuccessfulCommand(command) {
		t.Fatal("starting a job must not credit the command: a started job is not a completed check")
	}

	results := runBatch(t, a, provider.ToolCall{ID: "c1", Name: "bash_output", Arguments: `{"job_id":"bash-9"}`})
	if !a.task.ledger.HasSuccessfulCommand(command) {
		t.Fatalf("collecting a finished job must credit its command; result=%q", results[0])
	}
	if !strings.Contains(results[0], "cite it to sign this off") || !strings.Contains(results[0], command) {
		t.Fatalf("the collection must say what it just credited: %q", results[0])
	}
}

// TestFailedBackgroundCheckIsNeverCredited keeps the closure honest: a job that
// failed, was killed, or is still running must not become a successful check.
func TestFailedBackgroundCheckIsNeverCredited(t *testing.T) {
	for _, tc := range []struct{ name, state string }{
		{"failed", tool.ShellStateFailed},
		{"killed", tool.ShellStateCancelled},
		{"still running", tool.ShellStateRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := "go test ./internal/agent/"
			a := jobReceiptAgent(t, &jobShellTool{jobID: "bash-9"}, collectTool{name: "wait", state: tc.state})

			startBackgroundCheck(t, a, command)
			runBatch(t, a, provider.ToolCall{ID: "c1", Name: "wait", Arguments: `{"job_ids":["bash-9"]}`})

			if a.task.ledger.HasSuccessfulCommand(command) {
				t.Fatalf("%s: an unfinished or failed job was credited as a successful check", tc.name)
			}
		})
	}
}

func strconvQuote(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
