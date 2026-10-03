package recap

import (
	"context"
	"testing"
	"time"
)

func TestRunnerCoalescesAndDrains(t *testing.T) {
	ctx := context.Background()
	prov := &fakeProvider{name: "fake", answer: goodAnswer, delay: 10 * time.Millisecond}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	runner := NewRunner(h.generator)

	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	if !runner.Submit(path) {
		t.Fatal("first submit must be accepted")
	}
	if runner.Submit(path) {
		t.Fatal("a queued path must not be queued twice")
	}
	for i := range 3 {
		if !runner.Submit(h.session(t, "2026010"+string(rune('2'+i))+"-000000.000000000-fake.jsonl", "two\n")) {
			t.Fatalf("submit %d must be accepted", i)
		}
	}
	runner.Close()

	records, pending, err := h.store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if records != 4 || pending != 0 {
		t.Fatalf("after drain: records=%d pending=%d, want 4 and 0", records, pending)
	}
	if runner.Submit(path) {
		t.Fatal("a closed runner must not accept work")
	}
}

func TestRunnerGeneratesEachSessionOnce(t *testing.T) {
	prov := &fakeProvider{name: "fake", answer: goodAnswer}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	runner := NewRunner(h.generator)
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	runner.Submit(path)
	runner.Close()
	runner2 := NewRunner(h.generator)
	runner2.Submit(path)
	runner2.Close()
	if calls, _ := prov.counters(); calls != 1 {
		t.Fatalf("provider calls = %d, want 1 across two drains of an unchanged session", calls)
	}
}
