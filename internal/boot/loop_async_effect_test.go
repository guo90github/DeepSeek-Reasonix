package boot

// Requirement 9 at the real boundary: with the configured tier a slow shell call
// runs as a background job, so the batch settles at once instead of holding the
// loop (baseline measured here: 2137 ms). The budget counts host dispatch only.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type threadedProvider struct {
	mu    sync.Mutex
	round int
	at    []time.Time
	reqs  []provider.Request
}

func (p *threadedProvider) Name() string { return "boot-loop-async" }

func (p *threadedProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.at = append(p.at, time.Now())
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 3)
	switch round {
	case 1:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "slow", Name: "bash", Arguments: `{"command":"sleep 2"}`}}
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "later", Name: "read_file", Arguments: `{"path":"note.txt"}`}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *threadedProvider) rounds() ([]time.Time, []provider.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.at...), append([]provider.Request(nil), p.reqs...)
}

// TestEffectSlowShellCallDoesNotHoldTheLoop is the effect the tier owes: the
// round settles while the slow call keeps running as a job.
func TestEffectSlowShellCallDoesNotHoldTheLoop(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &threadedProvider{}
	provider.Register("boot-loop-async", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "note.txt", "alpha\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
shell_async = "fast"

[[providers]]
name = "test-model"
kind = "boot-loop-async"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "run the slow thing and read the note"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	at, reqs := rec.rounds()
	if len(at) < 2 {
		t.Fatalf("run stopped after %d rounds", len(at))
	}
	held := at[1].Sub(at[0])
	t.Logf("MEASURED: with shell_async=fast the round after a `sleep 2` batch arrived %.0f ms later", held.Seconds()*1000)
	if held >= time.Second {
		t.Fatalf("the round waited %.0f ms for a `sleep 2`; the host-side step must stay under 1000 ms", held.Seconds()*1000)
	}

	slow, ok := toolResultFor(reqs[1], "slow")
	if !ok || !strings.Contains(slow, "Started background job") {
		t.Fatalf("the slow call did not start as a job: ok=%v %q", ok, slow)
	}
	later, ok := toolResultFor(reqs[1], "later")
	if !ok || !strings.Contains(later, "moved to the background") {
		t.Fatalf("the call behind the job was not deferred: ok=%v %q", ok, later)
	}
	if body, err := os.ReadFile(filepath.Join(dir, "note.txt")); err != nil || !strings.Contains(string(body), "alpha") {
		t.Fatalf("fixture file = %q err=%v", body, err)
	}
}

// tierProvider issues one batch: the case's command, then a later call.
type tierProvider struct {
	command string
	mu      sync.Mutex
	round   int
	at      []time.Time
	reqs    []provider.Request
}

func (p *tierProvider) Name() string { return "boot-loop-tier" }

func (p *tierProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.at = append(p.at, time.Now())
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 3)
	switch round {
	case 1:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "first", Name: "bash", Arguments: `{"command":` + strconv.Quote(p.command) + `}`}}
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "later", Name: "read_file", Arguments: `{"path":"note.txt"}`}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *tierProvider) rounds() ([]time.Time, []provider.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.at...), append([]provider.Request(nil), p.reqs...)
}

// TestEffectBalancedTierOnlyLiftsChecks measures what the middle tier really
// buys, because that decides whether it can be the default: a check-shaped
// command stops holding the loop, an ordinary one keeps its place.
func TestEffectBalancedTierOnlyLiftsChecks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		lifted  bool
	}{
		{name: "check", command: "go vet ./...", lifted: true},
		{name: "ordinary", command: "sleep 2", lifted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfigHome(t)
			dir := robustTempDir(t)
			t.Chdir(dir)

			rec := &tierProvider{command: tc.command}
			kind := "boot-loop-tier-" + tc.name
			provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
			writeFile(t, dir, "note.txt", "alpha\n")
			writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
shell_async = "balanced"

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
			if err := ctrl.Run(context.Background(), "run it and read the note"); err != nil {
				t.Fatalf("Run: %v", err)
			}

			at, reqs := rec.rounds()
			if len(at) < 2 {
				t.Fatalf("run stopped after %d rounds", len(at))
			}
			held := at[1].Sub(at[0])
			t.Logf("MEASURED: balanced + %q held the round for %.0f ms", tc.command, held.Seconds()*1000)

			first, ok := toolResultFor(reqs[1], "first")
			if !ok {
				t.Fatal("the first call produced no result")
			}
			lifted := strings.Contains(first, "Started background job")
			if lifted != tc.lifted {
				t.Fatalf("lifted=%v, want %v for %q: %q", lifted, tc.lifted, tc.command, first)
			}
			if tc.lifted && held >= time.Second {
				t.Fatalf("a lifted check still held the round for %.0f ms", held.Seconds()*1000)
			}
			if !tc.lifted && held < 1800*time.Millisecond {
				t.Fatalf("an ordinary command was not left in the foreground (%.0f ms)", held.Seconds()*1000)
			}
			later, ok := toolResultFor(reqs[1], "later")
			if !ok {
				t.Fatalf("the later call produced no result")
			}
			deferred := strings.Contains(later, "moved to the background")
			if deferred != tc.lifted {
				t.Fatalf("later deferred=%v, want %v: %q", deferred, tc.lifted, later)
			}
		})
	}
}
