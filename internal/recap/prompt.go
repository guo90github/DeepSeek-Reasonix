package recap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptSource is the rule text a recap is generated from, plus the tag every
// record stores. A prompt change has to show up as an older version rather than as
// a silent difference in what the notes mean, which is why the tag travels with the
// record instead of being a compile-time constant only.
type PromptSource struct {
	Text string
	Tag  string
	// Note says why the built-in rules are in use when a file could not be used.
	Note string
}

const (
	// PromptFileName is the override a person can drop into the state directory.
	PromptFileName = "recap-prompt.md"
	// promptMaxBytes keeps a mistaken paste from becoming the rules: a prompt the
	// lane cannot afford to send is worse than the one it ships with.
	promptMaxBytes = 32 * 1024
)

// BuiltinPrompt is the rules compiled into this binary.
func BuiltinPrompt() PromptSource {
	return PromptSource{Text: recapSystemPrompt, Tag: PromptVersion}
}

// LoadPromptOverride reads dir/PromptFileName. Every failure falls back to the
// built-in rules and says why in Note: half a prompt would silently lower the
// quality of everything the lane writes, and the note is what makes that visible.
func LoadPromptOverride(dir string) PromptSource {
	builtin := BuiltinPrompt()
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return builtin
	}
	path := filepath.Join(dir, PromptFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return builtin
		}
		builtin.Note = "could not read " + path + ": " + err.Error()
		return builtin
	}
	text := strings.TrimSpace(string(raw))
	switch {
	case text == "":
		builtin.Note = path + " is empty; using the built-in rules"
	case len(raw) > promptMaxBytes:
		builtin.Note = fmt.Sprintf("%s is %d bytes, past the %d-byte limit; using the built-in rules",
			path, len(raw), promptMaxBytes)
	default:
		sum := sha256.Sum256([]byte(text))
		return PromptSource{Text: text, Tag: PromptVersion + "+" + hex.EncodeToString(sum[:4])}
	}
	return builtin
}
