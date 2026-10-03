package boot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool/builtin"
)

func agentBusToolBuild(t *testing.T, kind string) (*control.Controller, *effectRecordingProvider) {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	rec := &effectRecordingProvider{}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctrl.Close)
	return ctrl, rec
}

// The board tool is part of the provider-visible surface for every session, joined or
// not: the tool list is inside the cache-stable prefix, so it may not appear and
// disappear as a session joins or leaves a board.
func TestEffectAgentBusBoardToolIsInEveryRequest(t *testing.T) {
	ctrl, rec := agentBusToolBuild(t, "boot-effect-agentbus-tool")
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	found := false
	for _, schema := range reqs[0].Tools {
		if schema.Name == "agent_bus" {
			found = true
			if !strings.Contains(schema.Description, "blackboard") {
				t.Fatalf("agent_bus description does not say what it is: %q", schema.Description)
			}
		}
	}
	if !found {
		t.Fatalf("agent_bus never reached the provider request: %d tools", len(reqs[0].Tools))
	}
}

// Both halves of the lazy seam, and the write path behind it: before control.New the
// port refuses, an unenrolled session is told where to join, and a joined session's op
// lands on the real board and comes back in its own view.
func TestAgentBusBoardToolPortRefusesThenActs(t *testing.T) {
	empty := &atomic.Pointer[control.Controller]{}
	tool := builtin.NewAgentBusTool(boardToolPort{ctrl: empty})
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"view"}`))
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("before the controller exists: err = %v, want a not-ready refusal", err)
	}

	ctrl, _ := agentBusToolBuild(t, "boot-effect-agentbus-tool-port")
	boardDir := filepath.Join(t.TempDir(), "agentbus", "default")
	bound := &atomic.Pointer[control.Controller]{}
	bound.Store(ctrl)
	tool = builtin.NewAgentBusTool(boardToolPort{ctrl: bound})

	_, err = tool.Execute(context.Background(), json.RawMessage(`{"action":"assert","node":"build","evidence":[{"ref":"go test ./..."}]}`))
	if err == nil || !strings.Contains(err.Error(), "collaboration entry") {
		t.Fatalf("an unenrolled session: err = %v, want it to point at the collaboration entry", err)
	}

	ctrl.SetAgentBus(boardDir, "me")
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"assert","node":"build","evidence":[{"ref":"go test ./..."}]}`))
	if err != nil {
		t.Fatalf("assert on a joined board: %v", err)
	}
	if !strings.Contains(out, "assert") || !strings.Contains(out, "recorded") {
		t.Fatalf("assert result = %q, want it to report the recorded op", out)
	}
	view, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"view"}`))
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if !strings.Contains(view, "build") {
		t.Fatalf("the op did not come back in the writer's own view:\n%s", view)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"decide","node":"build","outcome":"done"}`)); err != nil {
		t.Fatalf("a board refusal must be reported as text: %v", err)
	}
}

// The deliberation chain driven the way a session drives it: the model opens a hearing
// on a contested step through the tool, both sides answer, and the verdict lands on the
// node. The builtin's own tests stop at the port; this is the seam behind it.
func TestEffectAgentBusBoardToolDrivesADeliberation(t *testing.T) {
	ctx := context.Background()
	ctrl, _ := agentBusToolBuild(t, "boot-effect-agentbus-hearing")
	boardDir := filepath.Join(t.TempDir(), "agentbus", "default")
	bound := &atomic.Pointer[control.Controller]{}
	bound.Store(ctrl)
	tool := builtin.NewAgentBusTool(boardToolPort{ctrl: bound})
	ctrl.SetAgentBus(boardDir, "me")

	// A step this session holds, asserted on its own reference.
	if _, err := tool.Execute(ctx, json.RawMessage(
		`{"action":"assert","node":"design","evidence":[{"kind":"opinion","ref":"me-says-so"}]}`)); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := tool.Execute(ctx, json.RawMessage(`{"action":"claim","node":"design","steps":2}`)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Another participant refutes it and answers with what can be checked.
	other := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	t.Cleanup(other.Close)
	other.SetAgentBus(boardDir, "someone")
	if _, err := other.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbRefute, Node: "design", Actor: "someone",
		Reason: "the reference behind this is not something I can check",
	}); err != nil {
		t.Fatalf("refute: %v", err)
	}

	opened, err := tool.Execute(ctx, json.RawMessage(`{"action":"hearing_open","node":"design"}`))
	if err != nil {
		t.Fatalf("hearing_open: %v", err)
	}
	for _, want := range []string{"design", "me", "someone"} {
		if !strings.Contains(opened, want) {
			t.Fatalf("hearing_open = %q, want it to name %q", opened, want)
		}
	}
	if _, err := other.AnswerAgentBusHearing(ctx, "design", "it does not hold", []board.Evidence{
		{Kind: "test", Ref: "go test ./internal/agentbus/..."},
		{Kind: "diff", Ref: "revert.patch"},
	}); err != nil {
		t.Fatalf("the other side answers: %v", err)
	}
	if _, err := tool.Execute(ctx, json.RawMessage(
		`{"action":"hearing_answer","node":"design","text":"it works, trust me"}`)); err != nil {
		t.Fatalf("hearing_answer: %v", err)
	}

	settled, err := tool.Execute(ctx, json.RawMessage(`{"action":"hearing_settle","node":"design"}`))
	if err != nil {
		t.Fatalf("hearing_settle: %v", err)
	}
	if !strings.Contains(settled, "settled") || !strings.Contains(settled, agentbus.VerdictRefuted) {
		t.Fatalf("hearing_settle = %q, want the refuting verdict", settled)
	}

	// The verdict is in the files and on the node, not only in this process's answer.
	log, err := agentbus.OpenHearingLog(boardDir)
	if err != nil {
		t.Fatalf("open the hearing log: %v", err)
	}
	hearings, _, err := log.Read()
	if err != nil {
		t.Fatalf("read the hearing log: %v", err)
	}
	hearing := hearings.Hearings["design"]
	if hearing == nil || hearing.Verdict != agentbus.VerdictRefuted {
		t.Fatalf("recorded hearing = %+v, want the refuting verdict", hearing)
	}
	brd, err := board.Open(boardDir)
	if err != nil {
		t.Fatalf("open the board: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := state.Nodes["design"].Outcome; got != board.OutcomeBlocked {
		t.Fatalf("outcome = %q, want the refuted step blocked", got)
	}
}
