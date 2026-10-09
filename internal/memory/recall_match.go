package memory

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"reasonix/internal/retrieval"
)

// Recall evidence: the phrases a turn and a fact actually share, and the gates
// that decide which of them are worth prompt space. Scoring lives in
// auto_recall.go; this gate sequence is what keeps an unrelated fact out.

// recallRun is one phrase a fact and the turn actually share. Bigram overlap
// alone is not evidence — two bigrams of one three-character word used to count
// as two matches — so a run survives only if the substring is present verbatim.
type recallRun struct {
	text  string
	runes int
	cjk   bool
	// label marks a run the fact's own label fields carry: evidence about what
	// the fact is, not about what its prose happens to mention.
	label bool
}

// matchedRecallRuns groups the query's matched terms into shared phrases. Terms
// arrive in first-seen order, which is document order inside one run, so
// consecutive overlapping bigrams reconstruct the substring both texts share.
func matchedRecallRuns(queryTerms []string, doc autoRecallDoc) []recallRun {
	var runs []recallRun
	var pending []string
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if run, ok := recallRunFor(pending, doc); ok {
			runs = append(runs, run)
		}
		pending = nil
	}
	for _, term := range queryTerms {
		if doc.counts[term] == 0 {
			continue
		}
		if len(pending) > 0 && chainsWith(pending[len(pending)-1], term) {
			pending = append(pending, term)
			continue
		}
		flush()
		pending = []string{term}
	}
	flush()
	return runs
}

func recallRunFor(terms []string, doc autoRecallDoc) (recallRun, bool) {
	var text strings.Builder
	text.WriteString(terms[0])
	for _, term := range terms[1:] {
		runes := []rune(term)
		text.WriteString(string(runes[len(runes)-1]))
	}
	run := recallRun{text: strings.ToLower(text.String()), cjk: retrieval.CJKBigram(terms[0])}
	if !strings.Contains(doc.lower, run.text) {
		return recallRun{}, false
	}
	run.runes = utf8.RuneCountInString(run.text)
	run.label = sharesPhraseWith(doc.identity, run.text)
	return run, true
}

// sharesPhraseWith reports whether a fact's label text carries the run's own
// phrase of recallMinRunRunes. Requiring the whole run is too strict: a turn
// that opens with a particle ("把支付部署到绿色集群") shares the fact's wording
// without sharing the particle.
func sharesPhraseWith(identity, run string) bool {
	runes := []rune(run)
	if len(runes) <= recallMinRunRunes {
		return strings.Contains(identity, run)
	}
	for i := 0; i+recallMinRunRunes <= len(runes); i++ {
		if strings.Contains(identity, string(runes[i:i+recallMinRunRunes])) {
			return true
		}
	}
	return false
}

func chainsWith(prev, next string) bool {
	if !retrieval.CJKBigram(prev) || !retrieval.CJKBigram(next) {
		return false
	}
	left, right := []rune(prev), []rune(next)
	return left[len(left)-1] == right[0]
}

// strongRecallMatch keeps automatic recall out of one-common-word territory: two
// separate shared phrases, or one phrase long enough to be a real string (three
// CJK runes), or the single symbol an identifier makes distinctive.
func strongRecallMatch(query string, queryTerms []string, runs []recallRun) bool {
	if len(runs) >= 2 {
		return true
	}
	if len(runs) != 1 {
		return false
	}
	run := runs[0]
	if run.cjk {
		return run.runes >= recallMinRunRunes
	}
	if len(queryTerms) <= 2 && run.runes >= 6 {
		return true
	}
	return distinctiveQueryTerm(query, run.text)
}

// identityFieldEvidence requires the shared phrase to be part of what the fact
// says it is. Body-only overlap is how a UI checklist reached a Go task: every
// long body shares some common verb with every long turn.
func identityFieldEvidence(runs []recallRun) bool {
	for _, run := range runs {
		if run.label {
			return true
		}
	}
	return false
}

func recallCoverageRunes(queryRunes int) int {
	return max(queryRunes/recallCoverageDivisor, recallCoverageFloorRunes)
}

// sharesEnoughOfTheTurn bounds how little of a long turn may carry a match: a
// pasted specification shares a handful of words with every fact in the store.
func sharesEnoughOfTheTurn(runs []recallRun, needRunes int) bool {
	shared := 0
	for _, run := range runs {
		shared += run.runes
	}
	return shared >= needRunes
}

const (
	recallGateEvidence = "evidence"
	recallGateLabel    = "label"
	recallGateCoverage = "coverage"
	recallGateRarity   = "rarity"
)

var recallGateReasons = map[string]string{
	recallGateLabel:    "sharing words only with a fact's body",
	recallGateCoverage: "sharing too little of the turn",
	recallGateEvidence: "no shared phrase",
	recallGateRarity:   "words the whole library shares",
}

// suppressedRecallReason names the gate that turned a turn away, so a reader can
// tell a threshold to tune from a fact whose label fields want a better word.
func suppressedRecallReason(rejected map[string]int, docs int) string {
	gate, count := "", 0
	for _, name := range []string{recallGateLabel, recallGateCoverage, recallGateEvidence, recallGateRarity} {
		if rejected[name] > count {
			gate, count = name, rejected[name]
		}
	}
	if gate == "" {
		return "no sufficiently distinctive match"
	}
	return fmt.Sprintf("no sufficiently distinctive match: %d of %d candidates rejected for %s", count, docs, recallGateReasons[gate])
}

// autoRecallIdentityText is what a fact claims about itself: the fields a writer
// curates so the fact can be found, as opposed to the prose that explains it.
func autoRecallIdentityText(memory Memory) string {
	return strings.Join([]string{memory.Name, memory.Title, memory.Description, memory.Keywords, memory.SubjectKey}, "\n")
}

// recallReason names the phrases a hit was allowed on, where they were found and
// the numbers the coverage gate used. A reason that lists every matched bigram
// reads like evidence and is not.
func recallReason(runs []recallRun, needRunes int, scope FactScope) string {
	words := make([]string, 0, len(runs))
	shared, label := 0, false
	for _, run := range runs {
		shared += run.runes
		label = label || run.label
		if len(words) < 3 {
			words = append(words, run.text)
		}
	}
	where := "body"
	if label {
		where = "label fields"
	}
	return fmt.Sprintf("matched %s in %s (%d of %d runes needed); %s scope",
		strings.Join(words, ", "), where, shared, needRunes, NormalizeFactScope(string(scope)))
}
