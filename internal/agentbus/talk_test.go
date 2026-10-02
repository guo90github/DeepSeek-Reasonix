package agentbus

import (
	"testing"
	"time"
)

var talkBase = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func sayLine(topic, from, text string, at time.Time) TalkLine {
	return TalkLine{Topic: topic, Kind: TalkSay, From: from, Text: text, At: at}
}

func askLine(topic, from, correlation, text string, at time.Time, tokens int64) TalkLine {
	return TalkLine{Topic: topic, Kind: TalkAsk, From: from, Correlation: correlation, Text: text, At: at, Tokens: tokens}
}

func answerLine(topic, from, correlation, text string, at time.Time) TalkLine {
	return TalkLine{Topic: topic, Kind: TalkAnswer, From: from, Correlation: correlation, Text: text, At: at}
}

func withSeq(line TalkLine, seq uint64) TalkLine {
	line.Seq = seq
	return line
}

func withTokens(line TalkLine, tokens int64) TalkLine {
	line.Tokens = tokens
	return line
}

func TestTalkLimitsRefuseInOrder(t *testing.T) {
	cases := []struct {
		name   string
		limits TalkLimits
		seed   []TalkLine
		line   TalkLine
		want   string
	}{
		{"no topic", TalkLimits{}, nil, TalkLine{Kind: TalkSay, From: "a", Text: "x", At: talkBase}, RefuseNoTopic},
		{"no text", TalkLimits{}, nil, sayLine("t", "a", "   ", talkBase), RefuseNoText},
		{"unknown kind", TalkLimits{}, nil, TalkLine{Topic: "t", Kind: "shout", From: "a", Text: "x", At: talkBase}, RefuseUnknownKind},
		{
			"rounds", TalkLimits{MaxRounds: 1},
			[]TalkLine{sayLine("t", "a", "one", talkBase)},
			sayLine("t", "a", "two", talkBase.Add(time.Second)), RefuseRounds,
		},
		{
			"budget", TalkLimits{TokenBudget: 5},
			[]TalkLine{withTokens(sayLine("t", "a", "one", talkBase), 5)},
			withTokens(sayLine("t", "a", "two", talkBase.Add(time.Second)), 1), RefuseBudget,
		},
		{
			"rate", TalkLimits{RateWindow: time.Minute, RateMax: 1},
			[]TalkLine{sayLine("t", "a", "one", talkBase)},
			sayLine("t", "a", "two", talkBase.Add(time.Second)), RefuseRate,
		},
		{
			"rate does not fire outside the window", TalkLimits{RateWindow: time.Minute, RateMax: 1},
			[]TalkLine{sayLine("t", "a", "one", talkBase)},
			sayLine("t", "a", "two", talkBase.Add(2*time.Minute)), "",
		},
		{
			"closed", TalkLimits{},
			[]TalkLine{sayLine("t", "a", "one", talkBase), {Topic: "t", Kind: TalkClose, From: "system", At: talkBase.Add(time.Minute)}},
			sayLine("t", "a", "two", talkBase.Add(2*time.Minute)), RefuseTopicClosed,
		},
		{
			"ask without correlation", TalkLimits{}, nil,
			TalkLine{Topic: "t", Kind: TalkAsk, From: "a", Text: "x", At: talkBase}, RefuseNoCorrelation,
		},
		{
			"answer without a chain", TalkLimits{}, nil,
			answerLine("t", "b", "missing", "x", talkBase), RefuseNoCorrelation,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := FoldTalk(tc.seed)
			err := ApplyTalk(st, tc.line, tc.limits)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got %v", err)
				}
				return
			}
			reason, ok := IsTalkReject(err)
			if !ok || reason != tc.want {
				t.Fatalf("refusal = (%v, %q), want %q", ok, reason, tc.want)
			}
		})
	}
}

func TestRefusedLineChangesNothing(t *testing.T) {
	limits := TalkLimits{MaxRounds: 1}
	st := FoldTalk([]TalkLine{sayLine("t", "a", "one", talkBase)})
	if _, ok := IsTalkReject(ApplyTalk(st, sayLine("t", "a", "two", talkBase.Add(time.Second)), limits)); !ok {
		t.Fatal("the second line should be refused")
	}
	if got := len(st.Topics["t"].Lines); got != 1 {
		t.Fatalf("lines = %d, want 1: a refusal must not append", got)
	}
}

func TestAskAnswerChainsMeterHopTTLAndBudget(t *testing.T) {
	limits := TalkLimits{MaxHop: 2, AskTTL: time.Minute}

	st := NewTalkState()
	if err := ApplyTalk(st, askLine("t", "alice", "c1", "q1", talkBase, 1), limits); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if err := ApplyTalk(st, answerLine("t", "bob", "c1", "a1", talkBase.Add(10*time.Second)), limits); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if got := st.Chains["c1"].Hops; got != 2 {
		t.Fatalf("hops = %d, want 2", got)
	}
	reason, ok := IsTalkReject(ApplyTalk(st, askLine("t", "alice", "c1", "q2", talkBase.Add(20*time.Second), 1), limits))
	if !ok || reason != RefuseHop {
		t.Fatalf("third hop = (%v, %q), want hop_exhausted", ok, reason)
	}

	expired := NewTalkState()
	if err := ApplyTalk(expired, askLine("t", "alice", "c2", "q", talkBase, 1), limits); err != nil {
		t.Fatalf("ask: %v", err)
	}
	reason, ok = IsTalkReject(ApplyTalk(expired, answerLine("t", "bob", "c2", "late", talkBase.Add(2*time.Minute)), limits))
	if !ok || reason != RefuseExpired {
		t.Fatalf("late answer = (%v, %q), want expired", ok, reason)
	}

	cheap := NewTalkState()
	budget := TalkLimits{TokenBudget: 3}
	if err := ApplyTalk(cheap, askLine("t", "alice", "c3", "q", talkBase, 2), budget); err != nil {
		t.Fatalf("ask: %v", err)
	}
	reason, ok = IsTalkReject(ApplyTalk(cheap, withTokens(answerLine("t", "bob", "c3", "a", talkBase.Add(time.Second)), 2), budget))
	if !ok || reason != RefuseBudget {
		t.Fatalf("an answer over the topic budget = (%v, %q), want budget_exhausted", ok, reason)
	}
}

func TestDigestDeliversOnlyWhatNamesYou(t *testing.T) {
	lines := []TalkLine{
		withSeq(sayLine("t", "alice", "noise", talkBase), 1),
		withSeq(TalkLine{Topic: "t", Kind: TalkSay, From: "alice", Text: "to bob", Mentions: []string{"bob"}, At: talkBase.Add(time.Second)}, 2),
		withSeq(TalkLine{Topic: "t", Kind: TalkSay, From: "alice", To: "bob", Text: "direct", At: talkBase.Add(2 * time.Second)}, 3),
		withSeq(sayLine("t", "bob", "mine", talkBase.Add(3*time.Second)), 4),
	}
	st := FoldTalk(lines)

	named, hidden := Digest(st, "t", "bob", 0)
	if len(named) != 3 {
		t.Fatalf("named = %d, want 3 (mention, direct, own): %+v", len(named), named)
	}
	if hidden != 1 {
		t.Fatalf("hidden = %d, want 1 (the line that names nobody)", hidden)
	}
	for _, line := range named {
		if line.Text == "noise" {
			t.Fatalf("a line naming nobody must never be delivered")
		}
	}

	named, hidden = Digest(st, "t", "bob", 4)
	if len(named) != 0 || hidden != 0 {
		t.Fatalf("after the cursor, digest = (%d, %d), want nothing left", len(named), hidden)
	}
	if named, _ := Digest(st, "t", "carol", 0); len(named) != 0 {
		t.Fatalf("an unaddressed participant must receive nothing: %+v", named)
	}
}

func TestSilenceClosingIsRecordedNotDerived(t *testing.T) {
	limits := TalkLimits{SilenceWindow: time.Minute}
	st := FoldTalk([]TalkLine{sayLine("t", "alice", "hello", talkBase)})

	if TopicLapsed(st, "t", talkBase.Add(30*time.Second), limits) {
		t.Fatal("the topic is inside its silence window")
	}
	if !TopicLapsed(st, "t", talkBase.Add(2*time.Minute), limits) {
		t.Fatal("the topic has lapsed")
	}
	if TopicLapsed(st, "t", talkBase.Add(2*time.Minute), TalkLimits{}) {
		t.Fatal("no window configured means never lapsed")
	}

	closeLine, ok := CloseLine(st, "t", talkBase.Add(2*time.Minute))
	if !ok {
		t.Fatal("a lapsed topic must be closable")
	}
	if err := ApplyTalk(st, closeLine, limits); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !st.Topics["t"].Closed || st.Topics["t"].CloseReason != CloseSilence {
		t.Fatalf("topic not closed by silence: %+v", st.Topics["t"])
	}
	if reason, _ := IsTalkReject(ApplyTalk(st, closeLine, limits)); reason != RefuseTopicClosed {
		t.Fatalf("closing twice = %q, want topic_closed", reason)
	}
	reason, _ := IsTalkReject(ApplyTalk(st, sayLine("t", "alice", "again", talkBase.Add(3*time.Minute)), limits))
	if reason != RefuseTopicClosed {
		t.Fatalf("a say after the close = %q, want topic_closed (state alone decides)", reason)
	}
}

func TestFoldTalkIsDeterministic(t *testing.T) {
	lines := []TalkLine{
		withSeq(sayLine("t", "alice", "one", talkBase), 1),
		withSeq(askLine("t", "alice", "c1", "q", talkBase.Add(time.Second), 2), 2),
		withSeq(answerLine("t", "bob", "c1", "a", talkBase.Add(2*time.Second)), 3),
	}
	a := FoldTalk(lines)
	b := FoldTalk(lines)
	if len(a.Topics) != len(b.Topics) || len(a.Chains) != len(b.Chains) {
		t.Fatalf("fold is not deterministic: %+v vs %+v", a, b)
	}
	if got := len(a.Topics["t"].Lines); got != 3 {
		t.Fatalf("lines = %d, want 3", got)
	}
	if a.Topics["t"].Tokens != b.Topics["t"].Tokens || a.Chains["c1"].Hops != b.Chains["c1"].Hops {
		t.Fatalf("fold counters diverged")
	}
	if names := a.TopicNames(); len(names) != 1 || names[0] != "t" {
		t.Fatalf("topic names = %v", names)
	}
}
