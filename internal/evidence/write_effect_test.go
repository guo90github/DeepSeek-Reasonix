package evidence

import (
	"slices"
	"testing"
)

func sameWindow(a, b TextObservation) bool {
	return a.Path == b.Path && a.StartLine == b.StartLine && a.Snapshot == b.Snapshot &&
		a.Token == b.Token && slices.Equal(a.LineHashes, b.LineHashes)
}

func observedWindow(startLine int, hashes ...string) TextObservation {
	return TextObservation{
		Path:       "/w/a.go",
		StartLine:  startLine,
		Snapshot:   "snap-1",
		Version:    "v1",
		Token:      "r_1",
		LineHashes: hashes,
	}
}

func TestRebaseTextObservationKeepsTheLinesAWriteLeftAlone(t *testing.T) {
	o := observedWindow(1, "a", "b", "c", "d", "e")

	got := RebaseTextObservation(o, []WriteLineSpan{{FirstLine: 3, LastLine: 3, Delta: 0}})

	want := []TextObservation{
		{Path: "/w/a.go", StartLine: 1, LineHashes: []string{"a", "b"}},
		{Path: "/w/a.go", StartLine: 4, LineHashes: []string{"d", "e"}},
	}
	if !slices.EqualFunc(got, want, sameWindow) {
		t.Fatalf("rebased = %+v, want %+v", got, want)
	}
}

func TestRebaseTextObservationShiftsLinesAfterAGrowingHunk(t *testing.T) {
	o := observedWindow(5, "a", "b", "c")

	got := RebaseTextObservation(o, []WriteLineSpan{{FirstLine: 1, LastLine: 1, Delta: 2}})

	want := []TextObservation{{Path: "/w/a.go", StartLine: 7, LineHashes: []string{"a", "b", "c"}}}
	if !slices.EqualFunc(got, want, sameWindow) {
		t.Fatalf("rebased = %+v, want %+v", got, want)
	}
}

func TestRebaseTextObservationDropsAWindowTheWriteReplaced(t *testing.T) {
	o := observedWindow(1, "a", "b")

	if got := RebaseTextObservation(o, []WriteLineSpan{{FirstLine: 1, LastLine: 2, Delta: 0}}); len(got) != 0 {
		t.Fatalf("rebased = %+v, want nothing", got)
	}
}

func TestRebaseTextObservationWithoutSpansIsTheOriginalWindow(t *testing.T) {
	o := observedWindow(1, "a")

	got := RebaseTextObservation(o, nil)
	if !slices.EqualFunc(got, []TextObservation{o}, sameWindow) {
		t.Fatalf("rebased = %+v, want the original window", got)
	}
}

func TestLedgerRebaseReanchorsWindowsPastTheWriteThatInvalidatedThem(t *testing.T) {
	l := NewLedger()
	l.RecordTextObservation(observedWindow(1, "a", "b", "c"))

	l.RebaseObservations("/w/a.go", []WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: 1}})

	want := []TextObservation{
		{Path: "/w/a.go", StartLine: 1, LineHashes: []string{"a"}},
		{Path: "/w/a.go", StartLine: 4, LineHashes: []string{"c"}},
	}
	if got := l.TextObservations(); !slices.EqualFunc(got, want, sameWindow) {
		t.Fatalf("observations = %+v, want %+v", got, want)
	}
	for _, o := range l.TextObservations() {
		if o.Sequence <= 1 {
			t.Fatalf("rebase must stamp its windows after the write, got sequence %d", o.Sequence)
		}
	}
}

func TestLedgerRetireDropsWindowsAndLeavesOtherFiles(t *testing.T) {
	l := NewLedger()
	l.RecordTextObservation(observedWindow(1, "a"))
	other := observedWindow(1, "z")
	other.Path = "/w/b.go"
	l.RecordTextObservation(other)

	l.RetireObservations("/w/a.go")

	got := l.TextObservations()
	if len(got) != 1 || got[0].Path != "/w/b.go" {
		t.Fatalf("observations = %+v, want only /w/b.go", got)
	}
}
