package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"reasonix/internal/sessioninbox"
)

// One ending, one name. The wording lives in the store; the desktop keeps a copy
// it cannot import, so this test compares the two verbatim and refuses to let a
// disposition exist without wording.
func TestSettledEndingsShareOneWording(t *testing.T) {
	glossSrc := readRepoFile(t, "desktop/frontend/src/lib/inboxRoomLineExit.ts")
	frontend := parseFrontendGloss(t, glossSrc)

	if len(frontend) != len(sessioninbox.SettledGloss) {
		t.Fatalf("desktop gloss has %d endings, the store declares %d: %v vs %v",
			len(frontend), len(sessioninbox.SettledGloss), frontend, sessioninbox.SettledGloss)
	}
	for word, want := range sessioninbox.SettledGloss {
		got, ok := frontend[word]
		if !ok {
			t.Fatalf("desktop gloss is missing %q (store says %q)", word, want)
		}
		if got != want {
			t.Fatalf("desktop gloss for %q is %q, the store says %q — one ending, one name", word, got, want)
		}
	}

	// The wording must cover exactly the endings this store writes, so adding one
	// without naming it fails here instead of reading as a bare code to a person.
	written := dispositionsWrittenByStore(t, readRepoFile(t, "internal/sessioninbox/ops.go"))
	for _, word := range written {
		if _, ok := sessioninbox.SettledGloss[word]; !ok {
			t.Fatalf("the store writes disposition %q but no wording is declared for it", word)
		}
	}
	if len(written) != len(sessioninbox.SettledGloss) {
		t.Fatalf("the store writes %v but wording is declared for %v", written, keysOf(sessioninbox.SettledGloss))
	}
	if strings.TrimSpace(glossSrc) == "" {
		t.Fatal("empty desktop gloss source")
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}

// parseFrontendGloss pulls `word: "wording",` pairs out of the desktop's map.
func parseFrontendGloss(t *testing.T, src string) map[string]string {
	t.Helper()
	entry := regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*"([^"]+)",\s*$`)
	out := map[string]string{}
	for _, match := range entry.FindAllStringSubmatch(src, -1) {
		out[match[1]] = match[2]
	}
	if len(out) == 0 {
		t.Fatal("no endings found in the desktop gloss")
	}
	return out
}

func dispositionsWrittenByStore(t *testing.T, src string) []string {
	t.Helper()
	written := regexp.MustCompile(`Disposition\("([a-z_]+)"\)`).FindAllStringSubmatch(src, -1)
	seen := map[string]bool{}
	var out []string
	for _, match := range written {
		if !seen[match[1]] {
			seen[match[1]] = true
			out = append(out, match[1])
		}
	}
	if len(out) == 0 {
		t.Fatal("no dispositions found where the store writes them")
	}
	return out
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
