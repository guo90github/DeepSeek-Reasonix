package recap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
)

// Calibrating the offer rule needs real text on both sides: the notes a person
// actually kept, and the openers sessions actually begin with. This test replays
// one against the other on a live machine and reports how often the rule fires
// where it should and where it should not; it asserts nothing about the numbers
// because the numbers are the result. Both env vars are required, and the
// projection must be a copy the run may migrate.
//
//	RECAP_CALIB_DB=<copy of session-recap/v1.sqlite> \
//	  RECAP_CALIB_ROOT=%APPDATA%\reasonix \
//	  go test ./internal/recap/ -run CalibrateOfferRule -v -count=1
func TestCalibrateOfferRule(t *testing.T) {
	// Never DefaultPath() here: a mistyped env var once resolved to the live
	// cache, and opening it migrated — which drops the rows a person was reading.
	db := strings.TrimSpace(os.Getenv("RECAP_CALIB_DB"))
	root := strings.TrimSpace(os.Getenv("RECAP_CALIB_ROOT"))
	if root == "" || db == "" {
		t.Skip("set RECAP_CALIB_DB (a copy of the projection) and RECAP_CALIB_ROOT (state root)")
	}
	ctx := context.Background()
	store, err := Open(ctx, Options{Path: db})
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
	var items, ownHits, otherPairs, shippedOther, firedTotal int
	rules := offerGrid()
	ownHitsByRule := make([]int, len(rules))
	otherHitsByRule := make([]int, len(rules))
	// Which branch fires on unrelated text decides which knob matters: the
	// identifier branch is the one a whole project shares, so it is the one worth
	// making stricter. The histogram says where the false positives actually sit.
	fired := map[string]int{}
	rarityOwn := make([]int, len(rarityCuts))
	rarityOther := make([]int, len(rarityCuts))
	// Titles are one line per session saying what it was about, and the sidebar
	// keeps one for every session — so before inventing a corpus, measure whether
	// the corpus the product already has is enough.
	titles := map[string]map[string]string{}
	for _, path := range sessions {
		bucket := projectOf[path]
		if bucket == "" {
			continue
		}
		if titles[bucket] == nil {
			titles[bucket] = loadTitles(filepath.Join(bucket, "sessions"))
		}
	}
	titleOwn := make([]int, len(titleRarityCuts))
	titleOther := make([]int, len(titleRarityCuts))
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
			tokens := tokensOf(item)
			// A file the whole project talks about cannot say "this is the work we
			// were discussing": count the other texts naming each identifier here.
			df := dfOver(tokens.identifiers, others)
			ownTitles := make([]string, 0, len(titles[project]))
			for path, title := range titles[project] {
				if path != own && strings.TrimSpace(title) != "" {
					ownTitles = append(ownTitles, title)
				}
			}
			titleDF := dfOver(tokens.identifiers, ownTitles)
			if ownOpener != "" && len(MatchOpenItems(single, ownOpener, now)) == 1 {
				ownHits++
			}
			ids, pairs := sharedIdentifiers(tokens, ownOpener), sharedPairs(tokens, ownOpener)
			for i, candidate := range rules {
				if ownOpener != "" && candidate.hit(ids, pairs) {
					ownHitsByRule[i]++
				}
			}
			for i, cut := range rarityCuts {
				if ownOpener != "" && rarityRule.hit(distinctiveIDs(tokens, ownOpener, df, cut), pairs) {
					rarityOwn[i]++
				}
			}
			for i, cut := range titleRarityCuts {
				if ownOpener != "" && rarityRule.hit(distinctiveIDs(tokens, ownOpener, titleDF, cut), pairs) {
					titleOwn[i]++
				}
			}
			for _, turn := range others {
				turnIDs := sharedIdentifiers(tokens, turn)
				turnPairs := sharedPairs(tokens, turn)
				if len(MatchOpenItems(single, turn, now)) == 1 {
					shippedOther++
					firedTotal++
					fired[fmt.Sprintf("ids=%d pairs=%d", turnIDs, turnPairs)]++
				}
				for i, candidate := range rules {
					if candidate.hit(turnIDs, turnPairs) {
						otherHitsByRule[i]++
					}
				}
				for i, cut := range rarityCuts {
					if rarityRule.hit(distinctiveIDs(tokens, turn, df, cut), turnPairs) {
						rarityOther[i]++
					}
				}
				for i, cut := range titleRarityCuts {
					if rarityRule.hit(distinctiveIDs(tokens, turn, titleDF, cut), turnPairs) {
						titleOther[i]++
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
	for i, candidate := range rules {
		t.Logf("%-26s recall=%d/%d  false-positives=%d/%d (%.1f%%)", candidate.name,
			ownHitsByRule[i], items, otherHitsByRule[i], otherPairs,
			100*float64(otherHitsByRule[i])/float64(max(otherPairs, 1)))
	}
	top := make([]firedBucket, 0, len(fired))
	for key, n := range fired {
		top = append(top, firedBucket{key, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	for i, bucket := range top {
		if i == 10 {
			break
		}
		t.Logf("  fp at %-16s %4d (%4.1f%% of all fp)", bucket.key, bucket.n,
			100*float64(bucket.n)/float64(max(firedTotal, 1)))
	}
	for i, cut := range rarityCuts {
		t.Logf("rarity: opener df>=%-3d ordinary | recall=%d/%d  false-positives=%d/%d (%.1f%%)", cut,
			rarityOwn[i], items, rarityOther[i], otherPairs,
			100*float64(rarityOther[i])/float64(max(otherPairs, 1)))
	}
	for i, cut := range titleRarityCuts {
		t.Logf("rarity: title  df>=%-3d ordinary | recall=%d/%d  false-positives=%d/%d (%.1f%%)", cut,
			titleOwn[i], items, titleOther[i], otherPairs,
			100*float64(titleOther[i])/float64(max(otherPairs, 1)))
	}
}

type firedBucket struct {
	key string
	n   int
}

// rule is one candidate offer rule, measured against real text.
type rule struct {
	name string
	hit  func(identifiers, pairs int) bool
}

// rarityRule is the operating point read off the grid, so the rarity rows measure
// the filter rather than a second threshold change.
var rarityRule = rule{
	name: "rarity",
	hit:  func(i, p int) bool { return i >= 3 || (i >= 1 && p >= 2) || p >= 3 },
}

// rarityCuts are the document frequencies to read: an identifier named by at
// least this many of the bucket's other sessions is ordinary inside it.
var rarityCuts = []int{3, 10, 30, 60}

// titleRarityCuts are the same thresholds over the bucket's session titles, which
// are one line per session and already exist — a corpus the product has.
var titleRarityCuts = []int{1, 2, 3}

// loadTitles reads the titles the sidebar shows for one sessions directory.
func loadTitles(dir string) map[string]string {
	out := map[string]string{}
	infos, err := agent.ListSessions(dir)
	if err != nil {
		return out
	}
	for _, info := range infos {
		title := strings.TrimSpace(info.CustomTitle)
		if title == "" {
			title = strings.TrimSpace(info.TopicTitle)
		}
		if title == "" {
			title = strings.TrimSpace(info.Preview)
		}
		out[info.Path] = title
	}
	return out
}

// dfOver counts, for each identifier, how many of the given texts name it.
func dfOver(identifiers []string, texts []string) map[string]int {
	out := make(map[string]int, len(identifiers))
	for _, identifier := range identifiers {
		for _, text := range texts {
			if strings.Contains(strings.ToLower(text), identifier) {
				out[identifier]++
			}
		}
	}
	return out
}

// distinctiveIDs counts the matched identifiers that are not ordinary in the
// bucket — the ones that could still single this item's subject out.
func distinctiveIDs(item tokens, turn string, df map[string]int, cut int) int {
	lower := strings.ToLower(turn)
	hits := 0
	for _, identifier := range item.identifiers {
		if df[identifier] >= cut {
			continue
		}
		if strings.Contains(lower, identifier) {
			hits++
		}
	}
	return hits
}

// offerGrid sweeps the three knobs the rule has: how many named things count on
// their own, how many character pairs make one named thing enough, and how many
// pairs stand alone. The operating point is chosen from the measured table, so
// the sweep exists to be read rather than to pass.
func offerGrid() []rule {
	var out []rule
	for _, minIDs := range []int{2, 3} {
		for _, idPairs := range []int{2, 3} {
			for _, pairsAlone := range []int{3, 4, 5} {
				out = append(out, rule{
					name: fmt.Sprintf("%did | 1id&%dpair | %dpair", minIDs, idPairs, pairsAlone),
					hit: func(i, p int) bool {
						return i >= minIDs || (i >= 1 && p >= idPairs) || p >= pairsAlone
					},
				})
			}
		}
	}
	return out
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
