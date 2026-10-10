package boot

// The product path end to end: a tier set through the config API and saved to
// the user config has to reach the run. The renderer used to drop the key, so
// the switch saved nothing and every session kept the default.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// persistProvider asks for a slow command and an independent read in one batch.
type persistProvider struct {
	mu    sync.Mutex
	round int
	reqs  []provider.Request
}

func (*persistProvider) Name() string { return "boot-loop-persist" }

func (p *persistProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 3)
	if round == 1 {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "slow", Name: "bash", Arguments: `{"command":"sleep 2"}`}}
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "later", Name: "read_file", Arguments: `{"path":"note.txt"}`}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *persistProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// saveTier writes the tier the way the desktop switch does: load the user
// config, set it, save it. A project file supplies the model, since the tier is
// a user-global setting. kind names the registered provider so each subtest gets
// its own (registration is process-wide).
func saveTier(t *testing.T, dir, tier, kind string) {
	t.Helper()
	writeFile(t, dir, "note.txt", "alpha\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	userPath := config.UserConfigPath()
	if userPath == "" {
		t.Fatal("the fixture has no user config path")
	}
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte("config_version = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	unlock := config.LockUserConfigEdits()
	defer unlock()
	cfg := config.LoadForEdit(userPath)
	if err := cfg.SetAgentShellAsync(tier); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTo(userPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
}

// TestEffectSavedTierReachesTheRun pins that path: the saved tier decides
// whether a slow shell call leaves the critical path.
func TestEffectSavedTierReachesTheRun(t *testing.T) {
	for _, tc := range []struct {
		tier   string
		lifted bool
	}{
		{tier: "fast", lifted: true},
		{tier: "off", lifted: false},
	} {
		t.Run(tc.tier, func(t *testing.T) {
			isolateConfigHome(t)
			dir := robustTempDir(t)
			t.Chdir(dir)
			kind := "boot-loop-persist-" + tc.tier
			saveTier(t, dir, tc.tier, kind)

			rec := &persistProvider{}
			provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })

			ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			defer ctrl.Close()
			if err := ctrl.Run(context.Background(), "run the slow thing and read the note"); err != nil {
				t.Fatalf("Run: %v", err)
			}

			reqs := rec.requests()
			if len(reqs) < 2 {
				t.Fatalf("run stopped after %d rounds", len(reqs))
			}
			slow, ok := toolResultFor(reqs[1], "slow")
			if !ok {
				t.Fatal("the slow call produced no result")
			}
			if lifted := strings.Contains(slow, "Started background job"); lifted != tc.lifted {
				t.Fatalf("tier %q lifted=%v, want %v: %q", tc.tier, lifted, tc.lifted, slow)
			}
		})
	}
}
