package agentbus

import (
	"testing"
	"time"
)

// An ask that has been answered is not owed any more — whatever the answer names. The chain
// counts the answer as its second hop, so the wake must stop naming it: a real wake kept
// naming an ask that had already been answered, and the session answered it again (F27).
func TestAnAnsweredAskIsNoLongerOwed(t *testing.T) {
	state := NewTalkState()
	ask := askLine("t", "alice", "ask-1", "which one?", talkBase, 0)
	ask.To = "bob"
	if err := ApplyTalk(state, ask, TalkLimits{}); err != nil {
		t.Fatal(err)
	}
	// The answer cites the correlation and nobody else: that is how a real answer looks when
	// its sender does not repeat the addressee.
	if err := ApplyTalk(state, answerLine("t", "bob", "ask-1", "the first one", talkBase.Add(time.Minute)), TalkLimits{}); err != nil {
		t.Fatal(err)
	}

	assertAskOwed(t, state, false)
}

// An ask nobody answered is still owed.
func TestAnUnansweredAskIsOwed(t *testing.T) {
	state := NewTalkState()
	ask := askLine("t", "alice", "ask-2", "which one?", talkBase, 0)
	ask.To = "bob"
	if err := ApplyTalk(state, ask, TalkLimits{}); err != nil {
		t.Fatal(err)
	}

	assertAskOwed(t, state, true)
}

func assertAskOwed(t *testing.T, state *TalkState, want bool) {
	t.Helper()
	targets := WakeTargets(WakeInput{Talk: state, Now: talkBase.Add(time.Hour)})
	owed := false
	for _, target := range targets {
		if target.Participant == "bob" && len(target.Asks) > 0 {
			owed = true
		}
	}
	if owed != want {
		t.Fatalf("bob's asks = %+v, want owed=%t", targets, want)
	}
}
