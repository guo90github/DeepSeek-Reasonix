package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/history"
	"reasonix/internal/recap"
)

func installHistoryTestSeams(t *testing.T, sessionDir, recapPath string) {
	t.Helper()
	prevOptions := historySearchOptions
	prevStore := openHistoryRecapStore
	historySearchOptions = func() history.Options {
		return history.Options{SessionDir: sessionDir}
	}
	openHistoryRecapStore = func(ctx context.Context) (*recap.Store, error) {
		return recap.Open(ctx, recap.Options{Path: recapPath})
	}
	t.Cleanup(func() {
		historySearchOptions = prevOptions
		openHistoryRecapStore = prevStore
	})
}

func writeHistoryRecap(t *testing.T, recapPath, sessionPath string) {
	t.Helper()
	store, err := recap.Open(context.Background(), recap.Options{Path: recapPath})
	if err != nil {
		t.Fatalf("open recap store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Put(context.Background(), recap.Record{
		Path: sessionPath, Goal: "wire the user entry", Actions: "registered history",
		Conclusion: "hits show recaps", FollowUps: "none", Model: "fake/model",
		PromptVersion: recap.PromptVersion, GeneratedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put recap: %v", err)
	}
}

// runHistoryCommand captures what the command writes to stdout.
func runHistoryCommand(t *testing.T, args ...string) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := historyCommand(args)
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	_ = r.Close()
	return code, string(data)
}

func TestHistoryCommandShowsHitsWithAndWithoutRecap(t *testing.T) {
	sessionDir := t.TempDir()
	withRecap := writeRecapSession(t, sessionDir, "20260101-000000.000000000-alpha.jsonl")
	withoutRecap := writeRecapSession(t, sessionDir, "20260102-000000.000000000-beta.jsonl")
	recapPath := filepath.Join(t.TempDir(), "recap.db")
	writeHistoryRecap(t, recapPath, withRecap)
	installHistoryTestSeams(t, sessionDir, recapPath)

	code, out := runHistoryCommand(t, "thing")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; out=%s", code, out)
	}
	// c1: the recapped session carries its recap elements.
	if !strings.Contains(out, "wire the user entry") || !strings.Contains(out, "hits show recaps") {
		t.Fatalf("recap missing from output: %s", out)
	}
	// c2: both matched sessions appear, the recap-less one with the marker.
	if !strings.Contains(out, withRecap) || !strings.Contains(out, withoutRecap) {
		t.Fatalf("a matched session is missing from output: %s", out)
	}
	if !strings.Contains(out, "（尚无回顾）") {
		t.Fatalf("missing the no-recap marker: %s", out)
	}
}

func TestHistoryCommandReportsNoMatches(t *testing.T) {
	sessionDir := t.TempDir()
	writeRecapSession(t, sessionDir, "20260101-000000.000000000-alpha.jsonl")
	installHistoryTestSeams(t, sessionDir, filepath.Join(t.TempDir(), "recap.db"))

	code, out := runHistoryCommand(t, "zzz-nonexistent-token-zzz")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// c3: an empty result is stated, not padded with weak matches.
	if !strings.Contains(out, "无相关记录") {
		t.Fatalf("want the no-match notice, got: %s", out)
	}
}

func TestHistoryCommandJSONCarriesHitsAndRecap(t *testing.T) {
	sessionDir := t.TempDir()
	withRecap := writeRecapSession(t, sessionDir, "20260101-000000.000000000-alpha.jsonl")
	writeRecapSession(t, sessionDir, "20260102-000000.000000000-beta.jsonl")
	recapPath := filepath.Join(t.TempDir(), "recap.db")
	writeHistoryRecap(t, recapPath, withRecap)
	installHistoryTestSeams(t, sessionDir, recapPath)

	code, out := runHistoryCommand(t, "thing", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// c4: the JSON report carries the query, per-session hits, and embedded recap.
	var report historyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report %q: %v", out, err)
	}
	if report.Query != "thing" {
		t.Fatalf("query = %q, want %q", report.Query, "thing")
	}
	if len(report.Hits) != 2 {
		t.Fatalf("hits = %d, want 2 (one per matched session)", len(report.Hits))
	}
	var recapped *historyHitView
	for i := range report.Hits {
		if report.Hits[i].SessionPath == withRecap {
			recapped = &report.Hits[i]
		}
	}
	if recapped == nil || recapped.Recap == nil || recapped.Recap.Goal != "wire the user entry" {
		t.Fatalf("recap not embedded for the recapped session: %+v", report.Hits)
	}
}
