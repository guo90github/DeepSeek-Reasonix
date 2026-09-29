package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

const recapTestAnswer = "Goal: wire the batch command\n" +
	"Actions: enumerated the roots and called the lane\n" +
	"Conclusion: recaps stored\n" +
	"Follow-ups: none\n"

type recapTestProvider struct {
	calls int
}

func (p *recapTestProvider) Name() string { return "fake" }

func (p *recapTestProvider) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	p.calls++
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: recapTestAnswer}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

type recapTestModels struct {
	prov  *recapTestProvider
	paths []string
}

func (m *recapTestModels) Resolve(_ context.Context, sessionPath string) (provider.Provider, string, bool) {
	m.paths = append(m.paths, sessionPath)
	return m.prov, "fake/model", true
}

func writeRecapSession(t *testing.T, dir, name string) string {
	t.Helper()
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "please do the thing"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "done: the thing is done"})
	path := filepath.Join(dir, name)
	if err := session.Save(path); err != nil {
		t.Fatalf("save session: %v", err)
	}
	return path
}

func installRecapTestSeams(t *testing.T, models recap.ModelResolver, dbPath string) {
	t.Helper()
	prevStore := openSessionRecapStore
	prevModels := newSessionRecapModelResolver
	openSessionRecapStore = func(ctx context.Context) (*recap.Store, error) {
		return recap.Open(ctx, recap.Options{Path: dbPath})
	}
	newSessionRecapModelResolver = func() (recap.ModelResolver, error) { return models, nil }
	t.Cleanup(func() {
		openSessionRecapStore = prevStore
		newSessionRecapModelResolver = prevModels
	})
}

// runRecapCommandJSON captures the --json report the command prints to stdout.
func runRecapCommandJSON(t *testing.T, args ...string) (int, sessionRecapReport) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := reindexSessionRecap(append(args, "--json"))
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	_ = r.Close()
	var report sessionRecapReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report %q: %v", data, err)
	}
	return code, report
}

func TestSessionRecapBackfillSkipsInadmissibleAndResumes(t *testing.T) {
	dir := t.TempDir()
	prov := &recapTestProvider{}
	models := &recapTestModels{prov: prov}
	installRecapTestSeams(t, models, filepath.Join(t.TempDir(), "recap.db"))

	first := writeRecapSession(t, dir, "20260101-000000.000000000-one.jsonl")
	second := writeRecapSession(t, dir, "20260102-000000.000000000-two.jsonl")
	recovery := filepath.Join(dir, "20260103-000000.000000000-base-recovery-0123456789abcdef.jsonl")
	if err := os.WriteFile(recovery, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, report := runRecapCommandJSON(t, "--dir", dir)
	if code != 0 {
		t.Fatalf("first run exit code = %d, want 0", code)
	}
	if report.Generated != 2 || report.Failed != 0 {
		t.Fatalf("first run = %+v, want generated=2 failed=0", report)
	}
	// The recovery copy is inadmissible (Admissible), so it is skipped, not failed.
	if report.Skipped != 1 {
		t.Fatalf("first run skipped = %d, want 1 (the recovery copy)", report.Skipped)
	}
	// c1: only the two admissible sessions reached the model.
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	resolved := map[string]bool{}
	for _, path := range models.paths {
		if strings.Contains(filepath.Base(path), "-recovery-") {
			t.Fatalf("a recovery copy reached the model: %s", path)
		}
		resolved[path] = true
	}
	if len(models.paths) != 2 || !resolved[first] || !resolved[second] {
		t.Fatalf("admissible sessions not resolved as expected: %v", models.paths)
	}

	// c2: a second run leaves the unchanged sessions alone and does not re-call the model.
	code, report = runRecapCommandJSON(t, "--dir", dir)
	if code != 0 {
		t.Fatalf("second run exit code = %d, want 0", code)
	}
	if report.Generated != 0 {
		t.Fatalf("second run regenerated %d recaps, want 0", report.Generated)
	}
	if report.Skipped < 2 {
		t.Fatalf("second run skipped = %d, want >= 2 (the now-current sessions)", report.Skipped)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls after the rerun = %d, want 2", prov.calls)
	}
}

func TestSessionRecapJSONReportHasThreeStates(t *testing.T) {
	dir := t.TempDir()
	models := &recapTestModels{prov: &recapTestProvider{}}
	installRecapTestSeams(t, models, filepath.Join(t.TempDir(), "recap.db"))
	writeRecapSession(t, dir, "20260101-000000.000000000-one.jsonl")

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := reindexSessionRecap([]string{"--dir", dir, "--json"})
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	_ = r.Close()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// c3: the JSON report carries the three-state counts.
	var fields map[string]int
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("decode report %q: %v", data, err)
	}
	for _, key := range []string{"generated", "skipped", "failed"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("report %q is missing the %q count", data, key)
		}
	}
	if fields["generated"] != 1 {
		t.Fatalf("generated = %d, want 1", fields["generated"])
	}
}
