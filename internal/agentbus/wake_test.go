package agentbus

import (
	"context"
	"strconv"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func TestWakeTargetsNameWhoAskedForStartableWork(t *testing.T) {
	ops := []board.Op{assertOp("design", "alice"), requireOp("design", "schema")}

	targets := WakeTargets(WakeInput{State: board.Fold(ops)})
	if len(targets) != 1 || targets[0].Participant != "alice" {
		t.Fatalf("targets = %+v, want alice, who asked for the step", targets)
	}
	if len(targets[0].Ready) != 1 || targets[0].Ready[0] != "schema" {
		t.Fatalf("ready = %v, want the startable step", targets[0].Ready)
	}
	if len(targets[0].Waiting) != 1 || targets[0].Waiting[0] != "design" {
		t.Fatalf("waiting = %v, want the node stalled on it", targets[0].Waiting)
	}

	if again := WakeTargets(WakeInput{State: board.Fold(ops)}); again[0].Key != targets[0].Key {
		t.Fatal("the key must derive from the work, not from the call")
	}
	more := append(append([]board.Op(nil), ops...),
		board.Op{Verb: board.VerbRequire, Node: "design", Actor: "alice", Dep: &board.NodeSpec{ID: "errors"}})
	if WakeTargets(WakeInput{State: board.Fold(more)})[0].Key == targets[0].Key {
		t.Fatal("a different work set must produce a different key")
	}
}

func TestWakeTargetsStopOnceTheStepIsTaken(t *testing.T) {
	ops := []board.Op{assertOp("design", "alice"), requireOp("design", "schema")}
	claimed := append(append([]board.Op(nil), ops...), claimOp("schema", "bob"))
	if targets := WakeTargets(WakeInput{State: board.Fold(claimed)}); len(targets) != 0 {
		t.Fatalf("a claimed step is nobody's wake: %+v", targets)
	}
}

func TestWakeTargetsCarryUnansweredQuestionsOnly(t *testing.T) {
	talk := NewTalkState()
	ask := askLine("t", "alice", "c1", "which schema?", talkBase, 0)
	ask.To = "bob"
	ask.Seq = 1
	if err := ApplyTalk(talk, ask, TalkLimits{}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	targets := WakeTargets(WakeInput{Talk: talk})
	if len(targets) != 1 || targets[0].Participant != "bob" || len(targets[0].Asks) != 1 {
		t.Fatalf("targets = %+v, want bob holding the open question", targets)
	}

	answer := answerLine("t", "bob", "c1", "agentbus-view/1", talkBase.Add(time.Second))
	answer.Seq = 2
	if err := ApplyTalk(talk, answer, TalkLimits{}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if targets := WakeTargets(WakeInput{Talk: talk}); len(targets) != 0 {
		t.Fatalf("an answered question wakes nobody: %+v", targets)
	}
}

func TestWakeTargetsDropWhatIsAlreadyDone(t *testing.T) {
	ops := []board.Op{
		assertOp("design", "alice"),
		requireOp("design", "schema"),
		doneOp("design", "alice", "bob"),
	}
	if targets := WakeTargets(WakeInput{State: board.Fold(ops)}); len(targets) != 0 {
		t.Fatalf("work that is finished owes nobody a wake: %+v", targets)
	}
}

func TestWakeTargetsNameWhoOwesAnAnswer(t *testing.T) {
	hearings := NewHearingState()
	if err := ApplyHearing(hearings, openHearing("n1", []string{"alice", "bob"}, talkBase), HearingLimits{}); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := ApplyHearing(hearings, answerHearing("n1", "alice", talkBase.Add(time.Second)), HearingLimits{}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	lim := HearingLimits{RoundTTL: time.Minute}
	in := WakeInput{Hearings: hearings, Limits: lim, Now: talkBase.Add(30 * time.Second)}
	if targets := WakeTargets(in); len(targets) != 0 {
		t.Fatalf("inside the round window nobody owes anything: %+v", targets)
	}
	in.Now = talkBase.Add(2 * time.Minute)
	targets := WakeTargets(in)
	if len(targets) != 1 || targets[0].Participant != "bob" {
		t.Fatalf("targets = %+v, want bob alone owing an answer", targets)
	}
	if len(targets[0].Owes) != 1 || targets[0].Owes[0] != "n1" {
		t.Fatalf("owes = %v, want the deliberation he has not answered", targets[0].Owes)
	}
}

// A step that has spent the host's retry budget stops being handed out, and nothing else on
// the board changes for it: being told, with the reason, is the only thing that moves it
// (G5). It is reported as stalled instead of startable — one fact per node, and the fact
// matches what the dispatcher will do with it.
func TestWakeTargetsNameStalledWorkInsteadOfOfferingItAgain(t *testing.T) {
	ctx := context.Background()
	brd, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	// alice asks for `schema` by making it a dependency of her step.
	for _, op := range []board.Op{assertOp("design", "alice"), requireOp("design", "schema")} {
		if _, err := brd.Apply(ctx, op); err != nil {
			t.Fatalf("apply %s: %v", op.Verb, err)
		}
	}
	// Two hand-outs that both lapsed: the sweeper records no progress for each, which is
	// what stops the third. Each attempt is named, which is what makes it a new op rather
	// than a replay of the first (the dispatcher names its attempts the same way).
	for attempt := 0; attempt < 2; attempt++ {
		// Each hand-out is named by its own deadline: the sweeper keys a reclaim by
		// node + deadline, so two attempts sharing one would collapse into one record.
		if _, err := brd.Apply(ctx, board.Op{
			Verb: board.VerbClaim, Node: "schema", Actor: "bob",
			ID:       "handout-" + strconv.Itoa(attempt),
			Bounds:   &board.Bounds{Steps: 1},
			Deadline: time.Now().UTC().Add(time.Duration(attempt+1) * time.Minute),
		}); err != nil {
			t.Fatalf("claim %d: %v", attempt, err)
		}
		if _, err := brd.Sweep(ctx, time.Now().UTC().Add(10*time.Minute)); err != nil {
			t.Fatalf("sweep %d: %v", attempt, err)
		}
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := state.Nodes["schema"].NoProgress; got != 2 {
		t.Fatalf("NoProgress = %d, want both lapsed hand-outs on the record", got)
	}

	targets := WakeTargets(WakeInput{State: state, Now: time.Now().UTC(), StallAfter: 2})
	if len(targets) != 1 || targets[0].Participant != "alice" {
		t.Fatalf("targets = %+v, want alice, who asked for the stalled step", targets)
	}
	if len(targets[0].Stalled) != 1 || targets[0].Stalled[0] != "schema" {
		t.Fatalf("stalled = %v, want the step nobody will hand out again", targets[0].Stalled)
	}
	if len(targets[0].Ready) != 0 {
		t.Fatalf("ready = %v, want it reported once, as stalled", targets[0].Ready)
	}
	if len(targets[0].Waiting) != 1 || targets[0].Waiting[0] != "design" {
		t.Fatalf("waiting = %v, want her step still blocked on it", targets[0].Waiting)
	}

	// A host that set no retry budget never says this: the kernel invents no limit.
	quiet := WakeTargets(WakeInput{State: state, Now: time.Now().UTC()})
	if len(quiet) != 1 || len(quiet[0].Stalled) != 0 {
		t.Fatalf("targets = %+v, want nothing reported as stalled without a budget", quiet)
	}
	// Below the budget the step is ordinary startable work, and the key says which fact it
	// is carrying: a step that was already offered has to be able to wake somebody again
	// once it stops moving.
	below := WakeTargets(WakeInput{State: state, Now: time.Now().UTC(), StallAfter: 3})
	if len(below) != 1 || len(below[0].Ready) != 1 || below[0].Ready[0] != "schema" {
		t.Fatalf("targets = %+v, want it offered as startable while its budget lasts", below)
	}
	if below[0].Key == targets[0].Key {
		t.Fatal("the stalled state must carry its own key, or the wake never fires again")
	}
}

// A finished step is not a live symptom, whatever its history: the no-progress counter it
// collected while it was stuck stays on the record, and the wake must stay quiet about it
// (T8-2's rule, applied to the stall report).
func TestWakeTargetsStayQuietAboutAStalledStepThatFinished(t *testing.T) {
	ctx := context.Background()
	brd, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	for _, op := range []board.Op{assertOp("design", "alice"), requireOp("design", "schema")} {
		if _, err := brd.Apply(ctx, op); err != nil {
			t.Fatalf("apply %s: %v", op.Verb, err)
		}
	}
	claim := func(id string, deadline time.Time) board.Op {
		return board.Op{
			Verb: board.VerbClaim, Node: "schema", Actor: "bob", ID: id,
			Bounds: &board.Bounds{Steps: 1}, Deadline: deadline,
		}
	}
	if _, err := brd.Apply(ctx, claim("handout-0", time.Now().UTC().Add(5*time.Minute))); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := brd.Sweep(ctx, time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	// Bob comes back for it and finishes it this time: the done verdict rests on the step's
	// own assertion, not on the op that carries it.
	if _, err := brd.Apply(ctx, claim("handout-1", time.Now().UTC().Add(time.Hour))); err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbAssert, Node: "schema", Actor: "bob",
		Evidence: []board.Evidence{{Kind: "test", Ref: "go test ./internal/agentbus/..."}},
	}); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbDecide, Node: "schema", Actor: "bob", Outcome: board.OutcomeDone,
		ReproducedBy: "alice",
	}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if n := state.Nodes["schema"]; n.NoProgress == 0 || n.State != board.StateDone {
		t.Fatalf("schema = %+v, want a finished step with its no-progress history", n)
	}
	if targets := WakeTargets(WakeInput{State: state, Now: time.Now().UTC(), StallAfter: 1}); len(targets) != 0 {
		t.Fatalf("targets = %+v, want nobody: a settled step is not a live symptom", targets)
	}
}
