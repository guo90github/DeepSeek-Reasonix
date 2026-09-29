package recap

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"reasonix/internal/projectiondb"
)

const schemaV1 = `
CREATE TABLE IF NOT EXISTS recap_records (
    path TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL DEFAULT '',
    goal TEXT NOT NULL DEFAULT '',
    actions TEXT NOT NULL DEFAULT '',
    conclusion TEXT NOT NULL DEFAULT '',
    follow_ups TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    generated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS recap_pending (
    path TEXT PRIMARY KEY,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0
);
`

var migrations = []projectiondb.Migration{{
	Version: 1,
	Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV1)
		return err
	},
}}

// Options locates one process's recap projection. An empty path or an
// unavailable cache directory falls back to memory, matching every other
// disposable projection.
type Options struct {
	Path     string
	InMemory bool
	Now      func() time.Time
}

// Store is the disposable recap projection. It is never the only copy of the
// conversation: deleting it loses recaps and nothing else.
type Store struct {
	handle *projectiondb.Handle
}

// Open opens (and migrates) the recap projection.
func Open(ctx context.Context, opts Options) (*Store, error) {
	handle, err := projectiondb.Open(ctx, projectiondb.OpenOptions{
		Path:         opts.Path,
		MemoryName:   "session-recap",
		Migrations:   migrations,
		InMemory:     opts.InMemory,
		Now:          opts.Now,
		SecureDelete: true,
		AutoVacuum:   true,
	})
	if err != nil {
		return nil, err
	}
	return &Store{handle: handle}, nil
}

// Close releases the projection. Recaps are disposable, so callers may also
// simply delete the file.
func (s *Store) Close() error {
	if s == nil || s.handle == nil {
		return nil
	}
	return s.handle.DB.Close()
}

// DB exposes the underlying handle for diagnostics and reindexing.
func (s *Store) DB() *sql.DB {
	if s == nil || s.handle == nil {
		return nil
	}
	return s.handle.DB
}

// Get returns the stored recap for a path.
func (s *Store) Get(ctx context.Context, path string) (Record, bool, error) {
	if s == nil || s.handle == nil {
		return Record{}, false, nil
	}
	row := s.handle.DB.QueryRowContext(ctx, `SELECT path,fingerprint,goal,actions,conclusion,follow_ups,model,prompt_version,generated_at
		FROM recap_records WHERE path = ?`, path)
	var rec Record
	var generatedAt int64
	if err := row.Scan(&rec.Path, &rec.Fingerprint, &rec.Goal, &rec.Actions, &rec.Conclusion,
		&rec.FollowUps, &rec.Model, &rec.PromptVersion, &generatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	rec.GeneratedAt = time.Unix(0, generatedAt)
	return rec, true, nil
}

// Put stores a recap and clears any pending marker for the same path.
func (s *Store) Put(ctx context.Context, rec Record) error {
	if s == nil || s.handle == nil {
		return nil
	}
	tx, err := s.handle.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO recap_records
		(path,fingerprint,goal,actions,conclusion,follow_ups,model,prompt_version,generated_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET fingerprint=excluded.fingerprint,goal=excluded.goal,
		actions=excluded.actions,conclusion=excluded.conclusion,follow_ups=excluded.follow_ups,
		model=excluded.model,prompt_version=excluded.prompt_version,generated_at=excluded.generated_at`,
		rec.Path, rec.Fingerprint, rec.Goal, rec.Actions, rec.Conclusion, rec.FollowUps,
		rec.Model, rec.PromptVersion, rec.GeneratedAt.UnixNano()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM recap_pending WHERE path = ?`, rec.Path); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete drops one recap, used when a session leaves the visible set.
func (s *Store) Delete(ctx context.Context, path string) error {
	if s == nil || s.handle == nil {
		return nil
	}
	if _, err := s.handle.DB.ExecContext(ctx, `DELETE FROM recap_records WHERE path = ?`, path); err != nil {
		return err
	}
	_, err := s.handle.DB.ExecContext(ctx, `DELETE FROM recap_pending WHERE path = ?`, path)
	return err
}

// MarkPending records a failed attempt so the session is retried later instead
// of being silently dropped.
func (s *Store) MarkPending(ctx context.Context, path, reason string, now time.Time) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx, `INSERT INTO recap_pending (path,attempts,last_error,updated_at)
		VALUES (?,1,?,?)
		ON CONFLICT(path) DO UPDATE SET attempts=attempts+1,last_error=excluded.last_error,updated_at=excluded.updated_at`,
		path, reason, now.UnixNano())
	return err
}

// List returns every stored recap, newest first.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	if s == nil || s.handle == nil {
		return nil, nil
	}
	rows, err := s.handle.DB.QueryContext(ctx, `SELECT path,fingerprint,goal,actions,conclusion,follow_ups,model,prompt_version,generated_at
		FROM recap_records ORDER BY generated_at DESC, path ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Record{}
	for rows.Next() {
		var rec Record
		var generatedAt int64
		if err := rows.Scan(&rec.Path, &rec.Fingerprint, &rec.Goal, &rec.Actions, &rec.Conclusion,
			&rec.FollowUps, &rec.Model, &rec.PromptVersion, &generatedAt); err != nil {
			return nil, err
		}
		rec.GeneratedAt = time.Unix(0, generatedAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Counts reports stored recaps and sessions still waiting for one.
func (s *Store) Counts(ctx context.Context) (records, pending int, err error) {
	if s == nil || s.handle == nil {
		return 0, 0, nil
	}
	if err = s.handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recap_records`).Scan(&records); err != nil {
		return 0, 0, err
	}
	err = s.handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recap_pending`).Scan(&pending)
	return records, pending, err
}
