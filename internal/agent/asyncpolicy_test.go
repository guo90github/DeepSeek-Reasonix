package agent

import (
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

// TestDecideOffPathTierMeanings pins the rule the shell tier has always
// followed, now that it is one decision every future off-path kind asks:
// off never lifts, balanced lifts only host-recognized long work, fast lifts
// anything with later work to overlap with.
func TestDecideOffPathTierMeanings(t *testing.T) {
	shell := OffPathRequest{Kind: OffPathShellCall, Tool: "bash", Later: LaterWorkSameBatch}
	for _, tc := range []struct {
		name string
		tier ShellAsyncTier
		req  OffPathRequest
		want bool
	}{
		{name: "off never lifts a check", tier: ShellAsyncOff, req: withHostKnowsLong(shell, true), want: false},
		{name: "off never lifts a batch with later work", tier: ShellAsyncOff, req: shell, want: false},
		{name: "off never lifts plan work", tier: ShellAsyncOff, req: withLaterWork(shell, LaterWorkTurnPending), want: false},
		{name: "balanced lifts a host-recognized check", tier: ShellAsyncBalanced, req: withHostKnowsLong(shell, true), want: true},
		{name: "balanced leaves an ordinary command", tier: ShellAsyncBalanced, req: withHostKnowsLong(shell, false), want: false},
		{name: "balanced still needs host-recognized work for plan work", tier: ShellAsyncBalanced, req: withLaterWork(shell, LaterWorkTurnPending), want: false},
		{name: "fast lifts an ordinary command", tier: ShellAsyncFast, req: withHostKnowsLong(shell, false), want: true},
		{name: "fast lifts with unfinished plan work", tier: ShellAsyncFast, req: withLaterWork(shell, LaterWorkTurnPending), want: true},
		{name: "fast leaves a lone call alone", tier: ShellAsyncFast, req: withLaterWork(shell, LaterWorkNone), want: false},
		{name: "balanced leaves a lone check alone", tier: ShellAsyncBalanced, req: withLaterWork(withHostKnowsLong(shell, true), LaterWorkNone), want: false},
		{name: "the model's own background call is never rewritten", tier: ShellAsyncFast, req: withModelAsked(shell), want: false},
		{name: "balanced leaves the model's own choice alone", tier: ShellAsyncBalanced, req: withModelAsked(withHostKnowsLong(shell, true)), want: false},
		{name: "a non-shell call is not covered", tier: ShellAsyncFast, req: withTool(shell, "read_file"), want: false},
		{name: "an unknown tier stays off", tier: ShellAsyncTier(-1), req: shell, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tier.DecideOffPath(tc.req)
			if got.OffPath != tc.want {
				t.Fatalf("OffPath = %v, want %v (reason: %q)", got.OffPath, tc.want, got.Reason)
			}
			if got.Reason == "" {
				t.Fatal("every verdict needs a reason: a declined call must not be silent")
			}
		})
	}
}

// TestOffPathLaterWorkReadsPendingCalls pins the condition promotion needs:
// work behind the call to overlap with.
func TestOffPathLaterWorkReadsPendingCalls(t *testing.T) {
	for _, tc := range []struct {
		name    string
		results []string
		start   int
		want    LaterWork
	}{
		{name: "nothing after the call", results: []string{"ran"}, start: 1, want: LaterWorkNone},
		{name: "a pending call follows", results: []string{"ran", ""}, start: 1, want: LaterWorkSameBatch},
		{name: "pending work after a finished one", results: []string{"ran", "ran", ""}, start: 1, want: LaterWorkSameBatch},
		{name: "every later call already ran", results: []string{"ran", "ran"}, start: 1, want: LaterWorkNone},
		{name: "no later index at all", results: []string{"ran"}, start: 9, want: LaterWorkNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := offPathLaterWork(tc.results, tc.start, len(tc.results)); got != tc.want {
				t.Fatalf("offPathLaterWork = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOffPathLaterReadsPendingPlanWork pins the widening the fast tier needed:
// an unfinished checklist is work to overlap with, so a lone shell call is
// lifted; a finished one leaves the call where it is.
func TestOffPathLaterReadsPendingPlanWork(t *testing.T) {
	for _, tc := range []struct {
		name  string
		todos []evidence.TodoItem
		want  LaterWork
	}{
		{name: "an unfinished checklist is later work", todos: []evidence.TodoItem{{Content: "still to do", Status: "pending"}}, want: LaterWorkTurnPending},
		{name: "a finished checklist leaves nothing", todos: []evidence.TodoItem{{Content: "done", Status: "completed"}}, want: LaterWorkNone},
		{name: "no checklist at all", todos: nil, want: LaterWorkNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shell, reader := &countingShell{}, &countingReader{}
			a := shellAsyncAgent(t, ShellAsyncFast, shell, reader)
			if tc.todos != nil {
				a.setTodoState(tc.todos)
			}
			if got := a.offPathLater([]string{"ran"}, 1, 1); got != tc.want {
				t.Fatalf("offPathLater = %v, want %v", got, tc.want)
			}
			runs := runBatch(t, a, provider.ToolCall{ID: "s1", Name: "bash", Arguments: `{"command":"go test ./..."}`})
			if len(runs) != 1 {
				t.Fatalf("results = %v, want one per call", runs)
			}
			_, args := shell.calls()
			if want := tc.want == LaterWorkTurnPending; shell.offPathLaunch() != want {
				t.Fatalf("off-path = %v, want %v: %v", shell.offPathLaunch(), want, args)
			}
			if runs[0] == "" {
				t.Fatal("the call produced no result")
			}
		})
	}
}

func withHostKnowsLong(r OffPathRequest, hostKnowsLong bool) OffPathRequest {
	r.HostKnowsLong = hostKnowsLong
	return r
}

func withLaterWork(r OffPathRequest, later LaterWork) OffPathRequest {
	r.Later = later
	return r
}

func withModelAsked(r OffPathRequest) OffPathRequest {
	r.ModelAsked = true
	return r
}

func withTool(r OffPathRequest, tool string) OffPathRequest {
	r.Tool = tool
	return r
}
