package recap

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
)

func TestAdmissible(t *testing.T) {
	dir := t.TempDir()
	normal := filepath.Join(dir, "20260101-000000.000000000-model.jsonl")
	if err := os.WriteFile(normal, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		path     string
		removing Remover
		want     bool
	}{
		{name: "empty path", path: "", want: false},
		{name: "recovery copy", path: filepath.Join(dir, "20260101-000000.000000000-model-recovery-0123456789abcdef.jsonl"), want: false},
		{name: "normal", path: normal, want: true},
		{name: "mid removal", path: normal, removing: func(string) bool { return true }, want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := Admissible(tt.path, tt.removing); got != tt.want {
				t.Fatalf("Admissible(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestAdmissibleRejectsCleanupPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260101-000000.000000000-model.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Admissible(path, nil) {
		t.Fatal("a fresh session must be admissible")
	}
	if err := agent.MarkCleanupPending(path, "delete"); err != nil {
		t.Fatal(err)
	}
	if Admissible(path, nil) {
		t.Fatal("a session pending cleanup must not be admissible")
	}
}

func TestFingerprintNeedsTranscript(t *testing.T) {
	if _, err := Fingerprint(""); err == nil {
		t.Fatal("empty path must not produce a fingerprint")
	}
	if _, err := Fingerprint(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("missing transcript must not produce a fingerprint")
	}
}

func TestFingerprintTracksContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260101-000000.000000000-model.jsonl")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("a much longer body"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("rewriting the transcript must change its fingerprint")
	}
}
