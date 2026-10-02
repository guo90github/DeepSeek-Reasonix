package agentbus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/fileutil"
)

// Result statuses, closed set.
const (
	ResultAnswered = "answered"
	ResultNoAnswer = "no_answer"
	ResultRefused  = "refused"
)

// Result is the structured terminal state of one dispatch, stored per
// correlation under <board>/results/<correlation>.json. It exists because a
// transcript is not an answer: a sender asks "what did you decide" and must be
// able to read it without parsing prose (AGENT_BUS §7).
type Result struct {
	Correlation string           `json:"correlation"`
	TaskID      string           `json:"taskId,omitempty"`
	From        string           `json:"from"`
	Status      string           `json:"status"`
	Text        string           `json:"text,omitempty"`
	Evidence    []board.Evidence `json:"evidence,omitempty"`
	Transcript  string           `json:"transcriptRef,omitempty"`
	At          time.Time        `json:"at"`
}

// ResultsDir is where one board keeps its reply envelopes.
func ResultsDir(boardDir string) string { return filepath.Join(boardDir, "results") }

// WriteResult publishes one result envelope, replacing any earlier one for the
// same correlation atomically: a reader sees the old envelope or the new one.
func WriteResult(boardDir string, r Result) error {
	r.Correlation = strings.TrimSpace(r.Correlation)
	if err := validateCorrelation(r.Correlation); err != nil {
		return err
	}
	switch r.Status {
	case ResultAnswered, ResultNoAnswer, ResultRefused:
	default:
		return fmt.Errorf("agentbus: unknown result status %q", r.Status)
	}
	dir := ResultsDir(boardDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("agentbus: create results dir: %w", err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("agentbus: encode result: %w", err)
	}
	return fileutil.AtomicWriteFileStrict(filepath.Join(dir, r.Correlation+".json"), raw, 0o600)
}

// ReadResult loads the envelope for one correlation. The bool reports whether
// one exists: an unknown correlation is an answer ("nobody replied"), not an
// error.
func ReadResult(boardDir, correlation string) (Result, bool, error) {
	correlation = strings.TrimSpace(correlation)
	if err := validateCorrelation(correlation); err != nil {
		return Result{}, false, err
	}
	path := filepath.Join(ResultsDir(boardDir), correlation+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, false, nil
		}
		return Result{}, false, fmt.Errorf("agentbus: read result: %w", err)
	}
	var out Result
	if err := json.Unmarshal(raw, &out); err != nil {
		return Result{}, false, fmt.Errorf("agentbus: decode result: %w", err)
	}
	return out, true, nil
}

// validateCorrelation keeps a correlation id usable as a file name: it becomes
// one, so anything that could climb out of the results directory is refused.
func validateCorrelation(id string) error {
	if id == "" {
		return fmt.Errorf("agentbus: empty correlation")
	}
	if len(id) > 128 {
		return fmt.Errorf("agentbus: correlation too long")
	}
	if strings.ContainsAny(id, `/\:`) || strings.Contains(id, "..") {
		return fmt.Errorf("agentbus: correlation %q is not a safe file name", id)
	}
	return nil
}
