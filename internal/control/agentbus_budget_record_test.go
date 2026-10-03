package control

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// captureHandler keeps the records a host wrote, so a test can assert what a person reading
// the log would see.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) find(message string) (slog.Record, map[string]string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, record := range h.records {
		if !strings.Contains(record.Message, message) {
			continue
		}
		attrs := map[string]string{}
		record.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.String()
			return true
		})
		return record, attrs, true
	}
	return slog.Record{}, nil, false
}

// Refusals are counted, not replaced: the panel's row is a tally of how often the host had to say
// "not now", so a second refusal at the same level has to add up — and Total has to follow it.
func TestBudgetRefusalsAccumulatePerLevel(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ctrl := newAgentBusTalkController(t, dir, "bob")
	ctrl.SetAgentBusLedger(agentbus.NewLedger(agentbus.BudgetLimits{Node: 1}))

	baseline := AgentBusBudgetRefusals()
	// Two different nodes, so nothing can collapse the second attempt into the first: both are
	// refused by the same level, and both have to be counted.
	for _, node := range []string{"step", "step-2"} {
		if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert(node, "bob")); err != nil {
			t.Fatalf("assert %s: %v", node, err)
		}
		_, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbClaim, Node: node, Actor: "bob",
			Bounds:   &board.Bounds{Steps: 5},
			Deadline: time.Now().UTC().Add(time.Hour),
		})
		if reason, refused := agentbus.IsBudgetReject(err); !refused || reason != agentbus.RefuseBudgetNode {
			t.Fatalf("claim %s = %v, want a node-budget refusal", node, err)
		}
	}

	got := AgentBusBudgetRefusals()
	if delta := got.Node - baseline.Node; delta != 2 {
		t.Fatalf("node refusals +%d, want both refusals counted rather than one replacing the other", delta)
	}
	if delta := got.Total() - baseline.Total(); delta != 2 {
		t.Fatalf("total refusals +%d, want the tally to follow the level", delta)
	}
}

// A budget refusal has to reach the host's own record, naming the ceiling that refused: the
// tool result tells the model, and a brake nobody can see is not a brake (G3/T12-3).
func TestABudgetRefusalIsRecordedWithItsCeiling(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ctrl := newAgentBusTalkController(t, dir, "bob")
	ctrl.SetAgentBusLedger(agentbus.NewLedger(agentbus.BudgetLimits{Node: 1}))

	previous := slog.Default()
	records := &captureHandler{}
	slog.SetDefault(slog.New(records))
	t.Cleanup(func() { slog.SetDefault(previous) })
	refusedAt := AgentBusBudgetRefusals()

	if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert("step", "bob")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	_, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "step", Actor: "bob",
		Bounds:   &board.Bounds{Steps: 5},
		Deadline: time.Now().UTC().Add(time.Hour),
	})
	if reason, refused := agentbus.IsBudgetReject(err); !refused || reason != agentbus.RefuseBudgetNode {
		t.Fatalf("claim err = %v, want a node-budget refusal", err)
	}

	_, attrs, ok := records.find("budget ceiling")
	if !ok {
		t.Fatalf("no host record of the refusal: %+v", records.records)
	}
	for key, want := range map[string]string{
		"level":     "node",
		"reason":    agentbus.RefuseBudgetNode,
		"board":     filepath.Base(dir),
		"node":      "step",
		"limit":     "1",
		"remaining": "1", // the refusal spends nothing, so the whole ceiling is still there
	} {
		if attrs[key] != want {
			t.Fatalf("record %s = %q, want %q (record: %+v)", key, attrs[key], want, attrs)
		}
	}
	if !strings.Contains(attrs["key"], "node:") {
		t.Fatalf("record key = %q, want the bucket that refused", attrs["key"])
	}
	if attrs["refusals"] == "" || attrs["refusals"] == "0" {
		t.Fatalf("record refusals = %q, want a running count", attrs["refusals"])
	}
	// The level is what a person has to move, so the host counts it per level — that is what
	// the collaboration panel shows when budget, not time, is what stopped the work (G3).
	if got := AgentBusBudgetRefusals(); got.Node != refusedAt.Node+1 || got.Total() != refusedAt.Total()+1 {
		t.Fatalf("refusal counts = %+v, want the node level up by one from %+v", got, refusedAt)
	}
}
