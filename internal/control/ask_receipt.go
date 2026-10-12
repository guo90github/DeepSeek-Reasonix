package control

import (
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// recordAskDecisionReceipt writes the durable record of an answered question: the
// session keeps it as provider-excluded metadata and the frontend shows it as a
// decision card. An answer the host made for an absent user says so in the
// receipt, so it never reads back as the user's own choice.
func (c *Controller) recordAskDecisionReceipt(id string, pending pendingAsk, answers []event.AskAnswer) {
	if c == nil || c.executor == nil {
		return
	}
	selected := make(map[string][]string, len(answers))
	for _, answer := range answers {
		selected[answer.QuestionID] = append([]string(nil), answer.Selected...)
	}
	parts := make([]string, 0, len(pending.questions))
	for _, question := range pending.questions {
		answer := strings.TrimSpace(strings.Join(selected[question.ID], ", "))
		if answer == "" {
			answer = "—"
		}
		prompt := strings.TrimSpace(question.Prompt)
		if prompt == "" {
			prompt = strings.TrimSpace(question.Header)
		}
		if prompt == "" {
			prompt = question.ID
		}
		parts = append(parts, prompt+": "+answer)
	}
	subject := strings.Join(parts, " · ")
	text := "Decision recorded: answered"
	if askAnswersAuto(answers) {
		subject = "auto-selected after no answer — " + subject
		text = "Decision recorded: the host auto-selected the recommended option after no answer"
	}
	receipt := &provider.DecisionReceipt{
		ID:      id,
		Kind:    "ask",
		Subject: clipUTF8(subject, 240),
		Outcome: "answered",
	}
	c.executor.Session().AddDecisionReceipt(receipt)
	c.sink.Emit(event.Event{
		Kind:            event.Notice,
		Code:            event.NoticeCodeDecisionReceipt,
		Level:           event.LevelInfo,
		Text:            text,
		DecisionReceipt: receipt,
	})
}

func askAnswersHaveSelection(answers []event.AskAnswer) bool {
	for _, answer := range answers {
		if len(answer.Selected) > 0 {
			return true
		}
	}
	return false
}
