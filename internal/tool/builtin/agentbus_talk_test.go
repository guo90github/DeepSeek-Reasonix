package builtin

import (
	"context"
	"strings"
	"testing"
)

// An answer cites the question it answers: the tool has to hand the correlation, the topic,
// the addressee and the text to the board, because that is what makes the reply land with
// the asker instead of in the topic's general chatter.
func TestAgentBusToolAnswerCarriesTheCorrelationItAnswers(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}
	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t,
		`{"action":"answer","topic":"handover","correlation":"c-1","to":"alice","text":"taking the migration step"}`))
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(port.answered) != 1 || port.answered[0] != "c-1|handover|alice|taking the migration step" {
		t.Fatalf("answered = %v, want the correlation, topic, addressee and text", port.answered)
	}
	if !strings.Contains(out, "answer") || !strings.Contains(out, "handover") {
		t.Fatalf("result = %q, want the recorded answer named", out)
	}
}

// An answer with no correlation is not an answer: nothing would tell the asker the reply is
// theirs, so the call is refused before it reaches the board.
func TestAgentBusToolAnswerWithoutACorrelationIsRefused(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}
	if _, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t,
		`{"action":"answer","topic":"handover","text":"hm"}`)); err == nil {
		t.Fatal("an answer that cites no question must be refused")
	}
	if len(port.answered) != 0 {
		t.Fatalf("answered = %v, want nothing sent to the board", port.answered)
	}
}
