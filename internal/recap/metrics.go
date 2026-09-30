package recap

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Metrics is what the recap channel produced, in numbers a person can argue with
// instead of adjectives: how many notes, how dense they are, how often one was
// decided, and how often one turned out to hold beyond its own project.
//
// Everything here is derived from the projection, so it costs no model calls and
// can be re-run at any time.
type Metrics struct {
	Records      int
	Notes        int
	ByKind       map[string]int
	AvgBodyRunes int
	// WithPointers is the share of notes that named a place to check. Density is
	// the point of the pointers: a note without one is taken on faith.
	WithPointers int
	Accepted     int
	Rejected     int
	// ObservedBeyond counts notes another bucket reached independently — the only
	// checkable ground for calling one general.
	ObservedBeyond int
	// OldestDays is how long ago the oldest stored recap was generated.
	OldestDays int
}

// Percent renders one share for a report, so a reader never divides by hand.
func (m Metrics) Percent(part, whole int) string {
	if whole <= 0 {
		return "n/a"
	}
	return strconv.Itoa(part*100/whole) + "%"
}

// Summary returns the kinds and their counts in a stable order.
func (m Metrics) Summary() string {
	kinds := make([]string, 0, len(m.ByKind))
	for kind := range m.ByKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, kind+"="+strconv.Itoa(m.ByKind[kind]))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, " ")
}

// MetricsOf reads the whole picture: notes and their shape, what the person
// decided, and which notes more than one project reached.
func (s *Store) MetricsOf(ctx context.Context, now time.Time) (Metrics, error) {
	out := Metrics{ByKind: map[string]int{}}
	if s == nil || s.handle == nil {
		return out, nil
	}
	records, err := s.List(ctx)
	if err != nil {
		return out, err
	}
	recurrences, err := s.Recurrences(ctx)
	if err != nil {
		recurrences = nil
	}
	total := 0
	oldest := time.Time{}
	for _, record := range records {
		out.Records++
		for _, entry := range record.Entries {
			if strings.TrimSpace(entry.Body) == "" {
				continue
			}
			out.Notes++
			out.ByKind[entry.Kind]++
			total += utf8.RuneCountInString(entry.Body)
			if len(entry.Refs) > 0 {
				out.WithPointers++
			}
			if len(ObservationOf(recurrences[HashEntry(entry.Kind, entry.Body)], ProjectOf(record.Path))) > 0 {
				out.ObservedBeyond++
			}
		}
		if record.GeneratedAt.After(oldest) {
			oldest = record.GeneratedAt
		}
	}
	if out.Notes > 0 {
		out.AvgBodyRunes = total / out.Notes
	}
	if !oldest.IsZero() {
		out.OldestDays = int(now.Sub(oldest).Hours() / 24)
	}
	decisions, err := s.Decisions(ctx)
	if err != nil {
		return out, nil
	}
	for _, decision := range decisions {
		switch decision.Choice {
		case DecisionAccept:
			out.Accepted++
		case DecisionReject:
			out.Rejected++
		}
	}
	return out, nil
}
