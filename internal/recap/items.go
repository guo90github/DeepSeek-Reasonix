package recap

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// OpenItem is one unfinished item a person kept from a session's handoff notes.
// It belongs to a project and stays open until someone closes it: the session
// that continues the work may be the tenth after the note was written, so
// nothing here assumes an immediate successor.
//
// Where such an item is delivered is not decided here: this file only keeps the
// list and answers "what is still open for this project".
type OpenItem struct {
	ID       string
	Project  string
	Body     string
	Evidence string
	From     string
	OpenedAt time.Time
	ClosedAt time.Time
}

// Open reports whether the item is still waiting for someone.
func (i OpenItem) Open() bool { return i.ClosedAt.IsZero() }

// TooOld reports whether the item has aged past automatic offers. It stays on the
// list: retiring an offer is not the same as calling the work done.
func (i OpenItem) TooOld(now time.Time) bool { return now.Sub(i.OpenedAt) > offerWindow }

// Offerable reports whether an automatic offer may still include this item.
func (i OpenItem) Offerable(now time.Time) bool { return i.Open() && !i.TooOld(now) }

// ProjectOf names the project a session belongs to: the sessions directory's
// parent, which is what both the per-project session roots and the archive use.
func ProjectOf(sessionPath string) string {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return ""
	}
	return ProjectOfDir(filepath.Dir(path))
}

// ProjectOfDir names the project that owns a sessions directory.
func ProjectOfDir(sessionDir string) string {
	dir := strings.TrimSpace(sessionDir)
	if dir == "" {
		return ""
	}
	if filepath.Base(dir) == "sessions" {
		return filepath.Dir(dir)
	}
	return dir
}

// KeepOpen records one handoff note as an open item of its project.
func (s *Store) KeepOpen(ctx context.Context, item OpenItem, now time.Time) error {
	if s == nil || s.handle == nil {
		return nil
	}
	if strings.TrimSpace(item.Body) == "" || strings.TrimSpace(item.Project) == "" {
		return nil
	}
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = HashEntry(KindHandoff, item.Body)
	}
	_, err := s.handle.DB.ExecContext(ctx, `INSERT INTO recap_open_items
		(id,project,body,evidence,from_path,opened_at,closed_at) VALUES (?,?,?,?,?,?,0)
		ON CONFLICT(id) DO UPDATE SET body=excluded.body,evidence=excluded.evidence,
		from_path=excluded.from_path,closed_at=0`,
		id, item.Project, strings.TrimSpace(item.Body), strings.TrimSpace(item.Evidence),
		strings.TrimSpace(item.From), now.UnixNano())
	return err
}

// CloseOpen marks an item handled. A closed item is kept rather than deleted so
// the page can still say it was dealt with.
func (s *Store) CloseOpen(ctx context.Context, id string, now time.Time) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx,
		`UPDATE recap_open_items SET closed_at = ? WHERE id = ? AND closed_at = 0`,
		now.UnixNano(), strings.TrimSpace(id))
	return err
}

// ReopenOpen undoes CloseOpen, for a mis-click.
func (s *Store) ReopenOpen(ctx context.Context, id string) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx,
		`UPDATE recap_open_items SET closed_at = 0 WHERE id = ?`, strings.TrimSpace(id))
	return err
}

// OpenItemsForProject returns a project's items, open ones first.
func (s *Store) OpenItemsForProject(ctx context.Context, project string) ([]OpenItem, error) {
	if s == nil || s.handle == nil {
		return nil, nil
	}
	rows, err := s.handle.DB.QueryContext(ctx, `SELECT id,project,body,evidence,from_path,opened_at,closed_at
		FROM recap_open_items WHERE project = ? ORDER BY closed_at = 0 DESC, opened_at DESC`, strings.TrimSpace(project))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []OpenItem{}
	for rows.Next() {
		var item OpenItem
		var openedAt, closedAt int64
		if err := rows.Scan(&item.ID, &item.Project, &item.Body, &item.Evidence,
			&item.From, &openedAt, &closedAt); err != nil {
			return nil, err
		}
		item.OpenedAt = time.Unix(0, openedAt)
		if closedAt != 0 {
			item.ClosedAt = time.Unix(0, closedAt)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// OpenItemsFor returns the project's items that are still waiting.
func (s *Store) OpenItemsFor(ctx context.Context, project string) ([]OpenItem, error) {
	items, err := s.OpenItemsForProject(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]OpenItem, 0, len(items))
	for _, item := range items {
		if item.Open() {
			out = append(out, item)
		}
	}
	return out, nil
}

const (
	// maxMatchedItems bounds one offer: a long list is noise, not a handoff.
	maxMatchedItems = 3
	// offerWindow retires an item from automatic offers. Past it the subject has
	// usually moved on — and a list that only grows is what makes an unrelated
	// turn match something.
	offerWindow = 30 * 24 * time.Hour
	// consideredLimit caps how many of a project's newest items may match at all,
	// so a backlog of still-young items cannot flood the offer either.
	consideredLimit = 20
)

// MatchOpenItems returns the project's open items that the turn appears to be
// about. Only items young enough to be about current work are considered, newest
// first and never more than a handful: offering work the person did not mean to
// continue costs more than staying quiet.
func MatchOpenItems(items []OpenItem, turn string, now time.Time) []OpenItem {
	return matchOpenItems(items, turn, now, tokens.matches)
}

// StrongMatchOpenItems returns only the items the turn names outright — the same
// path, command, or id. It backs the offer on a later turn that does not say it
// continues anything: shared prose is not enough there, a named thing is.
func StrongMatchOpenItems(items []OpenItem, turn string, now time.Time) []OpenItem {
	return matchOpenItems(items, turn, now, tokens.names)
}

// matchOpenItems weighs the turn against the project's young items, keeping the
// newest consideredLimit of them and reporting at most maxMatchedItems.
func matchOpenItems(items []OpenItem, turn string, now time.Time, about func(tokens, string) bool) []OpenItem {
	text := strings.ToLower(strings.TrimSpace(turn))
	if text == "" {
		return nil
	}
	young := make([]OpenItem, 0, len(items))
	for _, item := range items {
		if item.Offerable(now) {
			young = append(young, item)
		}
	}
	sort.SliceStable(young, func(i, j int) bool { return young[i].OpenedAt.After(young[j].OpenedAt) })
	if len(young) > consideredLimit {
		young = young[:consideredLimit]
	}
	var out []OpenItem
	for _, item := range young {
		if !about(tokensOf(item), text) {
			continue
		}
		out = append(out, item)
		if len(out) == maxMatchedItems {
			break
		}
	}
	return out
}

const (
	// minIdentifierLen keeps a token long enough to name one thing: a path, a
	// file, a command, or an id.
	minIdentifierLen = 5
	cjkPairLen       = 2
	// What a turn must carry to be about one item, calibrated on real sessions:
	// one shared identifier is ordinary, because a project's sessions name the
	// same files constantly — measured alone it fired on 40% of unrelated
	// openers in the same project.
	sharedIdentifiersNeeded = 2
	sharedPairsFloor        = 2
	sharedPairsAlone        = 3
	pairStride              = 2
	maxPairs                = 120
	maxIDs                  = 40
)

// tokens are the two kinds of evidence one item offers.
type tokens struct {
	identifiers []string
	pairs       []string
}

// names is the mid-conversation test: a turn that never says it continues
// anything must name the item's own things, and corroborate them.
func (t tokens) names(lowerTurn string) bool {
	identifiers := t.identifiersIn(lowerTurn)
	return identifiers >= sharedIdentifiersNeeded ||
		(identifiers >= 1 && t.pairsIn(lowerTurn) >= sharedPairsFloor)
}

// matches reports whether the turn carries this item's subject: enough named
// things, or a named thing with real prose agreement, or prose agreement alone.
func (t tokens) matches(lowerTurn string) bool {
	identifiers := t.identifiersIn(lowerTurn)
	if identifiers >= sharedIdentifiersNeeded {
		return true
	}
	pairs := t.pairsIn(lowerTurn)
	if identifiers >= 1 && pairs >= sharedPairsFloor {
		return true
	}
	return pairs >= sharedPairsAlone
}

// identifiersIn counts how many of the item's identifiers the turn names.
func (t tokens) identifiersIn(lowerTurn string) int {
	hits := 0
	for _, identifier := range t.identifiers {
		if strings.Contains(lowerTurn, identifier) {
			hits++
		}
	}
	return hits
}

// pairsIn counts the distinct character pairs the turn carries.
func (t tokens) pairsIn(lowerTurn string) int {
	seen := make(map[string]struct{}, len(t.pairs))
	hits := 0
	for _, pair := range t.pairs {
		if _, ok := seen[pair]; ok {
			continue
		}
		seen[pair] = struct{}{}
		if strings.Contains(lowerTurn, pair) {
			hits++
		}
	}
	return hits
}

func tokensOf(item OpenItem) tokens { return tokenize(item.Body + " " + item.Evidence) }

// tokenize reads the matchable tokens out of a note: identifiers kept whole, and
// CJK cut into pairs on a stride, so the same subject written slightly
// differently still shares pieces.
func tokenize(text string) tokens {
	var out tokens
	var identifier strings.Builder
	var cjk []rune
	flushIdentifier := func() {
		if identifier.Len() >= minIdentifierLen {
			out.identifiers = append(out.identifiers, identifier.String())
		}
		identifier.Reset()
	}
	flushCJK := func() {
		for i := 0; i+pairStride <= len(cjk); i += pairStride {
			if len(out.pairs) == maxPairs {
				break
			}
			out.pairs = append(out.pairs, string(cjk[i:i+pairStride]))
		}
		cjk = cjk[:0]
	}
	for _, r := range strings.ToLower(text) {
		switch {
		case isIdentifierRune(r):
			flushCJK()
			identifier.WriteRune(r)
		case r >= 128:
			flushIdentifier()
			cjk = append(cjk, r)
		default:
			flushIdentifier()
			flushCJK()
		}
	}
	flushIdentifier()
	flushCJK()
	if len(out.identifiers) > maxIDs {
		out.identifiers = out.identifiers[:maxIDs]
	}
	return out
}

// isIdentifierRune accepts what names a thing in this project: letters, digits,
// and the joiners a path or a command uses.
func isIdentifierRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return true
	case r == '/', r == '.', r == '-', r == '_':
		return true
	}
	return false
}
