package control

import (
	"fmt"
	"time"

	"reasonix/internal/event"
)

// defaultAskAutoAnswerTimeout is how long a displayed question waits for the user
// before the host resolves it with the recommended option. A turn must not sit
// waiting for a frontend nobody is watching until the approval timeout cancels
// it; Options.AskTimeout overrides this, and a negative value disables it.
const defaultAskAutoAnswerTimeout = 10 * time.Minute

// scheduleAskAutoAnswer arms the no-answer fallback for one displayed ask and
// returns the stop func. The synthetic answers take the ordinary
// AnswerQuestionChecked route, so the ledger, the receipt and the frontend's card
// follow the same path a real answer takes; whichever resolver arrives first wins,
// because resolveAskAfter admits exactly one resolution per id.
func (c *Controller) scheduleAskAutoAnswer(id string, questions []event.AskQuestion) func() {
	if c == nil || len(questions) == 0 {
		return func() {}
	}
	// Read the wait on the caller's goroutine, so the watchdog that outlives the
	// prompt owns its own copy of the deadline.
	wait := c.approval.timeouts.askAutoAnswerWait()
	if wait <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-done:
			return
		case <-timer.C:
		}
		answers := recommendedAskAnswers(questions)
		if len(answers) == 0 {
			return
		}
		resolved, err := c.answerQuestionChecked(id, answers)
		if err != nil || !resolved {
			return
		}
		c.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Code:  event.NoticeCodeAskAutoAnswered,
			Text:  "Nobody answered within " + askWaitText(wait) + ", so the host went with the recommended option.",
			Detail: "the question stayed open with nobody watching; if that choice was" +
				" wrong, say so and the work can be redirected",
		})
	}()
	return func() { close(done) }
}

// recommendedAskAnswers takes the option the `ask` schema documents as
// recommended — the first one — for every question.
func recommendedAskAnswers(questions []event.AskQuestion) []event.AskAnswer {
	answers := make([]event.AskAnswer, 0, len(questions))
	for _, question := range questions {
		if len(question.Options) == 0 {
			continue
		}
		answers = append(answers, event.AskAnswer{
			QuestionID: question.ID,
			Selected:   []string{question.Options[0].Label},
			Auto:       true,
		})
	}
	return answers
}

// askAnswersAuto reports whether these answers are the host's fallback rather
// than the user's own selection.
func askAnswersAuto(answers []event.AskAnswer) bool {
	selected := 0
	for _, answer := range answers {
		if len(answer.Selected) == 0 {
			continue
		}
		if !answer.Auto {
			return false
		}
		selected++
	}
	return selected > 0
}

func askWaitText(wait time.Duration) string {
	if wait%time.Minute == 0 {
		return fmt.Sprintf("%d minutes", int(wait/time.Minute))
	}
	return wait.String()
}
