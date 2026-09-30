package stats

import (
	"testing"
	"time"
)

func TestQueryPurposesSplitsByUsageSource(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)
	day := dayStart(time.Now())

	w.Append(record{Timestamp: day.Add(1 * time.Hour), ModelRef: "m1", Source: "desktop", UsageSource: "executor", Total: 100})
	w.Append(record{Timestamp: day.Add(2 * time.Hour), ModelRef: "m1", Source: "desktop", UsageSource: "session-recap", Total: 60})
	w.Append(record{Timestamp: day.Add(3 * time.Hour), ModelRef: "m1", Source: "desktop", UsageSource: "title", Total: 40})
	w.Append(record{Timestamp: day.Add(4 * time.Hour), Source: "desktop", Turn: true})
	w.Append(record{Timestamp: day.Add(5 * time.Hour), ModelRef: "m2", Source: "cli", UsageSource: "session-recap", Total: 10})

	got, err := w.QueryPurposes(SourceFilter{From: day, To: day})
	if err != nil {
		t.Fatalf("query purposes: %v", err)
	}
	counts := map[string]int64{}
	for _, p := range got {
		counts[p.Purpose] = p.Tokens
	}
	if len(got) != 3 || counts["executor"] != 100 || counts["session-recap"] != 70 || counts["title"] != 40 {
		t.Fatalf("purposes = %+v", got)
	}
	if got[0].Purpose != "executor" {
		t.Fatalf("largest purpose first: %+v", got)
	}

	// The entry-point filter still applies, and shares are over the filtered total.
	filtered, err := w.QueryPurposes(SourceFilter{From: day, To: day, Source: "cli"})
	if err != nil {
		t.Fatalf("query purposes (cli): %v", err)
	}
	if len(filtered) != 1 || filtered[0].Purpose != "session-recap" || filtered[0].Tokens != 10 {
		t.Fatalf("filtered purposes = %+v", filtered)
	}
	if filtered[0].Percent < 99 || filtered[0].Percent > 101 {
		t.Fatalf("a single purpose's share should be 100%%, got %v", filtered[0].Percent)
	}
}
