package boot

// Effect test for the post-write re-read: a turn that edits the line it just
// wrote must survive the real Build assembly, not only the agent harness.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type writeThenEditProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *writeThenEditProvider) Name() string { return "boot-write-reread" }

func (p *writeThenEditProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	round := len(p.reqs)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 3)
	call := func(id, name, args string) {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: id, Name: name, Arguments: args}}
	}
	switch round {
	case 1:
		call("r1", "read_file", `{"path":"note.txt"}`)
	case 2:
		call("e1", "edit_file", `{"path":"note.txt","old_string":"beta","new_string":"delta"}`)
	case 3:
		call("e2", "edit_file", `{"path":"note.txt","old_string":"delta","new_string":"epsilon"}`)
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *writeThenEditProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

func toolResultFor(req provider.Request, callID string) (string, bool) {
	for _, msg := range req.Messages {
		if msg.Role == provider.RoleTool && msg.ToolCallID == callID {
			return msg.Content, true
		}
	}
	return "", false
}

func TestEffectSecondEditOfTheLineAWriteProducedNeedsNoReadRound(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &writeThenEditProvider{}
	provider.Register("boot-write-reread", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "note.txt", "alpha\nbeta\ngamma\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "boot-write-reread"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "edit the note twice"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	reqs := rec.requests()
	if len(reqs) < 4 {
		t.Fatalf("run stopped after %d rounds", len(reqs))
	}
	last := reqs[len(reqs)-1]
	first, ok := toolResultFor(last, "e1")
	if !ok || strings.Contains(first, "blocked:") || strings.Contains(first, "error:") {
		t.Fatalf("the first edit did not land: ok=%v %q", ok, first)
	}
	second, ok := toolResultFor(last, "e2")
	if !ok {
		t.Fatalf("the second edit produced no result in the real assembly")
	}
	if strings.Contains(second, "evidence required") {
		t.Fatalf("the second edit owed a read round through the real Build stack: %q", second)
	}
	body, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "epsilon") {
		t.Fatalf("note.txt = %q, want the second edit applied", body)
	}
}
