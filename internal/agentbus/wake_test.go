package agentbus

import (
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
