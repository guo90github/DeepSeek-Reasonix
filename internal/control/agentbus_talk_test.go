package control

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/event"
)

func newAgentBusTalkController(t *testing.T, dir, participant string) *Controller {
	t.Helper()
	ctrl := New(Options{SessionDir: t.TempDir(), Sink: event.Discard})
	ctrl.SetAgentBus(dir, participant)
	return ctrl
}

func TestSayAskAnswerRoundTripThroughOneBoard(t *testing.T) {
	dir := t.TempDir()
	alice := newAgentBusTalkController(t, dir, "alice")
	bob := newAgentBusTalkController(t, dir, "bob")
	ctx := context.Background()

	if _, err := alice.AgentBusSay(ctx, "design", "thinking out loud", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	correlation, err := alice.AgentBusAsk(ctx, "design", "bob", "which schema?")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if correlation == "" {
		t.Fatal("ask must return the correlation the answer cites")
	}

	bobBlock := bob.agentBusTalkBlock()
	if !strings.Contains(bobBlock, "to=bob") || !strings.Contains(bobBlock, correlation) {
		t.Fatalf("bob's turn missed the ask:\n%s", bobBlock)
	}
	if strings.Contains(bobBlock, "thinking out loud") {
		t.Fatalf("unaddressed talk leaked into bob's turn:\n%s", bobBlock)
	}
	if !strings.Contains(bobBlock, "hidden=1") {
		t.Fatalf("bob must be told how much is being kept from him:\n%s", bobBlock)
	}
	if again := bob.agentBusTalkBlock(); again != "" {
		t.Fatalf("the delta was not consumed:\n%s", again)
	}

	if _, err := bob.AgentBusAnswer(ctx, correlation, "design", "alice", "agentbus-view/1"); err != nil {
		t.Fatalf("answer: %v", err)
	}
	aliceBlock := alice.agentBusTalkBlock()
	if !strings.Contains(aliceBlock, "kind=answer") || !strings.Contains(aliceBlock, correlation) {
		t.Fatalf("alice never saw her answer:\n%s", aliceBlock)
	}

	if err := bob.PostAgentBusResult(agentbus.Result{
		Correlation: correlation, From: "bob", Status: agentbus.ResultAnswered, Text: "agentbus-view/1",
	}); err != nil {
		t.Fatalf("post result: %v", err)
	}
	result, ok, err := alice.ReadAgentBusResult(correlation)
	if err != nil || !ok {
		t.Fatalf("read result = (%v, %v), want the envelope bob published", ok, err)
	}
	if result.Status != agentbus.ResultAnswered || result.Text != "agentbus-view/1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTalkRefusalsArriveTyped(t *testing.T) {
	ctrl := newAgentBusTalkController(t, t.TempDir(), "alice")
	ctrl.SetAgentBusTalkLimits(agentbus.TalkLimits{MaxRounds: 1})
	ctx := context.Background()

	if _, err := ctrl.AgentBusSay(ctx, "t", "one", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	_, err := ctrl.AgentBusSay(ctx, "t", "two", nil)
	if reason, ok := agentbus.IsTalkReject(err); !ok || reason != agentbus.RefuseRounds {
		t.Fatalf("second say = (%v, %q), want rounds_exhausted", ok, reason)
	}
}

func TestTalkRateLimitIsReportedAsRateLimited(t *testing.T) {
	ctrl := newAgentBusTalkController(t, t.TempDir(), "alice")
	ctrl.SetAgentBusTalkLimits(agentbus.TalkLimits{RateWindow: time.Minute, RateMax: 1})

	if _, err := ctrl.AgentBusSay(context.Background(), "t", "one", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	_, err := ctrl.AgentBusSay(context.Background(), "t", "two", nil)
	if reason, ok := agentbus.IsTalkReject(err); !ok || reason != agentbus.RefuseRate {
		t.Fatalf("burst = (%v, %q), want rate_limited", ok, reason)
	}
}

func TestTalkFromAnUnenrolledOrNamelessSessionIsRefused(t *testing.T) {
	unenrolled := New(Options{SessionDir: t.TempDir(), Sink: event.Discard})
	_, err := unenrolled.AgentBusSay(context.Background(), "t", "hello", nil)
	if !errors.Is(err, errAgentBusUnwired) {
		t.Fatalf("unenrolled say = %v, want the unwired refusal", err)
	}
	if block := unenrolled.agentBusTalkBlock(); block != "" {
		t.Fatalf("an unenrolled session must be shown nothing: %q", block)
	}

	nameless := New(Options{SessionDir: t.TempDir(), Sink: event.Discard})
	nameless.SetAgentBus(t.TempDir(), "")
	_, err = nameless.AgentBusSay(context.Background(), "t", "hello", nil)
	if !errors.Is(err, errAgentBusUnwired) {
		t.Fatalf("nameless say = %v, want the unwired refusal", err)
	}
	if block := nameless.agentBusTalkBlock(); block != "" {
		t.Fatalf("a nameless session must be shown nothing: %q", block)
	}
}

func TestTalkBlockBoundsHowMuchOneTurnCarries(t *testing.T) {
	dir := t.TempDir()
	alice := newAgentBusTalkController(t, dir, "alice")
	bob := newAgentBusTalkController(t, dir, "bob")
	ctx := context.Background()
	for i := range agentBusTalkMaxLines + 5 {
		if _, err := alice.AgentBusSay(ctx, "t", "line", []string{"bob"}); err != nil {
			t.Fatalf("say %d: %v", i, err)
		}
	}
	block := bob.agentBusTalkBlock()
	if !strings.Contains(block, "truncated=true") {
		t.Fatalf("a turn must be allowed to carry only so much talk:\n%s", block)
	}
	if got := strings.Count(block, "\nline "); got != agentBusTalkMaxLines {
		t.Fatalf("lines = %d, want the cap %d", got, agentBusTalkMaxLines)
	}
}
