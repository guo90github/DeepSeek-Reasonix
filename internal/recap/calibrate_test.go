package recap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Calibrating the offer rule needs real text on both sides: the notes a person
// actually kept, and the openers sessions actually begin with. This test replays
// one against the other on a live machine and reports how often the rule fires
// where it should and where it should not; it asserts nothing about the numbers
// because the numbers are the result. It is skipped without RECAP_CALIB_ROOT.
//
//	REASONIX_CACHE_HOME=<dir holding session-recap/v1.sqlite> \
//	  RECAP_CALIB_ROOT=%APPDATA%\reasonix \
//	  go test ./internal/recap/ -run CalibrateOfferRule -v -count=1
func TestCalibrateOfferRule(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("RECAP_CALIB_ROOT"))
	if root == "" {
		t.Skip("set RECAP_CALIB_ROOT to a reasonix state root to calibrate")
	}
	ctx := context.Background()
	store, err := Open(ctx, Options{Path: DefaultPath()})
	if err != nil {
		t.Fatalf("open the projection: %v", err)
	}
	defer func() { _ = store.Close() }()
	records, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) == 0 {
		t.Skip("the projection holds no records to calibrate against")
	}

	sessions := realSessions(t, root)
	openers := map[string]string{}
	projectOf := map[string]string{}
	for _, path := range sessions {
		project := ProjectOf(path)
		if project == "" {
			continue
		}
		projectOf[path] = project
		text, err := FileTranscript{}.Read(ctx, path)
		if err != nil {
			continue
		}
		if opener := firstUserTurn(text); opener != "" {
			openers[path] = opener
		}
	}

	now := time.Now()
	var items, ownHits, otherPairs, shippedOther int
	rules := offerRules()
	ownHitsByRule := make([]int, len(rules))
	otherHitsByRule := make([]int, len(rules))
	for _, record := range records {
		own := realPathFor(sessions, record.Path)
		if own == "" {
			continue
		}
		project := projectOf[own]
		ownOpener := openers[own]
		t.Logf("opener of %s: %s", filepath.Base(own), clipRunes(ownOpener, 90))
		others := []string{}
		for path, text := range openers {
			if path != own && projectOf[path] == project {
				others = append(others, text)
			}
		}
		for _, entry := range record.Entries {
			if strings.TrimSpace(entry.Body) == "" {
				continue
			}
			items++
			item := OpenItem{ID: HashEntry(entry.Kind, entry.Body), Project: project,
				Body: entry.Body, Evidence: entry.Evidence, OpenedAt: now}
			single := []OpenItem{item}
			if ownOpener != "" && len(MatchOpenItems(single, ownOpener, now)) == 1 {
				ownHits++
			}
			ids, pairs := sharedIdentifiers(tokensOf(item), ownOpener), sharedPairs(tokensOf(item), ownOpener)
			for i, rule := range rules {
				if ownOpener != "" && rule.hit(ids, pairs) {
					ownHitsByRule[i]++
				}
			}
			for _, turn := range others {
				if len(MatchOpenItems(single, turn, now)) == 1 {
					shippedOther++
				}
				turnIDs := sharedIdentifiers(tokensOf(item), turn)
				turnPairs := sharedPairs(tokensOf(item), turn)
				for i, rule := range rules {
					if rule.hit(turnIDs, turnPairs) {
						otherHitsByRule[i]++
					}
				}
			}
			otherPairs += len(others)
			t.Logf("%-10s own=%-5t ids=%d pairs=%d  %s",
				entry.Kind, ownOpener != "" && len(MatchOpenItems(single, ownOpener, now)) == 1,
				ids, pairs, clipRunes(entry.Body, 40))
		}
	}
	t.Logf("shipped MatchOpenItems: recall=%d/%d  false-positives=%d/%d (%.1f%%)",
		ownHits, items, shippedOther, otherPairs,
		100*float64(shippedOther)/float64(max(otherPairs, 1)))
	for i, rule := range rules {
		t.Logf("%-22s recall=%d/%d  false-positives=%d/%d (%.1f%%)", rule.name,
			ownHitsByRule[i], items, otherHitsByRule[i], otherPairs,
			100*float64(otherHitsByRule[i])/float64(max(otherPairs, 1)))
	}
}

// countRuleHits re-runs one rule over the same pairs, for the line above.
func countRuleHits(records []Record, openers, projectOf map[string]string, sessions []string, rules []rule, index int) int {
	now := time.Now()
	hits := 0
	for _, record := range records {
		own := realPathFor(sessions, record.Path)
		if own == "" {
			continue
		}
		for _, entry := range record.Entries {
			item := OpenItem{Body: entry.Body, Evidence: entry.Evidence, OpenedAt: now}
			for path, turn := range openers {
				if path == own || projectOf[path] != projectOf[own] {
					continue
				}
				if rules[index].hit(sharedIdentifiers(tokensOf(item), turn), sharedPairs(tokensOf(item), turn)) {
					hits++
				}
			}
		}
	}
	return hits
}

// rule is one candidate offer rule, measured against real text.
type rule struct {
	name string
	hit  func(identifiers, pairs int) bool
}

// offerRules are the candidates, loosest first, for the next calibration.
func offerRules() []rule {
	return []rule{
		{"1id|2pair (was shipped)", func(i, p int) bool { return i >= 1 || p >= 2 }},
		{"2id|1id&2pair|3pair (shipped)", func(i, p int) bool { return i >= 2 || (i >= 1 && p >= 2) || p >= 3 }},
		{"1id&2pair", func(i, p int) bool { return i >= 1 && p >= 2 }},
		{"2id|3pair", func(i, p int) bool { return i >= 2 || p >= 3 }},
		{"2id&2pair", func(i, p int) bool { return i >= 2 && p >= 2 }},
	}
}

// sharedIdentifiers counts how many of an item's identifiers the turn names.
func sharedIdentifiers(item tokens, turn string) int {
	lower := strings.ToLower(turn)
	hits := 0
	for _, identifier := range item.identifiers {
		if strings.Contains(lower, identifier) {
			hits++
		}
	}
	return hits
}

// sharedPairs counts how many of an item's character pairs the turn carries.
func sharedPairs(item tokens, turn string) int {
	lower := strings.ToLower(turn)
	seen := map[string]bool{}
	hits := 0
	for _, pair := range item.pairs {
		if !seen[pair] && strings.Contains(lower, pair) {
			seen[pair] = true
			hits++
		}
	}
	return hits
}

// realSessions lists the state root's session files, newest name first.
func realSessions(t *testing.T, root string) []string {
	t.Helper()
	out := []string{}
	err := filepath.WalkDir(filepath.Join(root, "projects"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != "sessions" {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// realPathFor finds the state root's own copy of a projected session, which is
// how a record made from a copied file still measures against live sessions.
func realPathFor(sessions []string, projected string) string {
	base := filepath.Base(projected)
	for _, path := range sessions {
		if filepath.Base(path) == base {
			return path
		}
	}
	return ""
}

// firstUserTurn reads what the person actually typed to open the session: the
// host's injected context rides the same message, so it is stripped first.
func firstUserTurn(text string) string {
	for _, block := range userTurns(text) {
		if opener := stripInjectedContext(block); opener != "" {
			return opener
		}
	}
	return ""
}

func userTurns(text string) []string {
	chunks := strings.Split(text, "## User (turn ")
	if len(chunks) < 2 {
		return nil
	}
	out := make([]string, 0, len(chunks)-1)
	for _, chunk := range chunks[1:] {
		if idx := strings.Index(chunk, "\n"); idx >= 0 {
			chunk = chunk[idx+1:]
		}
		if end := strings.Index(chunk, "\n\n## "); end >= 0 {
			chunk = chunk[:end]
		}
		out = append(out, chunk)
	}
	return out
}

// stripInjectedContext removes the host's own leading blocks from a turn.
func stripInjectedContext(block string) string {
	trimmed := strings.TrimSpace(block)
	for strings.HasPrefix(trimmed, "<") {
		end := strings.Index(trimmed, ">")
		if end < 0 {
			return ""
		}
		name := strings.Fields(trimmed[1:end])
		if len(name) == 0 {
			return ""
		}
		closing := "</" + name[0] + ">"
		at := strings.Index(trimmed, closing)
		if at < 0 {
			return ""
		}
		trimmed = strings.TrimSpace(trimmed[at+len(closing):])
	}
	return trimmed
}
