package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

type transientOnceTool struct {
	calls atomic.Int32
}

func (t *transientOnceTool) Name() string            { return "read_file" }
func (t *transientOnceTool) Description() string     { return "" }
func (t *transientOnceTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *transientOnceTool) ReadOnly() bool          { return true }
func (t *transientOnceTool) Execute(context.Context, json.RawMessage) (string, error) {
	if t.calls.Add(1) == 1 {
		return "", errors.New("connection reset by peer")
	}
	return "ok", nil
}

func TestDispatchResolvedToolRetriesReadOnlyTransientOnce(t *testing.T) {
	target := &transientOnceTool{}
	a := &Agent{}
	plan := &toolCallPlan{runTool: target, runArgs: json.RawMessage(`{}`), readOnly: true}
	result, _, _, err := a.dispatchResolvedTool(context.Background(), plan)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result != "ok" || target.calls.Load() != 2 {
		t.Fatalf("result=%q calls=%d", result, target.calls.Load())
	}
}

func TestDispatchResolvedToolDoesNotRetryWriterTransient(t *testing.T) {
	target := &transientOnceTool{}
	a := &Agent{}
	plan := &toolCallPlan{
		runTool: target, runArgs: json.RawMessage(`{}`), readOnly: true,
		effects: evidence.ToolEffects{StateMutation: true},
	}
	_, _, _, err := a.dispatchResolvedTool(context.Background(), plan)
	if err == nil || target.calls.Load() != 1 {
		t.Fatalf("writer retry: err=%v calls=%d", err, target.calls.Load())
	}
}

type fixedErrorTool struct {
	err   error
	calls atomic.Int32
}

type deterministicApplicationError string

func (e deterministicApplicationError) Error() string          { return string(e) }
func (deterministicApplicationError) RetryableToolError() bool { return false }

func (t *fixedErrorTool) Name() string            { return "read_file" }
func (t *fixedErrorTool) Description() string     { return "" }
func (t *fixedErrorTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *fixedErrorTool) ReadOnly() bool          { return true }
func (t *fixedErrorTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.calls.Add(1)
	return "", t.err
}

func TestDispatchResolvedToolNeverRetriesCancellationOrAmbiguousDispatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "cancelled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "may have completed", err: errors.New("connection reset after dispatch; execution may have completed and was not retried")},
		{name: "unknown result", err: errors.New("connection closed after dispatch; execution result is unknown")},
		{name: "typed application unavailable", err: deterministicApplicationError("resource unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &fixedErrorTool{err: tc.err}
			plan := &toolCallPlan{runTool: target, runArgs: json.RawMessage(`{}`), readOnly: true}
			_, _, _, _ = (&Agent{}).dispatchResolvedTool(context.Background(), plan)
			if got := target.calls.Load(); got != 1 {
				t.Fatalf("calls = %d, want 1", got)
			}
		})
	}
}

var _ tool.Tool = (*transientOnceTool)(nil)

type callerSessionRecordingTool struct{ seen string }

func (t *callerSessionRecordingTool) Name() string        { return "mcp__chatting__room_wait" }
func (t *callerSessionRecordingTool) Description() string { return "" }
func (t *callerSessionRecordingTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *callerSessionRecordingTool) ReadOnly() bool { return true }
func (t *callerSessionRecordingTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	t.seen = tool.CallerSessionPath(ctx)
	return "ok", nil
}

// One MCP child serves every session of a plugin host, so the session it reports
// must be the one whose agent is calling — not the host's default, which is a
// tab choice, and not the foreground tab, which is a window choice.
func TestDispatchResolvedToolMarksTheCallerSession(t *testing.T) {
	a := &Agent{}
	a.SetSessionPath("/sessions/room.jsonl")
	target := &callerSessionRecordingTool{}
	plan := &toolCallPlan{runTool: target, runArgs: json.RawMessage(`{}`), readOnly: true}
	if _, _, _, err := a.dispatchResolvedTool(context.Background(), plan); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if target.seen != "/sessions/room.jsonl" {
		t.Fatalf("tool saw caller session %q, want the agent's own session", target.seen)
	}
}
