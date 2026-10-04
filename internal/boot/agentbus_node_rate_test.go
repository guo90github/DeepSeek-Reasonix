package boot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool/builtin"
)

// agentBusRateBuild builds a host the way an operator arms the ceiling: through [agentbus] in
// the project config, never by reaching into the controller. The provider kind travels in
// because registering one kind twice panics.
func agentBusRateBuild(t *testing.T, kind, section string) *control.Controller {
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
`+section)
	ctrl, err := Build(context.Background(), Options{
		Sink:        event.Discard,
		AgentBusDir: filepath.Join(dir, "agentbus", "default"),
		AgentBusID:  "me",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctrl.Close)
	return ctrl
}

// §S5's fourth hard limit is the board's rule, so the operator's number has to arrive through
// the real assembly to bite: with the knob set, the second move on one node is refused at the
// tool face and the refusal says "not now"; with no knob, nothing is refused.
func TestEffectTheNodeRateCeilingIsArmedFromTheOperatorsConfig(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		kind    string
		section string
		refused bool
	}{
		{"one move per minute", "boot-effect-node-rate-armed", "\n[agentbus]\nnode_rate_per_minute = 1\n", true},
		{"no ceiling configured", "boot-effect-node-rate-off", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := agentBusRateBuild(t, tc.kind, tc.section)
			bound := &atomic.Pointer[control.Controller]{}
			bound.Store(ctrl)
			tool := builtin.NewAgentBusTool(boardToolPort{ctrl: bound})
			move := func(ref string) json.RawMessage {
				return json.RawMessage(`{"action":"assert","node":"hot","evidence":[{"kind":"test","ref":"` + ref + `"}]}`)
			}
			out, err := tool.Execute(ctx, move("e1"))
			if err != nil {
				t.Fatalf("the first move: %v", err)
			}
			if strings.Contains(out, "refused") {
				t.Fatalf("the first move was refused: %s", out)
			}
			out, err = tool.Execute(ctx, move("e2"))
			if err != nil {
				t.Fatalf("the second move: %v (a refusal is text, not an error)", err)
			}
			if got := strings.Contains(out, "refused (rate_limited)"); got != tc.refused {
				t.Fatalf("the second move = %q (refused=%v), want refused=%v", out, got, tc.refused)
			}
			if tc.refused && !strings.Contains(out, "not now") {
				t.Fatalf("the refusal does not tell the caller to come back: %q", out)
			}
		})
	}
}
