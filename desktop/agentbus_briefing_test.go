package main

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

func TestAgentBusBriefingBindingShapesWhatThePanelDraws(t *testing.T) {
	dir := t.TempDir()
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	ctrl.SetAgentBus(dir, "alice")
	if _, err := ctrl.OpenAgentBusHearing(context.Background(), "design", []string{"alice"}); err != nil {
		t.Fatalf("open hearing: %v", err)
	}
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: ctrl}}, activeTabID: "t1"}

	view, err := app.AgentBusBriefing()
	if err != nil {
		t.Fatalf("briefing: %v", err)
	}
	if view.Participant != "alice" {
		t.Fatalf("participant = %q, want the identity the panel shows", view.Participant)
	}
	if len(view.Cards) != 1 || view.Cards[0].Subtree != "design" || view.Cards[0].Disputed != 1 {
		t.Fatalf("cards = %+v, want the subtree under deliberation", view.Cards)
	}
	// Found by kind, not counted: the host appends its own rows (a refusal record) to this
	// list, so a fixed length would make the panel's content depend on test order.
	disputed := signalOfKind(view.Signals, "disputed")
	if disputed == nil {
		t.Fatalf("signals = %+v, want the drill-in row", view.Signals)
	}
	if disputed.Node != "design" || disputed.Detail == "" {
		t.Fatalf("signal = %+v, want an address and a reason", *disputed)
	}
}

func signalOfKind(signals []AgentBusSignalView, kind string) *AgentBusSignalView {
	for i := range signals {
		if signals[i].Kind == kind {
			return &signals[i]
		}
	}
	return nil
}

func TestAgentBusBriefingBindingSaysWhyItHasNothing(t *testing.T) {
	if _, err := (&App{}).AgentBusBriefing(); err == nil {
		t.Fatal("no active session must be reported, not shown as an empty board")
	}
	offBoard := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: offBoard}}, activeTabID: "t1"}
	if _, err := app.AgentBusBriefing(); err == nil {
		t.Fatal("a session that never joined a board must say so")
	}
}

// A brake a person cannot see is not a brake (G3): when the host's account turned work down, the
// panel's own signal list says which ceiling did it and how often. Nothing refused, no row.
func TestBudgetRefusalSignalNamesTheCeilingThatRefused(t *testing.T) {
	if _, refused := budgetRefusalSignal(control.BudgetRefusalCounts{}); refused {
		t.Fatal("a host that refused nothing must add no row")
	}
	signal, refused := budgetRefusalSignal(control.BudgetRefusalCounts{Node: 2, Turn: 1, Slots: 1})
	if !refused {
		t.Fatal("refusals must reach the panel")
	}
	if signal.Kind != "budget" {
		t.Fatalf("kind = %q, want the panel's budget row", signal.Kind)
	}
	// Slots is in the list because a full host parks work without ever claiming it: that
	// refusal is invisible everywhere else, so the row has to carry it.
	for _, want := range []string{"4", "node 2", "turn 1", "slots 1"} {
		if !strings.Contains(signal.Detail, want) {
			t.Fatalf("detail = %q, want it to name %q", signal.Detail, want)
		}
	}
	if strings.Contains(signal.Detail, "board") {
		t.Fatalf("detail = %q, want only the levels that refused", signal.Detail)
	}
}

// The process total says rate limits are happening at all; the lane is what a multi-provider
// operator has to act on. Nothing absorbed means no row, and a 429 the provider did not name is
// counted in the total without ever being attributed to a guess.
func TestRateLimitSignalNamesTheLaneThatWasThrottled(t *testing.T) {
	if _, throttled := rateLimitSignal(nil, 0); throttled {
		t.Fatal("a host that rode out nothing must add no row")
	}
	signal, throttled := rateLimitSignal(map[string]int64{"deepseek": 3, "other": 1}, 5)
	if !throttled {
		t.Fatal("absorbed 429s must reach the panel")
	}
	if signal.Kind != "rate_limited" {
		t.Fatalf("kind = %q, want the panel's rate-limit row", signal.Kind)
	}
	for _, want := range []string{"5", "deepseek 3", "other 1"} {
		if !strings.Contains(signal.Detail, want) {
			t.Fatalf("detail = %q, want it to name %q", signal.Detail, want)
		}
	}
	unnamed, throttled := rateLimitSignal(map[string]int64{}, 2)
	if !throttled || strings.Contains(unnamed.Detail, ":") {
		t.Fatalf("detail = %q, want the total with no lane list", unnamed.Detail)
	}
}

// A wake that reached nobody is a brake like a spent ceiling: the board stands still and the
// sender’s log is the only witness. The panel gets the participant and the reason (G3/T12-3).
func TestWakeFailureSignalNamesWhoCouldNotBeReached(t *testing.T) {
	if _, failed := wakeFailureSignal(control.WakeFailures{}); failed {
		t.Fatal("a host that delivered every wake must add no row")
	}
	signal, failed := wakeFailureSignal(control.WakeFailures{Count: 2, Last: "alice: no address owns it"})
	if !failed {
		t.Fatal("an undelivered wake must reach the panel")
	}
	if signal.Kind != "wake_undelivered" {
		t.Fatalf("kind = %q, want the panel’s wake row", signal.Kind)
	}
	for _, want := range []string{"2", "alice", "no address owns it"} {
		if !strings.Contains(signal.Detail, want) {
			t.Fatalf("detail = %q, want it to name %q", signal.Detail, want)
		}
	}
}

// A move the board's own rate ceiling turned down lands nothing, so the panel is the only place
// a person can see that the ceiling is biting. Its row must not be confused with the provider
// 429 row, which already owns the kind "rate_limited".
func TestNodeRateSignalNamesTheCeilingThatRefused(t *testing.T) {
	if _, throttled := nodeRateSignal(control.NodeRateRefusals{}); throttled {
		t.Fatal("a host whose ceiling refused nothing must add no row")
	}
	signal, throttled := nodeRateSignal(control.NodeRateRefusals{Count: 3, Last: "build"})
	if !throttled {
		t.Fatal("a refused move must reach the panel")
	}
	if signal.Kind != "node_rate" {
		t.Fatalf("kind = %q, want the board's own rate row rather than the provider's", signal.Kind)
	}
	for _, want := range []string{"3", "build"} {
		if !strings.Contains(signal.Detail, want) {
			t.Fatalf("detail = %q, want it to name %q", signal.Detail, want)
		}
	}
}
