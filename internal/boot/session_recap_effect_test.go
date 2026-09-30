package boot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

const recapPromptMarker = "distill one finished coding session"

type recapRecordingProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *recapRecordingProvider) Name() string { return "boot-effect-recap" }

func (p *recapRecordingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: `[{"kind":"fact","body":"the thing is done"}]`}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	close(ch)
	return ch, nil
}

// recapRequests returns only the requests the recap lane sent, identified by its
// fixed system policy.
func (p *recapRecordingProvider) recapRequests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []provider.Request
	for _, req := range p.reqs {
		if len(req.Messages) == 0 {
			continue
		}
		if strings.Contains(req.Messages[0].Content, recapPromptMarker) {
			out = append(out, req)
		}
	}
	return out
}

// Closing a session must not wait for the recap model, and the recap that follows
// must read the closed session's own transcript. Both halves pin the async lane at
// the boundary the close path actually touches.
func TestEffectClosingASessionRecapsInTheBackground(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	t.Cleanup(CloseRecapLanes)

	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessions, "20260101-000000.000000000-test-model.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "please recap the thing"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "done: the thing is done"})
	if err := session.Save(sessionPath); err != nil {
		t.Fatalf("save session: %v", err)
	}

	rec := &recapRecordingProvider{}
	kind := "boot-effect-recap"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
session_recap_model = "test-model"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard, SessionDir: sessions})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// A headless stack holds no write lease, so it never assigns itself a session
	// path; the turn only marks the controller started (which is what gates
	// SessionEnd), and the path is supplied explicitly.
	if err := ctrl.Run(context.Background(), "please recap the thing"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ctrl.SetSessionPath(sessionPath)

	started := time.Now()
	ctrl.Close()
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("closing waited %s; the recap lane must be async", elapsed)
	}

	deadline := time.Now().Add(20 * time.Second)
	var recaps []provider.Request
	for time.Now().Before(deadline) {
		if recaps = rec.recapRequests(); len(recaps) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(recaps) == 0 {
		t.Fatal("no recap request reached the provider boundary after close")
	}
	if len(recaps[0].Messages) < 2 || !strings.Contains(recaps[0].Messages[1].Content, "please recap the thing") {
		t.Fatalf("recap did not read the closed session's transcript: %+v", recaps[0].Messages)
	}
	if len(recaps[0].Tools) != 0 {
		t.Fatalf("recap lane must run without tools, got %d", len(recaps[0].Tools))
	}
}

// A manual generation has to reach the same lane the close path uses — that is
// the only reason to queue instead of calling the model here. So this pins the
// manual entry at the provider boundary and in the projection: a session that
// never closed must still end up with a stored recap.
func TestEffectManualGenerationUsesTheSameLane(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	t.Cleanup(CloseRecapLanes)

	if EnqueueSessionRecap("   ") {
		t.Fatal("an empty path must not be queued")
	}
	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessions, "20260101-000000.000000000-test-model.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "please recap the thing"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "done: the thing is done"})
	if err := session.Save(sessionPath); err != nil {
		t.Fatalf("save session: %v", err)
	}

	rec := &recapRecordingProvider{}
	const kind = "boot-effect-manual"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
session_recap_model = "test-model"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	// Building the lane also runs the projection's maintenance once per process.
	// Seeding a stale record proves the wiring, not just the SQL: nothing else in
	// this test would ever remove it.
	ctx := context.Background()
	seed, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		t.Fatalf("open projection: %v", err)
	}
	stalePath := filepath.Join(sessions, "20240101-000000.000000000-test-model.jsonl")
	if err := seed.Put(ctx, recap.Record{Path: stalePath, Fingerprint: "f",
		PromptVersion: recap.PromptVersion, GeneratedAt: time.Now().Add(-365 * 24 * time.Hour)}); err != nil {
		t.Fatalf("seed stale record: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close projection: %v", err)
	}

	if _, err := Build(context.Background(), Options{Sink: event.Discard, SessionDir: sessions}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	after, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		t.Fatalf("reopen projection: %v", err)
	}
	if _, ok, _ := after.Get(ctx, stalePath); ok {
		t.Fatal("a record past the retention window survived the lane's maintenance")
	}
	_ = after.Close()
	if !EnqueueSessionRecap(sessionPath) {
		t.Fatal("the manual entry refused a session while the lane was running")
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(rec.recapRequests()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(rec.recapRequests()) == 0 {
		t.Fatal("no recap request reached the provider boundary after a manual generate")
	}

	store, err := recap.Open(context.Background(), recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		t.Fatalf("open projection: %v", err)
	}
	defer func() { _ = store.Close() }()
	stored := false
	for time.Now().Before(deadline) {
		if _, ok, err := store.Get(context.Background(), sessionPath); err == nil && ok {
			stored = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !stored {
		t.Fatal("a manually generated recap was never stored")
	}
}
