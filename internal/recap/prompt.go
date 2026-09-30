package recap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MemoryPrompt rewrites one note into the sentence that will be stored as memory.
const memoryPrompt = `You turn one finding from a coding session into a single memory sentence that a future session will read as established fact.

The finding arrives as a note: its kind, its body, the evidence and pointers the session recorded, the tier it was proposed for, and how many other projects reached it. Neighbouring notes from the same session may follow — they are context, not extra memories: never merge them into this sentence.

Write exactly one sentence, in the note's own language, and answer with that sentence and nothing else. Rules:
- Keep the note's meaning exactly. Never add a fact, a number, a path or a caveat the note does not carry, and never soften a rule into a suggestion ("prefer" is not "always").
- Stand alone: no "this session", no "as decided", no "the earlier fix", no reference to a transcript, a turn or a checklist.
- Write what is true now, in the present tense, as a rule, a preference or a fact of the project. Keep every identifier, path, flag, version and number verbatim, exactly as the note writes it.
- Drop what only made sense while working: which command was run, what was verified once, how long it took, who was thanked.
- One clause per idea, no more than 160 characters. No bullet list, no markdown, no trailing summarising line.
- If the note is about how to work (a preference, a convention, a thing never to do again), write it as an instruction to the next session; if it is about how the project is, write it as a statement.
- Never include secrets, credentials, hostnames or internal addresses.`

// SkillPrompt writes a playbook from the notes of one topic.
const skillPrompt = `You write one playbook from a set of notes a coding session already accepted. Answer with markdown only.

The notes are given in the order the session produced them, each with its kind (a diagnosis, a ruled-out approach), its pointers and its evidence. Treat them as the only source of truth.
- Never invent a step, a path, a command, a flag or a number that is not in the notes, and never explain again why a note is true.
- Order the steps the way the work is actually done, not the way the notes were written.
- Each step says what to do and how to tell it worked, and keeps that note's pointers with it, exactly as written.
- Two notes describing the same step become one step carrying both sets of pointers.
- A note that is a rule rather than a step ("never X", "always Y") goes into a "Never" list at the end.
- When notes disagree, keep the newer one and say in one line which earlier claim it replaces.
- If the notes do not add up to a procedure yet, say so in one line under the title and still list what they do establish.

Shape: a title line, then "## Steps" with numbered steps, then "## Never" only when you have any, then "## Where to check" listing the pointers you actually used. No preamble, no summary of the notes, no advice that is not in them. Write in the notes' own language and keep identifiers verbatim.`

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
	// MemoryPromptTag and SkillPromptTag version the two button-driven prompts the
	// same way PromptVersion versions the notes: a change has to be visible in what
	// was produced, not only in the file.
	MemoryPromptTag = "memory-v1"
	SkillPromptTag  = "skill-v1"
	// PromptFileName is the override a person can drop into the state directory for
	// the rules that produce the notes themselves.
	PromptFileName = "recap-prompt.md"
	// MemoryPromptFileName overrides the rewrite of one note into a memory sentence.
	MemoryPromptFileName = "recap-memory-prompt.md"
	// SkillPromptFileName overrides the playbook written from a topic's notes.
	SkillPromptFileName = "recap-skill-prompt.md"
	// promptMaxBytes keeps a mistaken paste from becoming the rules: a prompt the
	// lane cannot afford to send is worse than the one it ships with.
	promptMaxBytes = 32 * 1024
)

// BuiltinPrompt is the rules compiled into this binary.
func BuiltinPrompt() PromptSource {
	return PromptSource{Text: recapSystemPrompt, Tag: PromptVersion}
}

// BuiltinMemoryPrompt rewrites one note into the sentence that becomes memory.
func BuiltinMemoryPrompt() PromptSource {
	return PromptSource{Text: memoryPrompt, Tag: MemoryPromptTag}
}

// BuiltinSkillPrompt writes a playbook from the notes of one topic.
func BuiltinSkillPrompt() PromptSource {
	return PromptSource{Text: skillPrompt, Tag: SkillPromptTag}
}

// LoadMemoryPromptOverride reads the memory rewrite rules, if a person wrote them.
func LoadMemoryPromptOverride(dir string) PromptSource {
	return loadPromptOverride(dir, MemoryPromptFileName, BuiltinMemoryPrompt())
}

// LoadSkillPromptOverride reads the playbook rules, if a person wrote them.
func LoadSkillPromptOverride(dir string) PromptSource {
	return loadPromptOverride(dir, SkillPromptFileName, BuiltinSkillPrompt())
}

// LoadPromptOverride reads dir/PromptFileName. Every failure falls back to the
// built-in rules and says why in Note: half a prompt would silently lower the
// quality of everything the lane writes, and the note is what makes that visible.
func LoadPromptOverride(dir string) PromptSource {
	return loadPromptOverride(dir, PromptFileName, BuiltinPrompt())
}

// loadPromptOverride is the one place the fallback policy lives, so all three
// prompts behave the same when a file is missing, empty, oversized or unreadable.
func loadPromptOverride(dir, name string, builtin PromptSource) PromptSource {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return builtin
	}
	path := filepath.Join(dir, name)
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
		return PromptSource{Text: text, Tag: builtin.Tag + "+" + hex.EncodeToString(sum[:4])}
	}
	return builtin
}
