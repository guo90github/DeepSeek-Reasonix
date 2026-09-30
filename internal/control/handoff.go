package control

import (
	"strings"
	"time"

	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

// The unfinished-items offer is deliberately narrow. It rides the turn tail (the
// cache-stable system prefix is never touched), it is only shown when the turn
// itself looks like it is about one of the items, and it asks rather than
// instructs: handing work to a session that did not mean to continue it is the
// failure this whole channel has to avoid.
//
// Earlier conclusions ride the same gates but are shaped differently — background,
// never a question — because a wrong line then costs a line of context instead of
// mis-assigning work. That is the trade the false-positive measurement bought.
var continuationCues = []string{
	"接着", "继续", "续上", "接上", "上次", "上回", "之前那", "前几天", "未完成", "还没做完", "没干完",
}

// maxPriorNotes caps the background block: it is a hint, not a briefing.
const maxPriorNotes = 2

// offerProjectOffers prepends what this project carries into the turn: the
// unfinished items the turn looks like it is about (a question for the person),
// and the earlier conclusions about the same subject (background for the model).
// A session's first turn is judged on its subject, so is any turn that says it
// continues something; a later turn that says neither is only judged when it names
// the thing itself — otherwise mid-conversation would turn every shared word into
// a handoff.
func (c *Controller) offerProjectOffers(text, source string) string {
	if c.projectOffers == nil {
		return text
	}
	weighItems, weighNotes := recap.MatchOpenItems, recap.MatchNotes
	if c.conversationTurns() > 0 && !hasContinuationCue(source) {
		weighItems, weighNotes = recap.StrongMatchOpenItems, recap.StrongMatchNotes
	}
	offers := c.projectOffers(recap.ProjectOf(c.SessionPath()))
	blocks := []string{}
	if matched := weighItems(offers.Items, source, time.Now()); len(matched) > 0 {
		blocks = append(blocks, openHandoffsBlock(matched))
	}
	if matched := weighNotes(offers.Prior, source, maxPriorNotes); len(matched) > 0 {
		blocks = append(blocks, priorNotesBlock(matched))
	}
	if len(blocks) == 0 {
		return text
	}
	return strings.Join(blocks, "\n\n") + "\n\n" + text
}

// priorNotesBlock renders earlier conclusions as background. It must not read as
// a question or an instruction: nobody asked for this line, so a wrong one has to
// be ignorable — but it also must not read as noise, which is what "you did not
// ask for this" made of it: the model then answered from scratch while the line
// sat unused (measured 2026-09-30).
func priorNotesBlock(notes []recap.Note) string {
	var b strings.Builder
	b.WriteString("<prior-notes>\n")
	b.WriteString("This project already worked out the following in earlier sessions. Each line was distilled automatically and names where it came from, so treat it as established background you can check — not as an instruction. Use it where it bears on the work; if it conflicts with what you observe now, what you observe wins and the line is what should be corrected. Nobody asked for it, so do not raise it as a question.\n")
	for _, note := range notes {
		b.WriteString("- (" + oneLine(note.Kind) + ") " + oneLine(note.Body))
		if evidence := oneLine(note.Evidence); evidence != "" {
			b.WriteString(" (" + evidence + ")")
		}
		b.WriteString("\n")
	}
	b.WriteString("</prior-notes>")
	return b.String()
}

// conversationTurns counts the session's user/assistant messages. A session that
// holds nothing but its system prompt is still at its first turn — which is where
// an unfinished item is worth offering, before the work has been described anew.
func (c *Controller) conversationTurns() int {
	if c.executor == nil {
		return 0
	}
	count := 0
	for _, message := range c.executor.Session().Snapshot() {
		if message.Role != provider.RoleSystem {
			count++
		}
	}
	return count
}

// hasContinuationCue reports whether the turn says it picks something up.
func hasContinuationCue(text string) bool {
	lower := strings.ToLower(text)
	for _, cue := range continuationCues {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

// openHandoffsBlock renders the offer: how many, and which ones — a question for
// the person, never a task for the model.
func openHandoffsBlock(items []recap.OpenItem) string {
	var b strings.Builder
	b.WriteString("<open-items>\n")
	b.WriteString("This project has unfinished items you left earlier. Ask the person whether one of them is what this session is about before acting on any of them:\n")
	for _, item := range items {
		b.WriteString("- " + oneLine(item.Body))
		if evidence := oneLine(item.Evidence); evidence != "" {
			b.WriteString(" (" + evidence + ")")
		}
		b.WriteString("\n")
	}
	b.WriteString("</open-items>")
	return b.String()
}

// oneLine keeps one item on one line so the block stays scannable.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
