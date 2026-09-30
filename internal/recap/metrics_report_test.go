package recap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Reporting what the recap channel actually produced needs a live projection; like
// the offer probe it reads a copy, asserts nothing, and prints the numbers, because
// the numbers are the result. It costs no model calls.
//
//	RECAP_CALIB_DB=<copy of session-recap/v1.sqlite> \
//	  go test ./internal/recap/ -run ReportRecapMetrics -v -count=1
func TestReportRecapMetrics(t *testing.T) {
	db := strings.TrimSpace(os.Getenv("RECAP_CALIB_DB"))
	if db == "" {
		t.Skip("set RECAP_CALIB_DB (a copy of the projection) to report metrics")
	}
	ctx := context.Background()
	store, err := Open(ctx, Options{Path: db})
	if err != nil {
		t.Fatalf("open the projection: %v", err)
	}
	defer func() { _ = store.Close() }()
	metrics, err := store.MetricsOf(ctx, time.Now())
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	t.Logf("stored: records=%d notes=%d  kinds=%s  avg body=%d runes  oldest=%d days",
		metrics.Records, metrics.Notes, metrics.Summary(), metrics.AvgBodyRunes, metrics.OldestDays)
	t.Logf("density: notes naming a place to check = %d (%s)",
		metrics.WithPointers, metrics.Percent(metrics.WithPointers, metrics.Notes))
	t.Logf("decided: accepted=%d (%s of notes)  dropped=%d (%s)",
		metrics.Accepted, metrics.Percent(metrics.Accepted, metrics.Notes),
		metrics.Rejected, metrics.Percent(metrics.Rejected, metrics.Notes))
	t.Logf("held beyond their own project: %d (%s)",
		metrics.ObservedBeyond, metrics.Percent(metrics.ObservedBeyond, metrics.Notes))
}
