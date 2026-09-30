package control

import (
	"strings"

	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

// The unfinished-items offer is deliberately narrow. It rides the turn tail (the
// cache-stable system prefix is never touched), it is only shown when the turn
// itself looks like it is about one of the items, and it asks rather than
// instructs: handing work to a session that did not mean to continue it is the
// failure this whole channel has to avoid.
var continuationCues = []string{
	"接着", "继续", "续上", "接上", "上次", "上回", "之前那", "前几天", "未完成", "还没做完", "没干完",
}

// offerOpenHandoffs prepends one short note when the session's project has an
// unfinished item the turn appears to be about. It runs on a session's first
// turn and on turns that say they are continuing something; every other turn is
// left untouched, so an unrelated conversation never sees this.
func (c *Controller) offerOpenHandoffs(text, source string) string {
	if c.openHandoffs == nil {
		return text
	}
	if c.conversationTurns() > 0 && !hasContinuationCue(source) {
		return text
	}
	items := c.openHandoffs(recap.ProjectOf(c.SessionPath()))
	matched := recap.MatchOpenItems(items, source)
	if len(matched) == 0 {
		return text
	}
	return openHandoffsBlock(matched) + "\n\n" + text
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
