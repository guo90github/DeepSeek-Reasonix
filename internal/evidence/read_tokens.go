package evidence

import (
	"path/filepath"
	"strings"
)

// SeenReadTokens lists read handles for path that the model was already shown,
// most recent first. A handle recorded at or after boundary is excluded: that
// result has not reached the model yet, so it cannot justify a write.
func (l *Ledger) SeenReadTokens(path string, boundary uint64, limit int) []string {
	if l == nil || limit <= 0 {
		return nil
	}
	canonical := filepath.Clean(strings.TrimSpace(path))
	if canonical == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	for i := len(l.receipts) - 1; i >= 0 && len(out) < limit; i-- {
		r := l.receipts[i]
		if r.ID == "" || !r.Read || r.Sequence > boundary || seen[r.ID] {
			continue
		}
		if !receiptNamesPath(r.Paths, canonical) || !observationCarriesToken(l.observations, r.ID, boundary) {
			continue
		}
		seen[r.ID] = true
		out = append(out, r.ID)
	}
	return out
}

func receiptNamesPath(paths []string, canonical string) bool {
	for _, path := range paths {
		if filepath.Clean(strings.TrimSpace(path)) == canonical {
			return true
		}
	}
	return false
}

func observationCarriesToken(observations []TextObservation, token string, boundary uint64) bool {
	for _, o := range observations {
		if o.Token == token && !o.Absent && o.Sequence <= boundary {
			return true
		}
	}
	return false
}
