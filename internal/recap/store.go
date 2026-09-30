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
}, {
	Version: 2,
	Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV2)
		return err
	},
}, {
	Version: 3,
	Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV3)
		return err
	},
}, {
	Version: 4,
	Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV4)
		return err
	},
}, {
	Version: 5,
	Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV5)
		return err
	},
}, {
	Version: 6,
	Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV6)
		return err
	},
}}

// schemaV2 adds the lane's decision log: it is what makes a missing recap
// diagnosable from the projection alone.
const schemaV2 = `
CREATE TABLE IF NOT EXISTS recap_activity (
    at INTEGER NOT NULL,
    stage TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT ''
);
`

// schemaV3 records how far a previous recap already read each session file, so a
// later close reads only what is new instead of the whole session.
const schemaV3 = `
CREATE TABLE IF NOT EXISTS recap_resume (
    path TEXT PRIMARY KEY,
    resume_offset INTEGER NOT NULL DEFAULT 0,
    prefix_hash TEXT NOT NULL DEFAULT '',
    content_digest TEXT NOT NULL DEFAULT '',
    head_text TEXT NOT NULL DEFAULT '',
    user_turns INTEGER NOT NULL DEFAULT 0
);
`

// schemaV4 replaces the four free-text elements with distilled candidate
// entries. Rows written by v2 are dropped rather than carried: they hold the old
// shape, the prompt version bump regenerates them, and an empty card is worse
// than an absent one.
const schemaV4 = `
CREATE TABLE recap_records_v4 (
    path TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL DEFAULT '',
    entries_json TEXT NOT NULL DEFAULT '[]',
    model TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    generated_at INTEGER NOT NULL DEFAULT 0
);
DROP TABLE recap_records;
ALTER TABLE recap_records_v4 RENAME TO recap_records;
`

// schemaV5 records what a person did with each note. It is keyed by content, not
// by session, so a rejected note stays rejected when the session is recapped
// again: dropping a bad note once has to be enough.
const schemaV5 = `
CREATE TABLE IF NOT EXISTS recap_decisions (
    kind TEXT NOT NULL,
    body_hash TEXT NOT NULL,
    choice TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    decided_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (kind, body_hash)
);
`

// schemaV6 keeps the handoff notes a person chose to carry forward. They belong
// to a project and stay open until someone closes them, because the session that
// continues the work is often not the next one.
const schemaV6 = `
CREATE TABLE IF NOT EXISTS recap_open_items (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    from_path TEXT NOT NULL DEFAULT '',
    opened_at INTEGER NOT NULL DEFAULT 0,
    closed_at INTEGER NOT NULL DEFAULT 0
);
`

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
	row := s.handle.DB.QueryRowContext(ctx, `SELECT path,fingerprint,entries_json,model,prompt_version,generated_at
		FROM recap_records WHERE path = ?`, path)
	var rec Record
	var entriesJSON string
	var generatedAt int64
	if err := row.Scan(&rec.Path, &rec.Fingerprint, &entriesJSON, &rec.Model,
		&rec.PromptVersion, &generatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	rec.Entries = decodeEntries(entriesJSON)
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
		(path,fingerprint,entries_json,model,prompt_version,generated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET fingerprint=excluded.fingerprint,
		entries_json=excluded.entries_json,model=excluded.model,
		prompt_version=excluded.prompt_version,generated_at=excluded.generated_at`,
		rec.Path, rec.Fingerprint, encodeEntries(rec.Entries),
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
	if _, err := s.handle.DB.ExecContext(ctx, `DELETE FROM recap_resume WHERE path = ?`, path); err != nil {
		return err
	}
	_, err := s.handle.DB.ExecContext(ctx, `DELETE FROM recap_pending WHERE path = ?`, path)
	return err
}

// Resume returns how far a previous close already read a session file.
func (s *Store) Resume(ctx context.Context, path string) (Resume, bool, error) {
	if s == nil || s.handle == nil {
		return Resume{}, false, nil
	}
	row := s.handle.DB.QueryRowContext(ctx, `SELECT resume_offset,prefix_hash,content_digest,head_text,user_turns
		FROM recap_resume WHERE path = ?`, path)
	var out Resume
	if err := row.Scan(&out.Offset, &out.Hash, &out.Digest, &out.Head, &out.UserTurns); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Resume{}, false, nil
		}
		return Resume{}, false, err
	}
	return out, true, nil
}

// PutResume records how far a read reached, replacing any earlier point.
func (s *Store) PutResume(ctx context.Context, path string, r Resume) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx, `INSERT INTO recap_resume
		(path,resume_offset,prefix_hash,content_digest,head_text,user_turns)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET resume_offset=excluded.resume_offset,
		prefix_hash=excluded.prefix_hash,content_digest=excluded.content_digest,
		head_text=excluded.head_text,user_turns=excluded.user_turns`,
		path, r.Offset, r.Hash, r.Digest, r.Head, r.UserTurns)
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

// ActivityEntry is one recorded lane decision.
type ActivityEntry struct {
	At     time.Time
	Stage  string
	Path   string
	Detail string
}

// Trace records one lane decision. The log is what tells "the close never
// reported" (no row at all) from "the lane refused it" or "it ran and skipped",
// so a missing recap is diagnosable without a debugger attached.
func (s *Store) Trace(ctx context.Context, stage, path, detail string, now time.Time) error {
	if s == nil || s.handle == nil {
		return nil
	}
	_, err := s.handle.DB.ExecContext(ctx,
		`INSERT INTO recap_activity (at,stage,path,detail) VALUES (?,?,?,?)`,
		now.UnixNano(), stage, path, detail)
	return err
}

// Activity returns the most recent lane decisions, newest first.
func (s *Store) Activity(ctx context.Context, limit int) ([]ActivityEntry, error) {
	if s == nil || s.handle == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.handle.DB.QueryContext(ctx,
		`SELECT at,stage,path,detail FROM recap_activity ORDER BY at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ActivityEntry{}
	for rows.Next() {
		var entry ActivityEntry
		var at int64
		if err := rows.Scan(&at, &entry.Stage, &entry.Path, &entry.Detail); err != nil {
			return nil, err
		}
		entry.At = time.Unix(0, at)
		out = append(out, entry)
	}
	return out, rows.Err()
}

// List returns every stored recap, newest first.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	if s == nil || s.handle == nil {
		return nil, nil
	}
	rows, err := s.handle.DB.QueryContext(ctx, `SELECT path,fingerprint,entries_json,model,prompt_version,generated_at
		FROM recap_records ORDER BY generated_at DESC, path ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Record{}
	for rows.Next() {
		var rec Record
		var entriesJSON string
		var generatedAt int64
		if err := rows.Scan(&rec.Path, &rec.Fingerprint, &entriesJSON, &rec.Model,
			&rec.PromptVersion, &generatedAt); err != nil {
			return nil, err
		}
		rec.Entries = decodeEntries(entriesJSON)
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
