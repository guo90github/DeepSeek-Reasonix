package jsonl

import (
	"os"
	"path/filepath"
	"testing"
)

type record struct {
	Seq  uint64 `json:"seq"`
	Text string `json:"text"`
}

func TestAppendAndReadAllRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	for i := 1; i <= 3; i++ {
		if err := Append(path, record{Seq: uint64(i), Text: "x"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	read, err := ReadAll[record](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Items) != 3 || read.Skipped != 0 || read.Truncated != 0 {
		t.Fatalf("read = %+v, want three clean records", read)
	}
	if read.Items[2].Seq != 3 {
		t.Fatalf("items = %+v", read.Items)
	}
}

func TestReadAllCountsWhatItLoses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	body := `{"seq":1,"text":"ok"}` + "\n" +
		`{"seq":` + "\n" + // corrupt but terminated
		"\n" + // blank
		`{"seq":4,"text":"torn"` // torn tail
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	read, err := ReadAll[record](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Items) != 1 {
		t.Fatalf("items = %+v, want only the intact record", read.Items)
	}
	if read.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2 (corrupt line and blank line)", read.Skipped)
	}
	if read.Truncated != 1 {
		t.Fatalf("truncated = %d, want 1 (the torn tail)", read.Truncated)
	}
}

func TestRepairTornTailDropsOnlyTheLastPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	if err := Append(path, record{Seq: 1, Text: "kept"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"seq":1,"text":"kept"}`+"\n"+`{"seq":2,"te`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, err := RepairTornTail(path)
	if err != nil || !changed {
		t.Fatalf("repair = (%v, %v), want a change", changed, err)
	}
	changed, err = RepairTornTail(path)
	if err != nil || changed {
		t.Fatalf("second repair = (%v, %v), want a no-op", changed, err)
	}

	if err := Append(path, record{Seq: 2, Text: "after"}); err != nil {
		t.Fatalf("append after repair: %v", err)
	}
	read, err := ReadAll[record](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Items) != 2 || read.Items[1].Text != "after" {
		t.Fatalf("read = %+v, want the repaired log plus the new record", read)
	}
}

func TestReadAllOfAMissingLogIsEmpty(t *testing.T) {
	read, err := ReadAll[record](filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Items) != 0 || read.Skipped != 0 || read.Truncated != 0 {
		t.Fatalf("read = %+v, want an empty log", read)
	}
}

func TestEnsureDirRefusesASymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := EnsureDir(link); err == nil {
		t.Fatal("a symlinked log directory must be refused")
	}
	if err := EnsureDir(filepath.Join(root, "fresh")); err != nil {
		t.Fatalf("fresh dir: %v", err)
	}
}

// ReadFrom is how a reader skips a prefix it has already folded, so it must apply exactly the
// rules ReadAll does to whatever it is handed from the offset onwards.
func TestReadFromDecodesOnlyWhatComesAfterTheOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	first := `{"seq":1,"text":"prefix"}` + "\n"
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatalf("write prefix: %v", err)
	}
	for i := 2; i <= 4; i++ {
		if err := Append(path, record{Seq: uint64(i), Text: "tail"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	read, err := ReadFrom[record](path, int64(len(first)))
	if err != nil {
		t.Fatalf("read from: %v", err)
	}
	if len(read.Items) != 3 || read.Items[0].Seq != 2 || read.Items[2].Seq != 4 {
		t.Fatalf("read = %+v, want the three records after the offset", read.Items)
	}
	if read.Skipped != 0 || read.Truncated != 0 {
		t.Fatalf("read = %+v, want nothing lost", read)
	}
	if _, err := ReadFrom[record](path, -1); err == nil {
		t.Fatal("a negative offset must be refused")
	}
}

func TestReadFromCountsWhatTheTailGivesUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	prefix := `{"seq":1,"text":"prefix"}` + "\n"
	body := prefix + `{"seq":2,"text":"ok"}` + "\n" + `{"seq":` + "\n" + `{"seq":4,"te`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	read, err := ReadFrom[record](path, int64(len(prefix)))
	if err != nil {
		t.Fatalf("read from: %v", err)
	}
	if len(read.Items) != 1 || read.Items[0].Seq != 2 {
		t.Fatalf("items = %+v, want only the intact tail record", read.Items)
	}
	if read.Skipped != 1 || read.Truncated != 1 {
		t.Fatalf("read = %+v, want one corrupt line and one torn tail counted", read)
	}
}
