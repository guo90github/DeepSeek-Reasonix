package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitAuditChunkNeverCutsInsideARune(t *testing.T) {
	text := strings.Repeat("思", 5)
	first, rest := splitAuditChunk(text, 7)
	if !utf8.ValidString(first) || !utf8.ValidString(rest) {
		t.Fatalf("split produced invalid UTF-8: %q / %q", first, rest)
	}
	if first+rest != text {
		t.Fatalf("split lost text: %q + %q != %q", first, rest, text)
	}
	if utf8.RuneCountInString(first) != 2 {
		t.Fatalf("first chunk = %q, want two runes", first)
	}
}

func TestSplitAuditChunkPassesShortTextThrough(t *testing.T) {
	first, rest := splitAuditChunk("abc", 8)
	if first != "abc" || rest != "" {
		t.Fatalf("split = %q / %q, want the whole text and nothing left", first, rest)
	}
}
