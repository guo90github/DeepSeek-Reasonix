package agentbus

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/agentbus/jsonl"
	"reasonix/internal/filelock"
)

const (
	hearingLogName  = "hearings.jsonl"
	hearingLockName = "hearings.jsonl.lock"
	hearingLockWait = 5 * time.Second
)

// HearingLog is the durable deliberation surface of one board: an append-only
// hearings.jsonl written under its own file lock, so two sessions deliberating at
// once cannot share a seq or lose a record.
type HearingLog struct {
	boardDir string
}

// OpenHearingLog addresses the deliberation log stored alongside a board.
func OpenHearingLog(boardDir string) (*HearingLog, error) {
	if boardDir == "" {
		return nil, fmt.Errorf("agentbus: empty board directory")
	}
	abs, err := filepath.Abs(boardDir)
	if err != nil {
		return nil, fmt.Errorf("agentbus: resolve board directory: %w", err)
	}
	if err := jsonl.EnsureDir(abs); err != nil {
		return nil, err
	}
	return &HearingLog{boardDir: abs}, nil
}

// Path is the file the records land in.
func (l *HearingLog) Path() string { return filepath.Join(l.boardDir, hearingLogName) }

// Append validates one record against the folded surface and publishes it with the
// next seq. A refusal is typed and changes nothing.
func (l *HearingLog) Append(ctx context.Context, rec HearingRecord, lim HearingLimits) (HearingRecord, error) {
	if rec.At.IsZero() {
		rec.At = time.Now().UTC()
	}
	release, err := filelock.AcquireWithExternalTimeout(ctx, filepath.Join(l.boardDir, hearingLockName), hearingLockWait)
	if err != nil {
		return HearingRecord{}, err
	}
	defer release()

	state, _, err := l.read()
	if err != nil {
		return HearingRecord{}, err
	}
	rec.Seq = state.nextHearingSeq()
	if err := ApplyHearing(state, rec, lim); err != nil {
		return HearingRecord{}, err
	}
	if err := jsonl.Append(l.Path(), rec); err != nil {
		return HearingRecord{}, err
	}
	return rec, nil
}

// Weigh closes the hearing on a node by the evidence each side brought. The verdict
// is recorded, so the decision is a fact in the log rather than a computation each
// reader repeats.
func (l *HearingLog) Weigh(ctx context.Context, node string, claim, refute []board.Evidence, actor string, lim HearingLimits) (HearingRecord, error) {
	state, _, err := l.read()
	if err != nil {
		return HearingRecord{}, err
	}
	verdict, reason := WeighResponse(claim, refute, state.Escalations, lim.EscalationQuota)
	kind := HearingRule
	if verdict == VerdictEscalate {
		kind = HearingEscalate
	}
	return l.Append(ctx, HearingRecord{
		Node: node, Kind: kind, Actor: actor, Verdict: verdict, Reason: reason,
	}, lim)
}

// Read folds the whole log. The counters keep a damaged file honest.
func (l *HearingLog) Read() (*HearingState, jsonl.Read[HearingRecord], error) {
	return l.read()
}

func (l *HearingLog) read() (*HearingState, jsonl.Read[HearingRecord], error) {
	read, err := jsonl.ReadAll[HearingRecord](l.Path())
	if err != nil {
		return NewHearingState(), read, err
	}
	return FoldHearings(read.Items), read, nil
}

func (st *HearingState) nextHearingSeq() uint64 {
	var max uint64
	for _, hearing := range st.Hearings {
		for _, rec := range hearing.Records {
			if rec.Seq > max {
				max = rec.Seq
			}
		}
	}
	return max + 1
}
