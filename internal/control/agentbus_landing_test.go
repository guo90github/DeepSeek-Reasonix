package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/agentbus/board"
)

func readinessIDs(result agent.ReadinessResult) string { return strings.Join(result.Missing, ",") }

// T9-5: a session on a board may not call the task complete while a deliverable is
// unfinished or a conclusion is still in doubt. The board's verdict rides the readiness
// the Goal already answers to, so every existing rule — including "a repeated complete
// claim may finish on leftover checks" — applies to it unchanged (AGENT_BUS §13.10).
func TestBoardLandingIsAReadinessRequirement(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ctrl := newAgentBusTalkController(t, dir, "orchestrator")

	// An empty board has claimed nothing, so it has not landed.
	got := ctrl.withBoardLanding(ctx, agent.ReadinessResult{Ready: true})
	if got.Ready {
		t.Fatalf("an empty board must not read as landed: %+v", got)
	}
	if !strings.Contains(readinessIDs(got), agentBusLandingID) {
		t.Fatalf("missing = %q, want the board category", readinessIDs(got))
	}
	if strings.TrimSpace(got.Reason) == "" {
		t.Fatal("the refusal must carry a reason whoever reads it can act on")
	}

	// A finished, undisputed deliverable is a landed task.
	verify := []board.Evidence{busEvidence("verification", "go test ./internal/agentbus/...")}
	applyBusOps(t, dir,
		board.Op{Verb: board.VerbAssert, Node: "publish", Actor: "worker", Evidence: verify},
		board.Op{
			Verb: board.VerbDecide, Node: "publish", Actor: "worker",
			Outcome: board.OutcomeDone, ReproducedBy: "verifier", Evidence: verify,
		},
	)
	if got := ctrl.withBoardLanding(ctx, agent.ReadinessResult{Ready: true}); !got.Ready {
		t.Fatalf("a done, undisputed deliverable is a landed task: %+v", got)
	}

	// A refutation puts that conclusion back in doubt.
	if _, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbRefute, Node: "publish", Actor: "skeptic",
		Reason: "the test does not cover the migration", Evidence: verify,
	}); err != nil {
		t.Fatalf("refute: %v", err)
	}
	got = ctrl.withBoardLanding(ctx, agent.ReadinessResult{Ready: true})
	if got.Ready {
		t.Fatal("a contested node must not allow a complete claim")
	}
	if !strings.Contains(got.Reason, "contested") {
		t.Fatalf("reason = %q, want it to name the dispute", got.Reason)
	}
	// A board that has not landed is not a leftover check, so a repeated complete claim
	// may not finish on it either.
	if repeatedCompleteMayFinish(got.Missing, nil) {
		t.Fatal("a board that has not landed must never be waved through")
	}

	// What the host already found missing is kept, not replaced.
	both := ctrl.withBoardLanding(ctx, agent.ReadinessResult{
		Missing: []string{"verification"}, Reason: "verification missing",
	})
	if !strings.Contains(readinessIDs(both), "verification") || !strings.Contains(readinessIDs(both), agentBusLandingID) {
		t.Fatalf("missing = %q, want the host's and the board's", readinessIDs(both))
	}
	if !strings.Contains(both.Reason, "verification missing") || !strings.Contains(both.Reason, "board:") {
		t.Fatalf("reason = %q, want both reasons", both.Reason)
	}
}

// A session that is not on any board composes unchanged (T4-4: unwired means no change).
func TestBoardLandingLeavesAnOffBoardSessionAlone(t *testing.T) {
	ctrl := newAgentBusTestController(t)
	got := ctrl.withBoardLanding(context.Background(), agent.ReadinessResult{Ready: true})
	if !got.Ready || len(got.Missing) != 0 || got.Reason != "" {
		t.Fatalf("an off-board session must be untouched, got %+v", got)
	}
}
