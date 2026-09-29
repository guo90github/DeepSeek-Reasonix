package recap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/provider"
)

// parkedProvider blocks every call until the test releases it, so a close path
// can be measured while a recap call is genuinely in flight.
type parkedProvider struct {
	answer string
	enters chan struct{}
	leave  chan struct{}
}

func (p *parkedProvider) Name() string { return "parked" }

func (p *parkedProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	select {
	case p.enters <- struct{}{}:
	default:
	}
	select {
	case <-p.leave:
	case <-ctx.Done():
		ch := make(chan provider.Chunk)
		close(ch)
		return ch, nil
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: p.answer}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	close(ch)
	return ch, nil
}

func parkedHarness(t *testing.T) (*harness, *parkedProvider, []string) {
	t.Helper()
	parked := &parkedProvider{
		answer: goodAnswer,
		enters: make(chan struct{}, 8),
		leave:  make(chan struct{}),
	}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: parked, ref: "fake/model", ok: true}
	})
	paths := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		name := fmt.Sprintf("20260101-%06d.000000000-fake.jsonl", i)
		path := filepath.Join(h.dir, name)
		if err := os.WriteFile(path, []byte("body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return h, parked, paths
}

// A session that just ended must never wait for its recap: Stop returns while the
// model call is still parked.
func TestRunnerStopDoesNotWaitForTheModel(t *testing.T) {
	h, parked, paths := parkedHarness(t)
	runner := NewRunner(h.generator)
	if !runner.Submit(paths[0]) {
		t.Fatal("submit must be accepted")
	}
	select {
	case <-parked.enters:
	case <-time.After(5 * time.Second):
		t.Fatal("recap call never started")
	}
	started := time.Now()
	runner.Stop()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Stop waited %s for a parked model call", elapsed)
	}
	close(parked.leave)
}

// Closing many tabs at once must not drop sessions silently: what the queue
// cannot take is recorded as pending for a later sweep.
func TestRunnerOverflowLeavesPending(t *testing.T) {
	ctx := context.Background()
	h, parked, paths := parkedHarness(t)
	runner := NewRunner(h.generator)
	defer func() {
		close(parked.leave)
		runner.Stop()
	}()
	rejected := 0
	for _, path := range paths {
		if !runner.Submit(path) {
			rejected++
		}
	}
	if rejected == 0 {
		t.Fatalf("expected the queue to overflow at %d paths", len(paths))
	}
	records, pending, err := h.store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if records != 0 {
		t.Fatalf("records = %d, want none while calls are parked", records)
	}
	if pending < rejected {
		t.Fatalf("pending = %d, want at least one marker per rejected submit (%d)", pending, rejected)
	}
}
