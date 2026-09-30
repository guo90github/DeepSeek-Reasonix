package recap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// Resume records what a previous recap already rendered from one session file:
// the byte offset it covered, the hash of the file's first Offset bytes, the
// sidecar digest the file carried then, the rendered head it kept, and how many
// user turns that head accounts for.
type Resume struct {
	Offset    int64
	Hash      string
	Digest    string
	Head      string
	UserTurns int64
}

// CoveringReader is implemented by transcript readers that can render only the
// part a previous read did not cover. text is what to recap — the kept head
// merged with whatever is new, unclipped — and next is the state to record for
// the next close. ok=false keeps the caller on the full read or the replay.
type CoveringReader interface {
	ReadCovered(ctx context.Context, sessionPath string, from Resume, maxBytes int) (text string, next Resume, ok bool, err error)
}

// ReadCovered renders a session for a recap, reusing the read a previous close
// already proved. from.Offset == 0 asks for the full, digest-proven read.
//
// A covered read cannot re-hash the whole file against the sidecar digest — that
// hash is what makes it cheap — so it stands on three cheaper proofs instead: the
// file's first Offset bytes still hash to from.Hash, the sidecar digest has moved
// on since from.Digest (a save records it, so a row appended without one keeps
// the replay), and the new rows parse. The replay stays the fallback for every
// file the transcript cannot prove.
func (FileTranscript) ReadCovered(_ context.Context, sessionPath string, from Resume, maxBytes int) (string, Resume, bool, error) {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return "", Resume{}, false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", Resume{}, false, nil
	}
	if from.Offset > 0 {
		return fileTranscriptCovered(path, fastReadKey(path, info), from, maxBytes)
	}
	rows, covered, content, ok := transcriptFileRows(path)
	if !ok {
		return "", Resume{}, false, nil
	}
	digest, recorded, err := agent.SessionContentDigest(path)
	if err != nil || !recorded {
		return "", Resume{}, false, nil
	}
	text := renderMessages(rows)
	next := Resume{
		Offset:    covered,
		Hash:      prefixHashOf(content, covered),
		Digest:    digest,
		Head:      headPiece(text, maxBytes),
		UserTurns: countUserTurns(rows),
	}
	return text, next, true, nil
}

// fileTranscriptCovered renders only what the recorded offset did not cover.
func fileTranscriptCovered(path, key string, from Resume, maxBytes int) (string, Resume, bool, error) {
	if fastReadUnproven(key) {
		return "", Resume{}, false, nil
	}
	digest, recorded, err := agent.SessionContentDigest(path)
	if err != nil || !recorded {
		return "", Resume{}, false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", Resume{}, false, nil
	}
	if int64(len(content)) < from.Offset {
		return "", Resume{}, false, nil
	}
	if digest == from.Digest && from.Offset < int64(len(content)) {
		return "", Resume{}, false, nil
	}
	if prefixHashOf(content, from.Offset) != from.Hash {
		return "", Resume{}, false, nil
	}
	rows, covered, err := parseTranscriptRows(content[from.Offset:])
	if err != nil {
		markFastReadUnproven(key)
		return "", Resume{}, false, nil
	}
	if len(rows) == 0 {
		return "", Resume{}, false, nil
	}
	next := Resume{
		Offset:    from.Offset + covered,
		Hash:      prefixHashOf(content, from.Offset+covered),
		Digest:    digest,
		UserTurns: from.UserTurns + countUserTurns(rows),
	}
	merged := mergeHead(from.Head, renderMessagesFrom(rows, int(from.UserTurns)), maxBytes)
	next.Head = headPiece(merged, maxBytes)
	return merged, next, true, nil
}

// mergeHead joins the head a previous read kept with the text rendered since. It
// says so explicitly when the join drops the middle: the bytes between them were
// dropped by an earlier read, so no byte count here would be honest.
func mergeHead(head, tail string, max int) string {
	if head == "" {
		return tail
	}
	if max <= 0 || len(head)+len(tail) <= max {
		return head + tail
	}
	return head + "\n…[earlier content omitted]…\n" + tail
}

// prefixHashOf hashes the first n bytes, in this package's 16-hex-digit form.
func prefixHashOf(content []byte, n int64) string {
	if n < 0 {
		n = 0
	}
	if int64(len(content)) < n {
		n = int64(len(content))
	}
	sum := sha256.Sum256(content[:n])
	return hex.EncodeToString(sum[:])[:16]
}

func countUserTurns(msgs []provider.Message) int64 {
	var turns int64
	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			turns++
		}
	}
	return turns
}

// readCovered renders one attempt's transcript: the read a previous close already
// proved, else the full digest-proven read, else the replay. Every proof it can
// establish is recorded, so the next close starts from the furthest point that
// held.
func readCovered(ctx context.Context, g *Generator, store *Store, path string) (string, error) {
	cover, ok := g.opts.Transcript.(CoveringReader)
	if !ok {
		return g.opts.Transcript.Read(ctx, path)
	}
	from := Resume{}
	if recorded, ok, err := store.Resume(ctx, path); err == nil && ok {
		from = recorded
	}
	if text, next, ok, err := cover.ReadCovered(ctx, path, from, g.opts.MaxInputBytes); err == nil && ok {
		g.recordResume(ctx, store, path, next)
		return text, nil
	}
	if from.Offset > 0 {
		if text, next, ok, err := cover.ReadCovered(ctx, path, Resume{}, g.opts.MaxInputBytes); err == nil && ok {
			g.recordResume(ctx, store, path, next)
			return text, nil
		}
	}
	return g.opts.Transcript.Read(ctx, path)
}

// recordResume stores the point a read reached. The projection is disposable, so
// a failure costs the next close a full read and nothing else.
func (g *Generator) recordResume(ctx context.Context, store *Store, path string, next Resume) {
	if err := store.PutResume(ctx, path, next); err != nil {
		g.trace(ctx, store, "resume", path, "not recorded: "+err.Error())
	}
}
