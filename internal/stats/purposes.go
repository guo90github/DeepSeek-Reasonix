package stats

import (
	"sort"
	"strings"
)

// PurposeUsage is one usage_source bucket within a range. usage_source labels
// what a request was for (executor, title, compaction, session-recap, ...),
// which is a different axis from Record.Source: the entry point that issued it.
type PurposeUsage struct {
	Purpose string  `json:"purpose"`
	Tokens  int64   `json:"tokens"`
	Percent float64 `json:"percent"` // 0..100
}

// QueryPurposes aggregates a range by usage_source. The rollup catalog does not
// carry that column, so this always reads the daily stats files; they hold one
// line per provider request, and the settings panel is the only caller.
func (w *Writer) QueryPurposes(f SourceFilter) ([]PurposeUsage, error) {
	if w == nil || w.dir == "" {
		return []PurposeUsage{}, nil
	}
	recordsByDay, err := readDailyRange(w.dir, daysInRange(f.From, f.To))
	if err != nil {
		return nil, err
	}
	totals := map[string]int64{}
	var total int64
	for _, recs := range recordsByDay {
		for _, rec := range recs {
			if rec.Turn || rec.Total <= 0 || !matchesSource(rec.Source, f.Source) {
				continue
			}
			purpose := strings.TrimSpace(rec.UsageSource)
			if purpose == "" {
				purpose = "(unknown)"
			}
			t := int64(rec.Total)
			totals[purpose] += t
			total += t
		}
	}
	out := purposesSorted(totals)
	if total > 0 {
		for i := range out {
			out[i].Percent = float64(out[i].Tokens) / float64(total) * 100
		}
	}
	return out, nil
}

func purposesSorted(totals map[string]int64) []PurposeUsage {
	out := make([]PurposeUsage, 0, len(totals))
	for purpose, t := range totals {
		out = append(out, PurposeUsage{Purpose: purpose, Tokens: t})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tokens == out[j].Tokens {
			return out[i].Purpose < out[j].Purpose
		}
		return out[i].Tokens > out[j].Tokens
	})
	return out
}
