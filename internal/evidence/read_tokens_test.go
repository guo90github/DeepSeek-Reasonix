package evidence

import "testing"

func TestSeenReadTokensOnlyOffersWhatTheModelWasShown(t *testing.T) {
	ledger := NewLedger()
	read := ledger.Record(Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{"/w/a.go"}})
	ledger.RecordTextObservation(TextObservation{Path: "/w/a.go", StartLine: 1, Token: read.ID, LineHashes: []string{"h1"}})
	other := ledger.Record(Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{"/w/b.go"}})
	ledger.RecordTextObservation(TextObservation{Path: "/w/b.go", StartLine: 1, Token: other.ID, LineHashes: []string{"h1"}})

	boundary := ledger.ObservationBoundary()
	if got := ledger.SeenReadTokens("/w/a.go", boundary, 4); len(got) != 1 || got[0] != read.ID {
		t.Fatalf("tokens = %v, want just the read of a.go (%q)", got, read.ID)
	}

	// A read issued in the current batch has not reached the model yet.
	fresh := ledger.Record(Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{"/w/a.go"}})
	ledger.RecordTextObservation(TextObservation{Path: "/w/a.go", StartLine: 1, Token: fresh.ID, LineHashes: []string{"h1"}})
	if got := ledger.SeenReadTokens("/w/a.go", boundary, 4); len(got) != 1 || got[0] != read.ID {
		t.Fatalf("tokens = %v, want the boundary to hide %q", got, fresh.ID)
	}

	// A write receipt carries no window, so it can never be cited as a read.
	write := ledger.Record(Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"/w/a.go"}})
	if got := ledger.SeenReadTokens("/w/a.go", ledger.ObservationBoundary(), 8); len(got) != 2 {
		t.Fatalf("tokens = %v, want the two reads only (not %q)", got, write.ID)
	}
}
