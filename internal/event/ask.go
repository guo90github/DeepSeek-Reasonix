package event

import "time"

// Ask carries an AskRequest: a batch of questions and the ID that correlates the
// controller's AnswerQuestion(ID, …) reply.
type Ask struct {
	ID        string
	Questions []AskQuestion
	TurnID    string
	// AutoAnswerIn is how long after this card is displayed the host answers it
	// with the recommended option; zero means the host never auto-answers.
	AutoAnswerIn time.Duration
}
