package board

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"reasonix/internal/agentbus/jsonl"
	"reasonix/internal/fileutil"
)

const (
	checkpointFileName = "snapshot.json"
	checkpointVersion  = 1
	// checkpointEvery is how many newly applied ops make a fresh snapshot worth its write.
	checkpointEvery = 256
	// tailPrintWindow is how much of the folded prefix's tail is fingerprinted, so a log that
	// shrank or was replaced under the snapshot is caught without re-reading the prefix.
	tailPrintWindow = 256
)

// checkpoint is a folded prefix of the op log: the state as of UptoSeq, plus the idempotency
// table that state needs. OpIDs travels with it because a duplicate op id is only recognisable
// against that table — State does not serialize it, and without it a retried delivery would be
// applied a second time.
type checkpoint struct {
	SchemaVersion int               `json:"schemaVersion"`
	UptoSeq       uint64            `json:"uptoSeq"`
	UptoOffset    int64             `json:"uptoOffset"`
	TailPrint     string            `json:"tailPrint"`
	State         *State            `json:"state"`
	OpIDs         map[string]uint64 `json:"opIds"`
}

func (b *Board) checkpointPath() string { return filepath.Join(b.dir, checkpointFileName) }

// writeCheckpoint records st as the folded prefix ending at the log's current end. It is an
// optimization, so a failure never fails the write that triggered it: the next read folds more.
// The write replaces the file atomically, so a crashed attempt leaves the previous snapshot.
func (b *Board) writeCheckpoint(st *State) error {
	info, err := os.Stat(b.logPath)
	if err != nil {
		return err
	}
	print, err := tailPrint(b.logPath, info.Size())
	if err != nil {
		return err
	}
	raw, err := json.Marshal(checkpoint{
		SchemaVersion: checkpointVersion,
		UptoSeq:       st.Seq,
		UptoOffset:    info.Size(),
		TailPrint:     print,
		State:         st,
		OpIDs:         st.OpIDs,
	})
	if err != nil {
		return fmt.Errorf("board: encode checkpoint: %w", err)
	}
	if err := fileutil.AtomicWriteFile(b.checkpointPath(), raw, filePerm); err != nil {
		return fmt.Errorf("board: write checkpoint: %w", err)
	}
	return nil
}

// loadCheckpoint returns the folded prefix, the log size it accounts for, how many ops it covers,
// and whether it could be trusted. Everything it cannot verify — a missing or torn file, an
// unknown version, a log that shrank, a prefix whose tail bytes changed, a tail that does not
// continue the seq — answers "no", which means folding the whole log rather than trusting a maybe.
func (b *Board) loadCheckpoint() (*State, int64, int, bool) {
	raw, err := os.ReadFile(b.checkpointPath())
	if err != nil {
		return nil, 0, 0, false
	}
	var ck checkpoint
	if err := json.Unmarshal(raw, &ck); err != nil || ck.SchemaVersion != checkpointVersion || ck.State == nil {
		return nil, 0, 0, false
	}
	info, err := os.Stat(b.logPath)
	if err != nil || info.Size() < ck.UptoOffset || ck.State.Seq != ck.UptoSeq {
		return nil, 0, 0, false
	}
	print, err := tailPrint(b.logPath, ck.UptoOffset)
	if err != nil || print != ck.TailPrint {
		return nil, 0, 0, false
	}
	tail, err := jsonl.ReadFrom[Op](b.logPath, ck.UptoOffset)
	if err != nil {
		return nil, 0, 0, false
	}
	switch {
	case len(tail.Items) == 0 && info.Size() != ck.UptoOffset:
		// Bytes after the offset that decoded to nothing: not the log this was folded from.
		return nil, 0, 0, false
	case len(tail.Items) > 0 && tail.Items[0].Seq != ck.UptoSeq+1:
		// The prefix no longer ends where the checkpoint says it does.
		return nil, 0, 0, false
	}
	covered := ck.State.Applied
	st := ck.State
	if st.Nodes == nil {
		st.Nodes = map[string]*Node{}
	}
	if ck.OpIDs == nil {
		ck.OpIDs = map[string]uint64{}
	}
	st.OpIDs = ck.OpIDs
	st = foldFrom(st, tail.Items)
	// Damage counters are cumulative: what the prefix already counted, plus this tail's share.
	st.Truncated += tail.Truncated
	st.Skipped += tail.Skipped
	return st, info.Size(), covered, true
}

// tailPrint fingerprints the log bytes just before offset. The checkpoint exists to skip the
// prefix, so its proof must not read the prefix: a window at the boundary is enough to notice a
// truncation or a replacement, and the seq of the first op after the offset proves the boundary.
func tailPrint(path string, offset int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	start := max(offset-tailPrintWindow, 0)
	buf := make([]byte, offset-start)
	if len(buf) > 0 {
		if _, err := f.ReadAt(buf, start); err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}
