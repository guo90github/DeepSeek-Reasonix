package control

import (
	"context"
	"sync"
	"testing"
	"time"

	"reasonix/internal/event"
)

type autoAnswerSink struct {
	mu      sync.Mutex
	asks    int
	notices []event.Event
}

func (s *autoAnswerSink) Emit(e event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch e.Kind {
	case event.AskRequest:
		s.asks++
	case event.Notice:
		s.notices = append(s.notices, e)
	}
}

func (s *autoAnswerSink) autoAnswered() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, notice := range s.notices {
		if notice.Code == event.NoticeCodeAskAutoAnswered {
			return true
		}
	}
	return false
}

func autoAnswerController(t *testing.T, sink event.Sink, askTimeout time.Duration) *Controller {
	t.Helper()
	return New(Options{Sink: sink, SessionDir: t.TempDir(), AskTimeout: askTimeout})
}

func waitForDisplayedAsk(t *testing.T, c *Controller, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, asks := c.approval.snapshotPrompts(); len(asks) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the question never reached the frontend (want %d displayed)", want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAskTimeoutResolution(t *testing.T) {
	cases := []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{"unset keeps the built-in wait", 0, defaultAskAutoAnswerTimeout},
		{"explicit wait", 90 * time.Second, 90 * time.Second},
		{"negative disables the fallback", -time.Minute, 0},
	}
	for _, tc := range cases {
		if got := (promptTimeouts{ask: tc.configured}).askAutoAnswerWait(); got != tc.want {
			t.Errorf("%s: %v resolved to %v, want %v", tc.name, tc.configured, got, tc.want)
		}
	}
}

// A question nobody answers must not hold its turn until the approval timeout:
// after Options.AskTimeout the host resolves it with the recommended option and
// tells the user it did.
func TestAskAutoAnswersWithTheRecommendedOption(t *testing.T) {
	sink := &autoAnswerSink{}
	c := autoAnswerController(t, sink, 60*time.Millisecond)

	type result struct {
		answers []event.AskAnswer
		err     error
	}
	done := make(chan result, 1)
	go func() {
		answers, err := c.Ask(context.Background(), askProbeQuestions())
		done <- result{answers: answers, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Ask: %v", got.err)
		}
		if len(got.answers) != 1 || got.answers[0].QuestionID != "q1" || !got.answers[0].Auto {
			t.Fatalf("answers = %+v, want the recommended option marked as automatic", got.answers)
		}
		if len(got.answers[0].Selected) != 1 || got.answers[0].Selected[0] != "A" {
			t.Fatalf("selected = %v, want the first option, which the schema calls recommended", got.answers[0].Selected)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("an unanswered question never resolved its turn")
	}

	// The notice follows the resolution, so it lands just after the turn resumes.
	deadline := time.Now().Add(time.Second)
	for !sink.autoAnswered() {
		if time.Now().After(deadline) {
			t.Fatal("the host answered for the user without saying so")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, pending := c.approval.snapshotPrompts(); len(pending) != 0 {
		t.Fatalf("the resolved ask is still pending: %+v", pending)
	}
}

// A real answer arriving first wins: the fallback must neither overwrite the
// user's choice nor report that it made one.
func TestAskAnswerBeforeTheFallbackWins(t *testing.T) {
	sink := &autoAnswerSink{}
	c := autoAnswerController(t, sink, 300*time.Millisecond)

	done := make(chan []event.AskAnswer, 1)
	go func() {
		answers, err := c.Ask(context.Background(), askProbeQuestions())
		if err != nil {
			done <- nil
			return
		}
		done <- answers
	}()

	waitForDisplayedAsk(t, c, 1)
	_, asks := c.approval.snapshotPrompts()
	c.AnswerQuestion(asks[0].ID, []event.AskAnswer{{QuestionID: "q1", Selected: []string{"B"}}})

	select {
	case answers := <-done:
		if len(answers) != 1 || len(answers[0].Selected) != 1 || answers[0].Selected[0] != "B" || answers[0].Auto {
			t.Fatalf("answers = %+v, want the user's own choice", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the user's answer never released the turn")
	}

	time.Sleep(400 * time.Millisecond)
	if sink.autoAnswered() {
		t.Fatal("the fallback announced an auto-answer after the user had answered")
	}
	if _, pending := c.approval.snapshotPrompts(); len(pending) != 0 {
		t.Fatalf("the answered ask is still pending: %+v", pending)
	}
}

// A negative AskTimeout is the "wait for me" posture: the question keeps blocking.
func TestAskFallbackDisabledKeepsWaiting(t *testing.T) {
	sink := &autoAnswerSink{}
	c := autoAnswerController(t, sink, -time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := c.Ask(ctx, askProbeQuestions())
		errc <- err
	}()

	waitForDisplayedAsk(t, c, 1)
	select {
	case err := <-errc:
		t.Fatalf("Ask returned %v with the fallback disabled; it must keep waiting", err)
	case <-time.After(150 * time.Millisecond):
	}
	if sink.autoAnswered() {
		t.Fatal("the disabled fallback answered anyway")
	}

	cancel()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("cancelled Ask returned a nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask did not unblock after cancellation")
	}
}
