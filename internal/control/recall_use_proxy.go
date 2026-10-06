package control

import (
	"strings"
	"unicode"

	"reasonix/internal/agent"
	"reasonix/internal/memory"
)

// The recall use proxy is recorded, never scored. It answers "did the reply repeat a
// word only this fact carried"; it cannot tell "unused" from "used without echoing the
// fact's own words", so nothing reads it as truth.

// markRecallUse fills LikelyUsed on the turn's injected hits. Called from inside the
// turn-outcome write so the sidecar is touched once.
func (c *Controller) markRecallUse(meta *agent.BranchMeta, turn int, reply string) {
	if meta == nil || turn < 1 {
		return
	}
	reply = strings.ToLower(strings.TrimSpace(reply))
	if reply == "" {
		return
	}
	facts := map[string]memory.Memory{}
	if set := c.memory.current(); set != nil {
		for _, fact := range set.Store.ListAll() {
			facts[fact.ID] = fact
		}
	}
	for i, recorded := range meta.MemoryRecall {
		if recorded.TurnSeq != turn {
			continue
		}
		meta.MemoryRecall[i] = judgeRecallUse(recorded, facts, reply)
	}
}

// judgeRecallUse marks each hit the model actually received: used when the reply
// repeats a word of the fact that the ask did not already contain.
func judgeRecallUse(turn agent.MemoryRecallTurn, facts map[string]memory.Memory, reply string) agent.MemoryRecallTurn {
	ask := strings.ToLower(turn.QueryExcerpt)
	for i, hit := range turn.Hits {
		if hit.Injected == nil || !*hit.Injected {
			continue
		}
		fact, ok := facts[hit.ID]
		if !ok {
			continue
		}
		verdict := false
		for _, term := range distinctiveFactTerms(fact) {
			if strings.Contains(ask, term) {
				continue
			}
			if strings.Contains(reply, term) {
				verdict = true
				break
			}
		}
		turn.Hits[i].LikelyUsed = &verdict
	}
	return turn
}

// distinctiveFactTerms are the fact's own words — its slug, title and keywords — minus
// anything too short to be evidence and the English connectives common in titles.
func distinctiveFactTerms(fact memory.Memory) []string {
	var terms []string
	for _, field := range []string{fact.Name, fact.Title, fact.Keywords} {
		for _, raw := range strings.FieldsFunc(field, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			term := strings.ToLower(raw)
			if len([]rune(term)) < 2 || recallUseStopwords[term] {
				continue
			}
			terms = append(terms, term)
		}
	}
	return terms
}

// recallUseStopwords keeps the proxy from firing on a connective. Deliberately tiny:
// the ask is subtracted term by term, not the language.
var recallUseStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "not": true,
	"that": true, "this": true, "from": true, "into": true, "when": true,
	"per": true, "via": true, "all": true, "any": true, "use": true, "uses": true,
}
