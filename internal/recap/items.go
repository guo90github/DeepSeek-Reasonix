package recap

import (
	"context"
	"path/filepath"
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

// maxMatchedItems bounds one offer: a long list is noise, not a handoff.
const maxMatchedItems = 3

// MatchOpenItems returns the project's open items that the turn appears to be
// about. The test is deliberately literal and local: it runs on a session's
// first turn and on continuation cues, and offering work the person did not mean
// to continue costs more than staying quiet.
func MatchOpenItems(items []OpenItem, turn string) []OpenItem {
	text := strings.ToLower(strings.TrimSpace(turn))
	if text == "" {
		return nil
	}
	var out []OpenItem
	for _, item := range items {
		if !item.Open() || !tokensOf(item).matches(text) {
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
	// cjkPairHits is how many shared character pairs a CJK-only match needs: one
	// shared pair is ordinary prose, two is a subject.
	cjkPairHits = 2
	pairStride  = 2
	maxPairs    = 120
	maxIDs      = 40
)

// tokens are the two kinds of evidence one item offers.
type tokens struct {
	identifiers []string
	pairs       []string
}

// matches reports whether the turn carries this item's subject. One identifier
// (a path, a command, a field) is decisive; short CJK pieces need corroboration.
func (t tokens) matches(lowerTurn string) bool {
	for _, identifier := range t.identifiers {
		if strings.Contains(lowerTurn, identifier) {
			return true
		}
	}
	hits := 0
	for _, pair := range t.pairs {
		if !strings.Contains(lowerTurn, pair) {
			continue
		}
		hits++
		if hits >= cjkPairHits {
			return true
		}
	}
	return false
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
