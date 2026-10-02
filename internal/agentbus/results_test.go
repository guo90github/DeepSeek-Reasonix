package agentbus

import (
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func TestWriteAndReadResult(t *testing.T) {
	dir := t.TempDir()
	envelope := Result{
		Correlation: "c1",
		TaskID:      "design",
		From:        "bob",
		Status:      ResultAnswered,
		Text:        "spec landed",
		Evidence:    []board.Evidence{{Kind: "test", Ref: "go test ./..."}},
		Transcript:  "sessions/bob.jsonl",
		At:          time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := WriteResult(dir, envelope); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, ok, err := ReadResult(dir, "c1")
	if err != nil || !ok {
		t.Fatalf("read = (%v, %v), want a hit", ok, err)
	}
	if got.Text != "spec landed" || got.Status != ResultAnswered || got.TaskID != "design" {
		t.Fatalf("round trip lost fields: %+v", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Ref != "go test ./..." {
		t.Fatalf("evidence lost: %+v", got.Evidence)
	}

	envelope.Text = "revised"
	if err := WriteResult(dir, envelope); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got, _, _ := ReadResult(dir, "c1"); got.Text != "revised" {
		t.Fatalf("rewrite did not replace the envelope: %+v", got)
	}
}

func TestUnknownCorrelationIsAnAnswerNotAnError(t *testing.T) {
	dir := t.TempDir()
	_, ok, err := ReadResult(dir, "nobody-asked")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ok {
		t.Fatal("no envelope exists for that correlation")
	}
}

func TestResultRefusesUnsafeCorrelations(t *testing.T) {
	dir := t.TempDir()
	cases := []string{"", "   ", "../escape", "a/b", `a\b`, "..", strings.Repeat("x", 129)}
	for _, correlation := range cases {
		if err := WriteResult(dir, Result{Correlation: correlation, Status: ResultAnswered}); err == nil {
			t.Fatalf("correlation %q should have been refused", correlation)
		}
		if _, _, err := ReadResult(dir, correlation); err == nil {
			t.Fatalf("reading %q should have been refused", correlation)
		}
	}
}

func TestResultRefusesUnknownStatus(t *testing.T) {
	dir := t.TempDir()
	err := WriteResult(dir, Result{Correlation: "c1", From: "bob", Status: "maybe"})
	if err == nil {
		t.Fatal("an unknown status must be refused, not stored")
	}
}
