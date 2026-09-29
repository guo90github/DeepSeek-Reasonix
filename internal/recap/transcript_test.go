package recap

import (
	"strings"
	"testing"
)

func TestDefaultPathLivesUnderTheCacheRoot(t *testing.T) {
	path := DefaultPath()
	if path == "" {
		t.Skip("no cache root on this machine")
	}
	if !strings.Contains(path, "session-recap") || !strings.HasSuffix(path, ".sqlite") {
		t.Fatalf("unexpected projection path %q", path)
	}
}

func TestClipRunes(t *testing.T) {
	if got := clipRunes("short", 10); got != "short" {
		t.Fatalf("clipRunes changed a short value: %q", got)
	}
	got := clipRunes(strings.Repeat("a", 20), 5)
	if got != "aaaaa…" {
		t.Fatalf("clipRunes = %q", got)
	}
}
