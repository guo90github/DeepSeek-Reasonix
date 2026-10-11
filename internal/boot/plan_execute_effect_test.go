package boot

// The host-executed call surface at the real boundary: with the tier off the
// provider-visible tool list stays byte-identical, and with it on two
// independent reads cost one round instead of two.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// planExecMode scripts how the model behaves: the baseline reads one file per
// round, the tier's mode asks for both in a single host-executed call.
type planExecMode string

const (
	planExecBaseline planExecMode = "baseline"
	planExecHostCall planExecMode = "host-call"
)

type planExecProvider struct {
	mode planExecMode

	mu       sync.Mutex
	round    int
	tools    [][]byte
	requests []provider.Request
}

func (*planExecProvider) Name() string { return "boot-plan-exec" }

func (p *planExecProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	toolsJSON, _ := json.Marshal(req.Tools)
	p.mu.Lock()
	p.round++
	round := p.round
	p.tools = append(p.tools, toolsJSON)
	p.requests = append(p.requests, req)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 2)
	switch {
	case p.mode == planExecBaseline && round <= 2:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID:        fmt.Sprintf("read-%d", round),
			Name:      "read_file",
			Arguments: fmt.Sprintf(`{"path":"note%d.txt"}`, round),
		}}
	case p.mode == planExecHostCall && round == 1:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID:   "host-calls-1",
			Name: "parallel_calls",
			Arguments: `{"calls":[` +
				`{"tool":"read_file","arguments":{"path":"note1.txt"},"description":"one"},` +
				`{"tool":"read_file","arguments":{"path":"note2.txt"},"description":"two"}]}`,
		}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *planExecProvider) observed() (rounds int, tools [][]byte, requests []provider.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.round, append([][]byte(nil), p.tools...), append([]provider.Request(nil), p.requests...)
}

// runPlanExecTurn builds and runs one turn with the given tier. An empty tier
// writes no key at all, which is how the untouched default is exercised.
func runPlanExecTurn(t *testing.T, mode planExecMode, tier string) *planExecProvider {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	writeFile(t, dir, "note1.txt", "alpha\n")
	writeFile(t, dir, "note2.txt", "beta\n")

	rec := &planExecProvider{mode: mode}
	// provider.Register is process-global, so the kind must differ per turn —
	// two runs of the same mode and tier in one test binary would collide.
	kind := fmt.Sprintf("boot-plan-exec-%s-%s-%s", mode, tier, t.Name())
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })

	tierLine := ""
	if tier != "" {
		tierLine = fmt.Sprintf("plan_execute = %q\n", tier)
	}
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
`+tierLine+`
[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "read both note files"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rec
}

// TestEffectPlanExecuteOffLeavesTheToolSurfaceByteIdentical is the promise the
// tier exists to keep: with the switch off — or absent — the tools the provider
// sees are the same bytes, and the host-executed surface is not offered at all.
func TestEffectPlanExecuteOffLeavesTheToolSurfaceByteIdentical(t *testing.T) {
	absent := runPlanExecTurn(t, planExecBaseline, "")
	off := runPlanExecTurn(t, planExecBaseline, "off")

	_, absentTools, _ := absent.observed()
	_, offTools, _ := off.observed()
	if len(absentTools) == 0 || len(offTools) == 0 {
		t.Fatal("no provider request was captured")
	}
	if !bytes.Equal(absentTools[0], offTools[0]) {
		t.Fatalf("the tool surface changed when the switch was written out:\nabsent: %s\noff:    %s", absentTools[0], offTools[0])
	}
	if strings.Contains(string(offTools[0]), "parallel_calls") {
		t.Fatalf("the off surface offers the host-executed tool: %s", offTools[0])
	}
	if strings.Contains(string(offTools[0]), `"parallel_tasks"`) || strings.Contains(string(offTools[0]), `"fleet"`) {
		t.Fatalf("the off surface offers the fan-out tools: %s", offTools[0])
	}
}

// TestEffectPlanExecuteOnRunsBothReadsInOneRound is the point of the tier: two
// independent reads cost one model round instead of two, and both answers reach
// the model in that same round.
func TestEffectPlanExecuteOnRunsBothReadsInOneRound(t *testing.T) {
	baseline := runPlanExecTurn(t, planExecBaseline, "off")
	hostCall := runPlanExecTurn(t, planExecHostCall, "on")

	baselineRounds, _, _ := baseline.observed()
	hostRounds, hostTools, hostRequests := hostCall.observed()
	if baselineRounds != 3 {
		t.Fatalf("the baseline spent %d rounds for two reads, want 3 (read, read, answer)", baselineRounds)
	}
	if hostRounds != 2 {
		t.Fatalf("the host-executed turn spent %d rounds, want 2 (one call, one answer)", hostRounds)
	}
	if !strings.Contains(string(hostTools[0]), "parallel_calls") {
		t.Fatalf("the on surface does not offer the host-executed tool: %s", hostTools[0])
	}
	// The tier is also what finally puts the sub-agent fan-out in front of the
	// model: before it, parallel_tasks and fleet were registered but hidden.
	for _, want := range []string{`"parallel_tasks"`, `"fleet"`} {
		if !strings.Contains(string(hostTools[0]), want) {
			t.Fatalf("the on surface does not offer %s: %s", want, hostTools[0])
		}
	}
	if len(hostRequests) < 2 {
		t.Fatalf("captured %d requests, want the tool round and the answer round", len(hostRequests))
	}
	var answerPrompt strings.Builder
	for _, msg := range hostRequests[1].Messages {
		answerPrompt.WriteString(msg.Content)
		answerPrompt.WriteString("\n")
	}
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(answerPrompt.String(), want) {
			t.Fatalf("the round after the call is missing %q:\n%s", want, answerPrompt.String())
		}
	}
}
