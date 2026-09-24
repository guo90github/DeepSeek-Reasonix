package agent

import "reasonix/internal/evidence"

// rebaseWriteObservations keeps the evidence a write left untouched. The write
// replaced specific lines: only those stop being evidence, so an edit
// elsewhere in the same file no longer owes a re-read.
//
// A writer whose line accounting the host cannot express retires the file's
// windows instead. Shifting an older version's lines as if they described the
// current one would manufacture evidence the model never had.
func (a *Agent) rebaseWriteObservations(plan *toolCallPlan, rec evidence.Receipt, err error) {
	if a == nil || plan == nil || a.task.ledger == nil || err != nil || !rec.Success || !rec.Write {
		return
	}
	if source := plan.expectedWriteSource; source.Path != "" && len(source.LineSpans) > 0 {
		a.task.ledger.RebaseObservations(source.Path, source.LineSpans)
		return
	}
	for _, path := range rec.Paths {
		a.task.ledger.RetireObservations(path)
	}
}
