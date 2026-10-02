package agentbus

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"reasonix/internal/agentbus/jsonl"
	"reasonix/internal/filelock"
)

const (
	talkLogName  = "messages.jsonl"
	talkLockName = "messages.jsonl.lock"
	talkLockWait = 5 * time.Second
)

// TalkLog is the durable talk surface of one board: an append-only
// messages.jsonl beside the op log, written under its own file lock so two
// sessions talking at once cannot lose a line or share a seq.
type TalkLog struct {
	boardDir string
}

// OpenTalkLog addresses the talk log stored alongside a board.
func OpenTalkLog(boardDir string) (*TalkLog, error) {
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
	return &TalkLog{boardDir: abs}, nil
}

// Dir is the board directory this talk log belongs to.
func (l *TalkLog) Dir() string { return l.boardDir }

// Path is the file the lines land in.
func (l *TalkLog) Path() string { return filepath.Join(l.boardDir, talkLogName) }

// Append validates one line against the folded surface and publishes it with the
// next seq. A refusal is typed and changes nothing; the seq is assigned while the
// file lock is held, so two processes never mint the same one.
func (l *TalkLog) Append(ctx context.Context, line TalkLine, limits TalkLimits) (TalkLine, error) {
	if line.At.IsZero() {
		line.At = time.Now().UTC()
	}
	release, err := filelock.AcquireWithExternalTimeout(ctx, filepath.Join(l.boardDir, talkLockName), talkLockWait)
	if err != nil {
		return TalkLine{}, err
	}
	defer release()

	state, _, err := l.read()
	if err != nil {
		return TalkLine{}, err
	}
	line.Seq = state.nextSeq()
	if err := ApplyTalk(state, line, limits); err != nil {
		return TalkLine{}, err
	}
	if err := jsonl.Append(l.Path(), line); err != nil {
		return TalkLine{}, err
	}
	return line, nil
}

// CloseLapsed publishes the silence closure for a topic that has gone quiet, the
// way a sweep reclaims an expired lease: the decision is recorded, not re-derived
// at read time. It reports whether a closure was written.
func (l *TalkLog) CloseLapsed(ctx context.Context, topic string, now time.Time, limits TalkLimits) (bool, error) {
	release, err := filelock.AcquireWithExternalTimeout(ctx, filepath.Join(l.boardDir, talkLockName), talkLockWait)
	if err != nil {
		return false, err
	}
	defer release()
	state, _, err := l.read()
	if err != nil {
		return false, err
	}
	if !TopicLapsed(state, topic, now, limits) {
		return false, nil
	}
	line, ok := CloseLine(state, topic, now)
	if !ok {
		return false, nil
	}
	line.Seq = state.nextSeq()
	if err := ApplyTalk(state, line, limits); err != nil {
		return false, err
	}
	if err := jsonl.Append(l.Path(), line); err != nil {
		return false, err
	}
	return true, nil
}

// Read folds the whole log. The counters keep a damaged file honest.
func (l *TalkLog) Read() (*TalkState, jsonl.Read[TalkLine], error) {
	return l.read()
}

func (l *TalkLog) read() (*TalkState, jsonl.Read[TalkLine], error) {
	read, err := jsonl.ReadAll[TalkLine](l.Path())
	if err != nil {
		return NewTalkState(), read, err
	}
	return FoldTalk(read.Items), read, nil
}

func (st *TalkState) nextSeq() uint64 {
	var max uint64
	for _, topic := range st.Topics {
		for _, line := range topic.Lines {
			if line.Seq > max {
				max = line.Seq
			}
		}
	}
	return max + 1
}

// ResultsDir is where this board's reply envelopes live.
func (l *TalkLog) ResultsDir() string { return ResultsDir(l.boardDir) }
