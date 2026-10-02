package agentbus

import (
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func TestWakeTargetsNameWhoAskedForStartableWork(t *testing.T) {
	ops := []board.Op{assertOp("design", "alice"), requireOp("design", "schema")}

	targets := WakeTargets(ops, nil)
	if len(targets) != 1 || targets[0].Participant != "alice" {
		t.Fatalf("targets = %+v, want alice, who asked for the step", targets)
	}
	if len(targets[0].Ready) != 1 || targets[0].Ready[0] != "schema" {
		t.Fatalf("ready = %v, want the startable step", targets[0].Ready)
	}
	if len(targets[0].Waiting) != 1 || targets[0].Waiting[0] != "design" {
		t.Fatalf("waiting = %v, want the node stalled on it", targets[0].Waiting)
	}

	if again := WakeTargets(ops, nil); again[0].Key != targets[0].Key {
		t.Fatal("the key must derive from the work, not from the call")
	}
	more := append(append([]board.Op(nil), ops...),
		board.Op{Verb: board.VerbRequire, Node: "design", Actor: "alice", Dep: &board.NodeSpec{ID: "errors"}})
	if WakeTargets(more, nil)[0].Key == targets[0].Key {
		t.Fatal("a different work set must produce a different key")
	}
}

func TestWakeTargetsStopOnceTheStepIsTaken(t *testing.T) {
	ops := []board.Op{assertOp("design", "alice"), requireOp("design", "schema")}
	claimed := append(append([]board.Op(nil), ops...), claimOp("schema", "bob"))
	if targets := WakeTargets(claimed, nil); len(targets) != 0 {
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
	targets := WakeTargets(nil, talk)
	if len(targets) != 1 || targets[0].Participant != "bob" || len(targets[0].Asks) != 1 {
		t.Fatalf("targets = %+v, want bob holding the open question", targets)
	}

	answer := answerLine("t", "bob", "c1", "agentbus-view/1", talkBase.Add(time.Second))
	answer.Seq = 2
	if err := ApplyTalk(talk, answer, TalkLimits{}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if targets := WakeTargets(nil, talk); len(targets) != 0 {
		t.Fatalf("an answered question wakes nobody: %+v", targets)
	}
}

func TestWakeTargetsDropWhatIsAlreadyDone(t *testing.T) {
	ops := []board.Op{
		assertOp("design", "alice"),
		requireOp("design", "schema"),
		doneOp("design", "alice", "bob"),
	}
	if targets := WakeTargets(ops, nil); len(targets) != 0 {
		t.Fatalf("work that is finished owes nobody a wake: %+v", targets)
	}
}
