package board

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/filelock"
)

const (
	defaultLockWait = 5 * time.Second
	maxSweepPerCall = 256
	// nodeRateWindow is the window the ceiling names: one node, per minute.
	nodeRateWindow = time.Minute
)

// Board is one coordination scope's op log. It owns no memory state: every
// write folds the log, so a process that dies loses nothing but work in flight.
type Board struct {
	dir      string
	logPath  string
	lockPath string
	// cache is the state this handle folded last, kept so the next write folds
	// nothing instead of the whole log (§11.4 boundary ①). Writers on one handle are
	// serialized by the board file lock, and readers never touch it.
	cache      *State
	cachedSize int64
	// checkpointed is how many ops the snapshot on disk already covers, so a writer only pays
	// for a new one when it would shorten the next cold read.
	checkpointed int
	// limits is the operator's allowance for writes; zero leaves every bound off.
	limits Limits
}

// Limits bound what one writer may do to one node. The zero value leaves every bound off:
// the kernel invents no ceilings on the operator's behalf.
type Limits struct {
	// NodeRatePerMinute caps how often one node may move within one nodeRateWindow.
	NodeRatePerMinute int
}

// Receipt is what a caller learns about a write.
type Receipt struct {
	Seq       uint64    `json:"seq"`
	Duplicate bool      `json:"duplicate,omitempty"`
	State     NodeState `json:"state,omitempty"`
}

// Open prepares a board directory. The caller supplies the directory (S1 does
// not resolve state home), and the log is created lazily on first write. At most
// one Limits may be passed; a second one is ignored.
func Open(dir string, lims ...Limits) (*Board, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("board: empty directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("board: resolve directory: %w", err)
	}
	if err := ensureDir(abs); err != nil {
		return nil, err
	}
	b := &Board{
		dir:      abs,
		logPath:  filepath.Join(abs, logFileName),
		lockPath: filepath.Join(abs, lockFileName),
	}
	if len(lims) > 0 {
		b.limits = lims[0]
	}
	return b, nil
}

// Dir is the directory this board reads and writes.
func (b *Board) Dir() string { return b.dir }

// foldedState returns the state this handle folds to, reusing the state folded last
// when the file has not grown behind its back, so a write and a read cost a stat
// instead of a full read and fold (T4-8). A cold handle starts from the snapshot when one is
// there, which is why a read no longer folds the whole log.
func (b *Board) foldedState() (*State, error) {
	info, err := os.Stat(b.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, fmt.Errorf("board: stat log: %w", err)
	}
	if b.cache != nil && b.cachedSize == info.Size() {
		return b.cache, nil
	}
	if st, size, covered, ok := b.loadCheckpoint(); ok {
		b.cache, b.cachedSize, b.checkpointed = st, size, covered
		return st, nil
	}
	read, err := readLog(b.logPath)
	if err != nil {
		return nil, err
	}
	st := Fold(read.Ops)
	st.Truncated = read.Truncated
	st.Skipped = read.Skipped
	b.cache, b.cachedSize = st, info.Size()
	b.checkpointed = 0
	return st, nil
}

// keepCache remembers that this state now matches the file on disk.
func (b *Board) keepCache(st *State) {
	info, err := os.Stat(b.logPath)
	if err != nil {
		b.dropCache()
		return
	}
	b.cache, b.cachedSize = st, info.Size()
}

// dropCache forgets the folded state, so the next write reads the log again.
func (b *Board) dropCache() {
	b.cache, b.cachedSize = nil, 0
}

// Apply validates op against the folded log and appends it, returning the
// receipt. A retried op id returns the first receipt instead of appending a
// second record, so delivery retries cannot multiply an op.
func (b *Board) Apply(ctx context.Context, op Op) (Receipt, error) {
	now := time.Now().UTC()
	if op.At.IsZero() {
		op.At = now
	}
	if strings.TrimSpace(op.ID) == "" {
		op.ID = DeriveID(op)
	}
	release, err := b.lock(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer release()

	st, err := b.foldedState()
	if err != nil {
		return Receipt{}, err
	}
	if op.Verb == VerbNoProgress {
		return Receipt{}, reject(op.Verb, op.Node, ReasonSystemOnly)
	}
	if seq, dup := st.OpIDs[op.ID]; dup {
		// A duplicate is a retry, not the hot path: read the log once to compare what
		// this id was first used for.
		previous, err := readLog(b.logPath)
		if err != nil {
			return Receipt{}, err
		}
		if !sameIntent(previous.Ops, op) {
			return Receipt{}, reject(op.Verb, op.Node, ReasonIdempotencyConflict)
		}
		return Receipt{Seq: seq, Duplicate: true}, nil
	}
	if err := validateFreshness(op, now); err != nil {
		return Receipt{}, err
	}
	if err := b.validateNodeRate(op); err != nil {
		return Receipt{}, err
	}
	op.Seq = st.Seq + 1
	if err := applyOp(st, op); err != nil {
		return Receipt{}, err
	}
	// Fold records the log's seq, id table and applied count as it reads; a cached
	// state has to be told every one of them, or the next write mints a seq twice,
	// stops recognising retries, or reports an applied count that never moved.
	st.Seq = op.Seq
	st.OpIDs[op.ID] = op.Seq
	st.Applied++
	if err := appendOp(b.logPath, op); err != nil {
		return Receipt{}, err
	}
	b.keepCache(st)
	// The snapshot is an optimization and the op is already committed, so a snapshot that could
	// not be taken must not turn a landed write into a failure: the next cold read folds more.
	if st.Applied-b.checkpointed >= checkpointEvery {
		if err := b.writeCheckpoint(st); err == nil {
			b.checkpointed = st.Applied
		}
	}
	return Receipt{Seq: op.Seq, State: nodeState(st, op.Node)}, nil
}

// validateFreshness refuses an op whose own deadline has already lapsed when it
// is written. Replay must not read the clock, so "this deadline is in the
// future" is a write-time guard, never a transition rule.
func validateFreshness(op Op, now time.Time) error {
	switch op.Verb {
	case VerbClaim, VerbHeartbeat:
		if op.Deadline.IsZero() {
			return reject(op.Verb, op.Node, ReasonMissingDeadline)
		}
		if !op.Deadline.After(now) {
			return reject(op.Verb, op.Node, ReasonDeadlineNotFuture)
		}
	}
	return nil
}

// validateNodeRate refuses an op that moves its node more often than the operator allows.
// It is a write-time rule measured off the log's own timestamps, so a replay never re-judges
// it. A heartbeat renews a lease and a system reclamation is not the writer moving the node,
// so neither counts — throttling a renewal would lapse the very lease the cluster leans on.
func (b *Board) validateNodeRate(op Op) error {
	if b.limits.NodeRatePerMinute <= 0 || op.Node == "" || op.Verb == VerbHeartbeat {
		return nil
	}
	previous, err := readLog(b.logPath)
	if err != nil {
		return err
	}
	cutoff := op.At.Add(-nodeRateWindow)
	moved := 0
	for _, prev := range previous.Ops {
		if prev.Node != op.Node || prev.Verb == VerbHeartbeat || prev.Actor == ActorSystem {
			continue
		}
		if !prev.At.Before(cutoff) {
			moved++
		}
	}
	if moved >= b.limits.NodeRatePerMinute {
		return reject(op.Verb, op.Node, ReasonRateLimited)
	}
	return nil
}

// sameIntent reports whether the op already carrying this id asks for the same
// thing. A client id reused for a different intent is a collision, not a retry,
// and must not be answered with a silent duplicate.
func sameIntent(existing []Op, op Op) bool {
	for _, prev := range existing {
		if prev.ID != op.ID {
			continue
		}
		return DeriveID(prev) == DeriveID(op)
	}
	return true
}

// ApplyAll applies ops in order, stopping at the first refusal.
func (b *Board) ApplyAll(ctx context.Context, ops ...Op) ([]Receipt, error) {
	out := make([]Receipt, 0, len(ops))
	for _, op := range ops {
		rec, err := b.Apply(ctx, op)
		if err != nil {
			return out, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// Snapshot reads the log under a shared lock and folds it.
func (b *Board) Snapshot(ctx context.Context, now time.Time) (*State, error) {
	release, err := b.lockShared(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	st, err := b.foldedState()
	if err != nil {
		return nil, err
	}
	// A caller keeps what it is given, so the folded state is copied out rather than
	// shared: the next write appends to those very nodes (T4-8 ②).
	return st.Clone(), nil
}

// Clone returns a copy a caller may keep and mutate. Node slices are copied too,
// because a later write appends to them through the cache this clone came from.
func (st *State) Clone() *State {
	if st == nil {
		return nil
	}
	out := *st
	out.Nodes = make(map[string]*Node, len(st.Nodes))
	for id, n := range st.Nodes {
		if n == nil {
			out.Nodes[id] = nil
			continue
		}
		clone := *n
		clone.Deps = append([]string(nil), n.Deps...)
		clone.Requesters = append([]string(nil), n.Requesters...)
		clone.Asserts = append([]Assertion(nil), n.Asserts...)
		clone.Refutes = append([]Refutation(nil), n.Refutes...)
		if n.Bounds != nil {
			bounds := *n.Bounds
			clone.Bounds = &bounds
		}
		out.Nodes[id] = &clone
	}
	out.OpIDs = make(map[string]uint64, len(st.OpIDs))
	maps.Copy(out.OpIDs, st.OpIDs)
	return &out
}

// Sweep reclaims expired claims. Any participant may call it at the start of a
// write; there is no scanning daemon, and each reclamation is idempotent.
func (b *Board) Sweep(ctx context.Context, now time.Time) ([]Receipt, error) {
	release, err := b.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	st, err := b.foldedState()
	if err != nil {
		return nil, err
	}
	var out []Receipt
	for _, id := range st.sortedNodeIDs() {
		if len(out) >= maxSweepPerCall {
			break
		}
		n := st.Nodes[id]
		if !claimExpired(n, now) {
			continue
		}
		op := ReclaimOp(n, now)
		if _, dup := st.OpIDs[op.ID]; dup {
			continue
		}
		op.Seq = st.Seq + 1
		if err := applyOp(st, op); err != nil {
			continue
		}
		st.Seq = op.Seq
		st.OpIDs[op.ID] = op.Seq
		st.Applied++
		if err := appendOp(b.logPath, op); err != nil {
			b.dropCache()
			return out, err
		}
		out = append(out, Receipt{Seq: op.Seq, State: StateOpen})
	}
	if len(out) > 0 {
		b.keepCache(st)
	}
	return out, nil
}

func (b *Board) lock(ctx context.Context) (func(), error) {
	return filelock.AcquireWithExternalTimeout(ctx, b.lockPath, defaultLockWait)
}

func (b *Board) lockShared(ctx context.Context) (func(), error) {
	bounded, cancel := context.WithTimeout(ctx, defaultLockWait)
	release, err := filelock.AcquireMode(bounded, b.lockPath, filelock.ModeShared)
	cancel()
	if err != nil {
		return nil, err
	}
	return release, nil
}

func nodeState(st *State, node string) NodeState {
	if n := st.Nodes[node]; n != nil {
		return n.State
	}
	return ""
}
