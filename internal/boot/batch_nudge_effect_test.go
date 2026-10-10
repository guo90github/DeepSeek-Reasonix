package boot

// The batching hint at the real boundary: once a run of single read-only rounds
// is visible, the host asks the model to fold the rest into one message, and the
// hint rides the round tail — the cached prefix stays byte-identical.

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// batchNudgeReads is how many files the scripted turn reads, one per round until
// the host points out that they could travel together.
const batchNudgeReads = 8

type batchingProvider struct {
	// complies models a model that acts on the hint; false is the control that
	// keeps issuing one call per round.
	complies bool

	mu      sync.Mutex
	round   int
	read    int
	batched bool
	hinted  int
	reqs    []provider.Request
}

func (*batchingProvider) Name() string { return "boot-loop-batching" }

func (p *batchingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	p.reqs = append(p.reqs, req)
	hinted := false
	for _, msg := range req.Messages {
		if msg.Role != provider.RoleSystem && strings.Contains(msg.Content, "Host batching hint") {
			hinted = true
			break
		}
	}
	if hinted {
		p.hinted++
	}

	ch := make(chan provider.Chunk, batchNudgeReads+2)
	read := func() {
		p.read++
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID:        fmt.Sprintf("read-%d", p.read),
			Name:      "read_file",
			Arguments: fmt.Sprintf(`{"path":"note%d.txt"}`, p.read),
		}}
	}
	switch {
	case p.read >= batchNudgeReads:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	case p.complies && hinted && !p.batched:
		p.batched = true
		for p.read < batchNudgeReads {
			read()
		}
	default:
		read()
	}
	p.mu.Unlock()

	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *batchingProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// TestEffectBatchingHintShortensTheTurnWithoutMovingThePrefix measures the hint
// twice on the same task: a control model that ignores it keeps one read per
// round, a model that acts on it folds the rest into one message and ends the
// turn rounds sooner. Either way the system message and the tool schemas the
// provider sees are unchanged between the first and the last round.
func TestEffectBatchingHintShortensTheTurnWithoutMovingThePrefix(t *testing.T) {
	run := func(t *testing.T, complies bool) (rounds, hinted int, reqs []provider.Request) {
		t.Helper()
		isolateConfigHome(t)
		dir := robustTempDir(t)
		t.Chdir(dir)

		for i := 1; i <= batchNudgeReads; i++ {
			writeFile(t, dir, fmt.Sprintf("note%d.txt", i), "alpha\n")
		}
		rec := &batchingProvider{complies: complies}
		kind := fmt.Sprintf("boot-loop-batching-%v", complies)
		provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
		writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

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
		if err := ctrl.Run(context.Background(), "read every note file"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		reqs = rec.requests()
		return len(reqs), rec.hinted, reqs
	}

	controlRounds, controlHinted, controlReqs := run(t, false)
	compliantRounds, compliantHinted, compliantReqs := run(t, true)
	t.Logf("MEASURED: single-read rounds %d (hint seen %d×) vs batched %d (hint seen %d×) for %d files",
		controlRounds, controlHinted, compliantRounds, compliantHinted, batchNudgeReads)

	if controlHinted == 0 || compliantHinted == 0 {
		t.Fatalf("the hint never reached the model: control=%d compliant=%d", controlHinted, compliantHinted)
	}
	if controlRounds != batchNudgeReads+1 {
		t.Fatalf("the control turn should be one round per read plus the closing turn: %d", controlRounds)
	}
	if compliantRounds >= controlRounds {
		t.Fatalf("acting on the hint did not shorten the turn: control=%d compliant=%d", controlRounds, compliantRounds)
	}
	for i, reqs := range [][]provider.Request{controlReqs, compliantReqs} {
		last := reqs[len(reqs)-1]
		if len(reqs[0].Messages) == 0 || len(last.Messages) == 0 {
			t.Fatalf("run %d: a request carried no messages", i)
		}
		if !reflect.DeepEqual(reqs[0].Messages[0], last.Messages[0]) {
			t.Fatalf("run %d: the system message moved between rounds", i)
		}
		if !reflect.DeepEqual(reqs[0].Tools, last.Tools) {
			t.Fatalf("run %d: the tool schemas moved between rounds", i)
		}
		if strings.Contains(last.Messages[0].Content, "Host batching hint") {
			t.Fatalf("run %d: the hint landed in the prefix", i)
		}
	}
}
