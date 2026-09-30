package recap

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// DecisionAccept and DecisionReject are the two ways a person can settle one
// note. A rejected note is remembered by content, so the same note never comes
// back from a later generation of the same session.
const (
	DecisionAccept = "accept"
	DecisionReject = "reject"
)

// Decision is one person's answer to one note.
type Decision struct {
	Kind      string
	Body      string
	Choice    string
	DecidedAt time.Time
}

// HashEntry keys a decision to the note's content rather than to the session, so
// a rejected note stays rejected when the session is recapped again.
func HashEntry(kind, body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(body)))
	return hex.EncodeToString(sum[:])[:16]
}

// Decide records what a person chose for one note, replacing any earlier choice.
func (s *Store) Decide(ctx context.Context, kind, body, choice string, now time.Time) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx, `INSERT INTO recap_decisions
		(kind,body_hash,choice,body,decided_at) VALUES (?,?,?,?,?)
		ON CONFLICT(kind,body_hash) DO UPDATE SET choice=excluded.choice,
		body=excluded.body,decided_at=excluded.decided_at`,
		strings.TrimSpace(kind), HashEntry(kind, body), choice, strings.TrimSpace(body), now.UnixNano())
	return err
}

// ClearDecision drops one note's choice, so a mis-click is reversible.
func (s *Store) ClearDecision(ctx context.Context, kind, body string) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx,
		`DELETE FROM recap_decisions WHERE kind = ? AND body_hash = ?`,
		strings.TrimSpace(kind), HashEntry(kind, body))
	return err
}

// Decisions returns every recorded choice, keyed by HashEntry.
func (s *Store) Decisions(ctx context.Context) (map[string]Decision, error) {
	out := map[string]Decision{}
	if s == nil || s.handle == nil {
		return out, nil
	}
	rows, err := s.handle.DB.QueryContext(ctx,
		`SELECT kind,body_hash,choice,body,decided_at FROM recap_decisions`)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, hash, choice, body string
		var at int64
		if err := rows.Scan(&kind, &hash, &choice, &body, &at); err != nil {
			return out, err
		}
		out[hash] = Decision{Kind: kind, Body: body, Choice: choice, DecidedAt: time.Unix(0, at)}
	}
	return out, rows.Err()
}

// Rejected reports which of the notes someone already ruled out.
func (s *Store) Rejected(ctx context.Context) (map[string]bool, error) {
	decisions, err := s.Decisions(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for hash, decision := range decisions {
		if decision.Choice == DecisionReject {
			out[hash] = true
		}
	}
	return out, nil
}

// ErrNoDecision reports that a note carries no recorded choice.
var ErrNoDecision = errors.New("recap: note carries no decision")

// DecisionFor returns one note's recorded choice.
func (s *Store) DecisionFor(ctx context.Context, kind, body string) (Decision, error) {
	if s == nil || s.handle == nil {
		return Decision{}, ErrNoDecision
	}
	row := s.handle.DB.QueryRowContext(ctx,
		`SELECT choice,decided_at FROM recap_decisions WHERE kind = ? AND body_hash = ?`,
		strings.TrimSpace(kind), HashEntry(kind, body))
	var choice string
	var at int64
	if err := row.Scan(&choice, &at); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Decision{}, ErrNoDecision
		}
		return Decision{}, err
	}
	return Decision{Kind: kind, Body: body, Choice: choice, DecidedAt: time.Unix(0, at)}, nil
}
