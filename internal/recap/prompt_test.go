package recap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The built-in rules are the default, and an absent override is not a problem to
// report: it is what every install has.
func TestLoadPromptOverrideWithoutAFileKeepsTheBuiltin(t *testing.T) {
	dir := t.TempDir()
	got := LoadPromptOverride(dir)
	if got.Tag != PromptVersion || got.Text != recapSystemPrompt || got.Note != "" {
		t.Fatalf("no override must mean the built-in rules, silently: %+v", got)
	}
	if empty := LoadPromptOverride(""); empty.Tag != PromptVersion || empty.Note != "" {
		t.Fatalf("no directory must mean the built-in rules: %+v", empty)
	}
}

// A real override has to be used verbatim *and* change the tag, because that tag is
// what makes every record written under the old rules show up as an older version
// instead of a silent difference in quality.
func TestLoadPromptOverrideTagsWhatItSends(t *testing.T) {
	dir := t.TempDir()
	text := "You distill one finished session into notes.\nAnswer with JSON.\n"
	if err := os.WriteFile(filepath.Join(dir, PromptFileName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadPromptOverride(dir)
	if got.Text != strings.TrimSpace(text) {
		t.Fatalf("override text = %q, want the file's contents", got.Text)
	}
	if !strings.HasPrefix(got.Tag, PromptVersion+"+") || len(got.Tag) != len(PromptVersion)+9 {
		t.Fatalf("override tag = %q, want %s+<8 hex>", got.Tag, PromptVersion)
	}
	if got.Note != "" {
		t.Fatalf("a usable override needs no note: %q", got.Note)
	}
	again := LoadPromptOverride(dir)
	if again.Tag != got.Tag {
		t.Fatalf("the tag must be stable for one file: %q vs %q", again.Tag, got.Tag)
	}
}

// A file that cannot be trusted must not become the rules: each of these falls back
// to the built-in prompt and says why, so a broken override costs a note rather than
// the quality of everything the lane writes.
func TestLoadPromptOverrideFallsBackAndSaysWhy(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty file", "   \n\t\n"},
		{"past the size limit", strings.Repeat("x", promptMaxBytes+1)},
	}
	for _, testCase := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, PromptFileName), []byte(testCase.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got := LoadPromptOverride(dir)
		if got.Text != recapSystemPrompt || got.Tag != PromptVersion {
			t.Fatalf("%s: must fall back to the built-in rules: %+v", testCase.name, got)
		}
		if got.Note == "" {
			t.Fatalf("%s: the fallback must say why", testCase.name)
		}
	}
}

// The two button-driven prompts answer to the same override policy as the notes
// prompt: their own file, their own tag, the same fallback with a reason.
func TestButtonPromptsOverrideAndFallBackAlike(t *testing.T) {
	dir := t.TempDir()
	if got := LoadMemoryPromptOverride(dir); got.Text != memoryPrompt || got.Tag != MemoryPromptTag || got.Note != "" {
		t.Fatalf("no memory override must mean the built-in rules: %+v", got)
	}
	if got := LoadSkillPromptOverride(dir); got.Text != skillPrompt || got.Tag != SkillPromptTag {
		t.Fatalf("no skill override must mean the built-in rules: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, MemoryPromptFileName), []byte("Rewrite the note as one sentence.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	memory := LoadMemoryPromptOverride(dir)
	if memory.Text != "Rewrite the note as one sentence." || !strings.HasPrefix(memory.Tag, MemoryPromptTag+"+") {
		t.Fatalf("memory override = %+v, want the file's text under a fingerprinted tag", memory)
	}
	if skill := LoadSkillPromptOverride(dir); skill.Text != skillPrompt {
		t.Fatal("one override must not leak into the other prompt")
	}
	if err := os.WriteFile(filepath.Join(dir, SkillPromptFileName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	broken := LoadSkillPromptOverride(dir)
	if broken.Text != skillPrompt || broken.Tag != SkillPromptTag || broken.Note == "" {
		t.Fatalf("an empty file must fall back with a reason: %+v", broken)
	}
}

// writePromptFile drops one prompt override into a temporary state directory.
func writePromptFile(t *testing.T, dir, name, body string) error {
	t.Helper()
	return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
}
