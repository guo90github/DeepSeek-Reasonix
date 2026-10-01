package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

// rereadAfterWrite files what a successful write authored as fresh evidence, so
// a later edit in the same turn owes no read round for content this turn already
// paid for. Only the lines the write itself produced are filed: the whole file
// would attest to content the model never saw.
//
// The window carries no snapshot: it is host-read content that is current by
// construction, and binding it to a source identity would make it answerable
// only while the reader and the writer agree on that identity — a coupling a
// host-side wrapper is free to break.
//
// It runs only for a write the evidence gate already approved, so every filed
// window stays anchored: the model knew the lines that write replaced, and
// these are the lines it put there.
func (a *Agent) rereadAfterWrite(ctx context.Context, source tool.EvidenceTargetInfo) {
	if a == nil || a.task.ledger == nil || a.svc.tools == nil || source.Path == "" || source.Absent || source.PreservesContent {
		return
	}
	reader, ok := a.svc.tools.Get("read_file")
	if !ok {
		return
	}
	if !source.WholeFile {
		written := evidence.WrittenLines(source.LineSpans)
		for _, span := range written[:min(len(written), maxEvidenceReadPages)] {
			a.rereadWindow(ctx, reader, source.Path, span[0]-1, span[1]-span[0]+1)
		}
		return
	}
	offset := 0
	for range maxEvidenceReadPages {
		next, more := a.rereadWindow(ctx, reader, source.Path, offset, readEvidencePageLines)
		if !more {
			return
		}
		offset = next
	}
}

// rereadWindow reads one window and files the lines it delivered, then names
// where the next page starts. A read the host cannot complete leaves the
// evidence exactly as it was.
func (a *Agent) rereadWindow(ctx context.Context, reader tool.Tool, path string, offset, limit int) (int, bool) {
	args := json.RawMessage(fmt.Sprintf(`{"path":%s,"offset":%d,"limit":%d}`, strconv.Quote(path), offset, limit))
	out, err := reader.Execute(ctx, args)
	if err != nil {
		return 0, false
	}
	if !a.fileReadWindow(path, out) {
		return 0, false
	}
	trailer := tool.ParseReadTrailer(out)
	if !trailer.HasMore || trailer.NextOffset <= offset {
		return 0, false
	}
	return trailer.NextOffset, true
}

// fileReadWindow records one reader window as evidence. The window is hashed
// exactly as a model-initiated read hashes it, and it carries no snapshot: it
// is host-read content that is current by construction, so binding it to a
// source identity would make it answerable only while the reader and the
// writer agree on that identity — a coupling a host-side wrapper may break.
func (a *Agent) fileReadWindow(path, out string) bool {
	if a == nil || a.task.ledger == nil || path == "" {
		return false
	}
	window, ok := tool.ParseReadWindow(out)
	if !ok || len(window.Lines) == 0 {
		return false
	}
	hashes := make([]string, len(window.Lines))
	for i, line := range window.Lines {
		hashes[i] = hashLine(line)
	}
	a.task.ledger.RecordTextObservation(evidence.TextObservation{
		Path:       path,
		StartLine:  window.StartLine,
		LineHashes: hashes,
	})
	return true
}
