package agentbus

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestTalkLogAssignsSeqAndFoldsBack(t *testing.T) {
	log, err := OpenTalkLog(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	for i, text := range []string{"one", "two", "three"} {
		line, err := log.Append(ctx, sayLine("t", "alice", text, talkBase.Add(time.Duration(i)*time.Second)), TalkLimits{})
		if err != nil {
			t.Fatalf("append %q: %v", text, err)
		}
		if line.Seq != uint64(i+1) {
			t.Fatalf("seq for %q = %d, want %d", text, line.Seq, i+1)
		}
	}

	state, read, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Items) != 3 || read.Skipped != 0 || read.Truncated != 0 {
		t.Fatalf("read counters = %+v, want three clean records", read)
	}
	topic := state.Topics["t"]
	if topic == nil || len(topic.Lines) != 3 {
		t.Fatalf("folded topic = %+v", topic)
	}
	if !topic.LastAt.Equal(talkBase.Add(2 * time.Second)) {
		t.Fatalf("lastAt = %v, want the newest line", topic.LastAt)
	}
}

func TestTalkLogRefusalKeepsTheSeqAndTheLog(t *testing.T) {
	log, err := OpenTalkLog(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if _, err := log.Append(ctx, sayLine("t", "alice", "one", talkBase), TalkLimits{}); err != nil {
		t.Fatalf("append: %v", err)
	}

	_, err = log.Append(ctx, sayLine("t", "alice", "two", talkBase.Add(time.Second)), TalkLimits{MaxRounds: 1})
	if reason, ok := IsTalkReject(err); !ok || reason != RefuseRounds {
		t.Fatalf("second line = (%v, %q), want rounds_exhausted", ok, reason)
	}

	line, err := log.Append(ctx, sayLine("t", "alice", "two", talkBase.Add(2*time.Second)), TalkLimits{})
	if err != nil {
		t.Fatalf("append after refusal: %v", err)
	}
	if line.Seq != 2 {
		t.Fatalf("seq after a refusal = %d, want 2: a refusal must not spend one", line.Seq)
	}
}

func TestTalkLogTwoWritersShareOneSeqLine(t *testing.T) {
	dir := t.TempDir()
	alice, err := OpenTalkLog(dir)
	if err != nil {
		t.Fatalf("open alice: %v", err)
	}
	bob, err := OpenTalkLog(dir)
	if err != nil {
		t.Fatalf("open bob: %v", err)
	}
	ctx := context.Background()

	first, err := alice.Append(ctx, sayLine("t", "alice", "hello", talkBase), TalkLimits{})
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	reply, err := bob.Append(ctx, sayLine("t", "bob", "hi", talkBase.Add(time.Second)), TalkLimits{})
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	if reply.Seq != first.Seq+1 {
		t.Fatalf("seqs = %d then %d, want consecutive: the second writer re-read under the lock", first.Seq, reply.Seq)
	}
	state, _, err := alice.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := len(state.Topics["t"].Lines); got != 2 {
		t.Fatalf("lines = %d, want both writers' lines", got)
	}
}

func TestTalkLogRepairsATornTailBeforeAppending(t *testing.T) {
	log, err := OpenTalkLog(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if _, err := log.Append(ctx, sayLine("t", "alice", "good", talkBase), TalkLimits{}); err != nil {
		t.Fatalf("append: %v", err)
	}
	f, err := os.OpenFile(log.Path(), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open torn: %v", err)
	}
	if _, err := f.WriteString(`{"seq":2,"topic":"t","ki`); err != nil {
		t.Fatalf("write torn: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close torn: %v", err)
	}

	if _, err := log.Append(ctx, sayLine("t", "alice", "after", talkBase.Add(time.Minute)), TalkLimits{}); err != nil {
		t.Fatalf("append after torn: %v", err)
	}
	state, read, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.Truncated != 0 || read.Skipped != 0 {
		t.Fatalf("counters after repair = %+v, want a clean log", read)
	}
	if got := len(state.Topics["t"].Lines); got != 2 {
		t.Fatalf("lines = %d, want 2: the new record must not be buried in the torn one", got)
	}
}

func TestTalkLogClosesALapsedTopicOnce(t *testing.T) {
	log, err := OpenTalkLog(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	limits := TalkLimits{SilenceWindow: time.Minute}
	if _, err := log.Append(ctx, sayLine("t", "alice", "hello", talkBase), limits); err != nil {
		t.Fatalf("append: %v", err)
	}

	closed, err := log.CloseLapsed(ctx, "t", talkBase.Add(30*time.Second), limits)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed {
		t.Fatal("the topic is inside its silence window")
	}
	closed, err = log.CloseLapsed(ctx, "t", talkBase.Add(2*time.Minute), limits)
	if err != nil || !closed {
		t.Fatalf("close after the window = (%v, %v), want a closure", closed, err)
	}
	again, err := log.CloseLapsed(ctx, "t", talkBase.Add(3*time.Minute), limits)
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if again {
		t.Fatal("a closed topic must not be closed twice")
	}

	state, _, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !state.Topics["t"].Closed || state.Topics["t"].CloseReason != CloseSilence {
		t.Fatalf("topic = %+v, want closed by silence", state.Topics["t"])
	}
	_, err = log.Append(ctx, sayLine("t", "alice", "again", talkBase.Add(4*time.Minute)), limits)
	if reason, ok := IsTalkReject(err); !ok || reason != RefuseTopicClosed {
		t.Fatalf("a say after the closure = (%v, %q), want topic_closed", ok, reason)
	}
}
