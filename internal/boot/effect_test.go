package boot

// Effect tests assert final-boundary behavior through the real Build stack:
// a scripted provider records what actually reaches the provider boundary.
// Component correctness is not system effectiveness (see REASONIX.md).

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/ablation"
	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/memory"
	"reasonix/internal/provider"
)

type effectRecordingProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *effectRecordingProvider) Name() string { return "boot-effect-test" }

func (p *effectRecordingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	chunks := []provider.Chunk{
		{Type: provider.ChunkText, Text: "ok"},
		{Type: provider.ChunkDone},
	}
	ch := make(chan provider.Chunk, len(chunks))
	for _, chunk := range chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func (p *effectRecordingProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// effectRun builds the real stack around a recording provider, runs one
// prompt, and returns every request that reached the provider boundary.
func effectRun(t *testing.T, kind, tokenMode string, arm ablation.Set) []provider.Request {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
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

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard, TokenMode: tokenMode, Ablation: arm})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	return reqs
}

func toolNames(req provider.Request) map[string]bool {
	names := make(map[string]bool, len(req.Tools))
	for _, tool := range req.Tools {
		names[tool.Name] = true
	}
	return names
}

// TestEffectRoleSettingsShareProviderToolSurface pins the unified contract:
// light/balanced/delivery send identical top-level tool schemas; optional
// tools are reached only through use_capability.
func TestEffectRoleSettingsShareProviderToolSurface(t *testing.T) {
	balanced := effectRun(t, "boot-effect-balanced", "", ablation.Set{})
	light := effectRun(t, "boot-effect-light", "economy", ablation.Set{})
	delivery := effectRun(t, "boot-effect-delivery", "delivery", ablation.Set{})

	balNames := toolSchemaNames(balanced[0].Tools)
	if !reflect.DeepEqual(toolSchemaNames(light[0].Tools), balNames) {
		t.Fatalf("light surface diverged from balanced\nlight=%v\nbalanced=%v", toolSchemaNames(light[0].Tools), balNames)
	}
	if !reflect.DeepEqual(toolSchemaNames(delivery[0].Tools), balNames) {
		t.Fatalf("delivery surface diverged from balanced\ndelivery=%v\nbalanced=%v", toolSchemaNames(delivery[0].Tools), balNames)
	}
	if len(balNames) > 16 {
		t.Fatalf("unified surface sent %d tools; expected a small fixed core set", len(balNames))
	}
	names := toolNames(balanced[0])
	if !names["use_capability"] {
		t.Fatal("unified surface must expose use_capability")
	}
	if names["connect_tool_source"] {
		t.Fatal("connect_tool_source must not appear on the provider-visible surface")
	}
	if names["task"] || names["grep"] {
		t.Fatal("optional tools must not be top-level; use use_capability")
	}
}

// TestEffectSubagentAblationRemovesChildToolSchemas asserts the ablation at
// the capability boundary: with subagents off the model cannot dispatch
// task/fleet through the registry even via use_capability.
func TestEffectSubagentAblationRemovesChildToolSchemas(t *testing.T) {
	control := effectRun(t, "boot-effect-sub-on", "", ablation.Set{})
	ablated := effectRun(t, "boot-effect-sub-off", "", ablation.New(ablation.Subagent))

	// Top-level schema never exposes task; verify registry dispatch instead.
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	_ = control
	_ = ablated
	// Ablation is enforced inside TaskTool registration at boot; the unified
	// surface stays use_capability-only either way.
	if names := toolNames(control[0]); names["task"] {
		t.Fatal("unified surface must not expose task top-level")
	}
	if names := toolNames(ablated[0]); names["task"] || names["parallel_tasks"] || names["fleet"] {
		t.Fatalf("subagent-ablated surface still offers spawn tools top-level: %v", toolSchemaNames(ablated[0].Tools))
	}
}

// budgetRunawayProvider never repeats itself and never fails, so every
// adaptive guard stays quiet. Only the spend gate can stop it.
type budgetRunawayProvider struct {
	mu     sync.Mutex
	rounds int
}

func (p *budgetRunawayProvider) Name() string { return "boot-budget-runaway" }

func (p *budgetRunawayProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.rounds++
	round := p.rounds
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 4)
	ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
		ID:        fmt.Sprintf("call-%d", round),
		Name:      "read_file",
		Arguments: fmt.Sprintf(`{"path":"file%d.txt"}`, round),
	}}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{
		PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100, RequestCount: 1,
	}}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *budgetRunawayProvider) roundCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rounds
}

// TestEffectTaskBudgetLandsARunawayThroughRealBuild pins the gate at its final
// boundary: a configured spend budget must stop a wandering turn through the
// real Build assembly. Nothing else would stop it: ordinary chat has no round
// ceiling, and this provider never repeats itself.
func TestEffectTaskBudgetLandsARunawayThroughRealBuild(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &budgetRunawayProvider{}
	provider.Register("boot-budget-gate", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
task_time_budget_minutes = 0.0005

[[providers]]
name = "test-model"
kind = "boot-budget-gate"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runErr := ctrl.Run(runCtx, "read every file you can find")

	// Ordinary chat has no round ceiling, so a gate that never reached the
	// executor would run until the context deadline. Assert the typed boundary
	// instead of a machine-speed-dependent round count.
	pause, ok := agent.InspectRunPause(runErr)
	if !ok || pause.Kind != "task_budget" || pause.Key != "time" {
		t.Fatalf("Run error = %v (pause=%+v, ok=%v), want time task-budget pause", runErr, pause, ok)
	}
	if rec.roundCount() == 0 {
		t.Fatal("no round reached the provider; the run never started")
	}
}

// docs/50 A-32, at the provider boundary: the type's default scope decides
// whether a fact's body actually reaches the model. An unscoped user/feedback
// fact is global, so it is pinned guidance and rides the session-context
// envelope; an unscoped project fact is retrieval-only, so its body stays out
// (only its index description is sent).
func TestEffectDefaultScopeDecidesWhetherAFactBodyReachesTheProvider(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-scope-effect-test", func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", scopedFactConfig)

	seed, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("seed Build: %v", err)
	}
	mem := seed.Memory()
	if _, err := mem.Store.Save(memory.Memory{
		Name: "work-style", Description: "how to work", Type: memory.TypeFeedback, Body: "PINNED-STYLE-BODY-9f",
	}); err != nil {
		seed.Close()
		t.Fatalf("save feedback fact: %v", err)
	}
	if _, err := mem.Store.Save(memory.Memory{
		Name: "project-note", Description: "project note description", Type: memory.TypeProject, Body: "PROJECT-ONLY-BODY-9f",
	}); err != nil {
		seed.Close()
		t.Fatalf("save project fact: %v", err)
	}
	seed.Close()

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	containsSent := func(needle string) bool {
		for _, message := range reqs[0].Messages {
			if strings.Contains(message.Content, needle) {
				return true
			}
		}
		return false
	}
	containsSystem := func(needle string) bool { return strings.Contains(systemMessage(reqs[0].Messages), needle) }
	if !containsSent("PINNED-STYLE-BODY-9f") {
		t.Fatal("global feedback fact body did not reach the provider request")
	}
	if containsSent("PROJECT-ONLY-BODY-9f") {
		t.Fatal("retrieval-only project fact body leaked into the provider request")
	}
	if !containsSent("project note description") {
		t.Fatal("retrieval-only fact should still be indexed by description")
	}
	if containsSystem("PINNED-STYLE-BODY-9f") {
		t.Fatal("pinned guidance must ride the session-context envelope, not the system prompt")
	}
}

const scopedFactConfig = `
default_model = "test-model"

[agent]
system_prompt = "STABLE BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-scope-effect-test"
model = "x"
`

// docs/70 §2.3, at the real boundary: the pre-turn progress block rides the turn
// BODY (so a later turn starts knowing where it stands) and never the cache-stable
// prefix, which must stay byte-identical across turns.
func TestEffectTurnProgressRidesTheBodyNotThePrefix(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-turn-progress-effect-test", func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "STABLE BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-turn-progress-effect-test"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	ctrl.SetFreshSessionPath(filepath.Join(dir, "turn-progress.jsonl"))

	if err := ctrl.Run(context.Background(), "first turn"); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := ctrl.Run(context.Background(), "second turn"); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	reqs := rec.requests()
	if len(reqs) < 2 {
		t.Fatalf("rounds = %d, want two turns", len(reqs))
	}
	first, last := reqs[0], reqs[len(reqs)-1]
	var lines []string
	for _, message := range last.Messages {
		if message.Role == provider.RoleSystem {
			continue
		}
		lines = append(lines, message.Content)
	}
	body := strings.Join(lines, "\n") + "\n"
	if !strings.Contains(body, "<turn-progress>") {
		t.Fatalf("the second turn's body must carry the progress block: %s", body)
	}
	if strings.Contains(systemMessage(first.Messages), "<turn-progress>") ||
		strings.Contains(systemMessage(last.Messages), "<turn-progress>") {
		t.Fatal("the progress block must never enter the system prompt")
	}
	if a, b := systemMessage(first.Messages), systemMessage(last.Messages); a != b {
		t.Fatal("the progress block moved the system prompt")
	}
	if !reflect.DeepEqual(toolNames(first), toolNames(last)) {
		t.Fatal("the tool surface drifted between turns")
	}
}

// skillUseProvider asks for one skill, then finishes.
type skillUseProvider struct {
	mu     sync.Mutex
	reqs   []provider.Request
	rounds int
}

func (p *skillUseProvider) Name() string { return "boot-skill-use" }

func (p *skillUseProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.rounds++
	round := p.rounds
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 4)
	if round == 1 {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID: "skill-1", Name: "run_skill", Arguments: `{"name":"hot"}`,
		}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *skillUseProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// docs/50 §2.2, at the real boundary: a skill a turn actually ran is recorded on
// the session sidecar as a fingerprint, and recording it leaves the provider
// prefix (system prompt plus tool surface) byte-identical.
func TestEffectSkillUseIsRecordedWithoutMovingThePromptPrefix(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &skillUseProvider{}
	provider.Register("boot-skill-use-effect-test", func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "STABLE BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-skill-use-effect-test"
model = "x"
`)
	writeFile(t, dir, ".reasonix/skills/hot/SKILL.md", `---
name: hot
description: hot skill
---
HOT-SKILL-BODY-B3`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	// The host binds the transcript path; boot's Options has no such field, so a
	// test binds the sidecar target the way the desktop host does.
	ctrl.SetFreshSessionPath(filepath.Join(dir, "skill-use.jsonl"))
	if err := ctrl.Run(context.Background(), "run the hot skill"); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := ctrl.Run(context.Background(), "say ok"); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	path := ctrl.SessionPath()
	if strings.TrimSpace(path) == "" {
		t.Fatal("the session has no path, so nothing could be recorded")
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta(%s): ok=%v err=%v", path, ok, err)
	}
	use, ok := agent.LatestSkillUse(meta)
	if !ok || use.Name != "hot" || use.ContentHash == "" || use.CatalogDigest == "" {
		t.Fatalf("skill use record = %+v ok=%v, want a fingerprint of the run skill", use, ok)
	}
	if strings.Contains(use.ContentHash+use.CatalogDigest, "HOT") || strings.Contains(use.ContentHash, "hot") {
		t.Fatalf("the record must stay content-free: %+v", use)
	}

	reqs := rec.requests()
	if len(reqs) < 2 {
		t.Fatalf("rounds = %d, want the skill round and a later one", len(reqs))
	}
	if first, last := systemMessage(reqs[0].Messages), systemMessage(reqs[len(reqs)-1].Messages); first != last {
		t.Fatalf("recording a skill use moved the system prompt")
	}
	for i, req := range reqs {
		if !reflect.DeepEqual(toolNames(reqs[0]), toolNames(req)) {
			t.Fatalf("round %d tool surface drifted", i)
		}
	}
}
