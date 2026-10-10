package boot

// The turn readout at the real boundary: a turn that took several model rounds
// reports where its time went, and a quick one stays silent. Nothing renders by
// hand here — the notice rides the existing event channel.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

const turnMetricsReads = 5

type metricsProvider struct {
	reads int

	mu    sync.Mutex
	round int
}

func (*metricsProvider) Name() string { return "boot-loop-metrics" }

func (p *metricsProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 2)
	if round <= p.reads {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID:        fmt.Sprintf("read-%d", round),
			Name:      "read_file",
			Arguments: fmt.Sprintf(`{"path":"note%d.txt"}`, round),
		}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// noticeRecorder collects the notices a run emits.
type noticeRecorder struct {
	mu      sync.Mutex
	details []string
}

func (r *noticeRecorder) sink() event.Sink {
	return event.FuncSink(func(e event.Event) {
		if e.Kind != event.Notice || e.Code != event.NoticeCodeTurnMetrics {
			return
		}
		r.mu.Lock()
		r.details = append(r.details, e.Detail)
		r.mu.Unlock()
	})
}

func (r *noticeRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.details...)
}

// TestEffectLongTurnReportsWhereItsTimeWent runs the same kind of turn twice:
// one that needs several rounds reports its metrics once, a single-round turn
// reports nothing.
func TestEffectLongTurnReportsWhereItsTimeWent(t *testing.T) {
	run := func(t *testing.T, reads int) []string {
		t.Helper()
		isolateConfigHome(t)
		dir := robustTempDir(t)
		t.Chdir(dir)

		for i := 1; i <= reads; i++ {
			writeFile(t, dir, fmt.Sprintf("note%d.txt", i), "alpha\n")
		}
		rec := &metricsProvider{reads: reads}
		kind := fmt.Sprintf("boot-loop-metrics-%d", reads)
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

		notices := &noticeRecorder{}
		ctrl, err := Build(context.Background(), Options{Sink: notices.sink()})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		defer ctrl.Close()
		if err := ctrl.Run(context.Background(), "read every note file"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return notices.recorded()
	}

	long := run(t, turnMetricsReads)
	if len(long) != 1 {
		t.Fatalf("a %d-round turn reported %d metrics notices, want exactly 1: %v", turnMetricsReads, len(long), long)
	}
	t.Logf("MEASURED: turn metrics notice detail = %q", long[0])
	for _, want := range []string{"rounds=", "wall=", "model=", "tools="} {
		if !strings.Contains(long[0], want) {
			t.Fatalf("detail %q is missing %q", long[0], want)
		}
	}

	if short := run(t, 1); len(short) != 0 {
		t.Fatalf("a single-round turn reported %d metrics notices: %v", len(short), short)
	}
}
