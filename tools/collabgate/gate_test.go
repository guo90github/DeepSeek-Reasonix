package collabgate

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const surfaceDoc = "docs/COLLAB-SURFACE.md"

var (
	anchorRe  = regexp.MustCompile("`([A-Za-z0-9_./-]+\\.(?:go|ts|tsx)):(\\d+)`")
	commandRe = regexp.MustCompile(`sed -n '(\d+),(\d+)p' ([^ ]+) *# *§(\d+)`)
	tokenRe   = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_.\\-\\[\\]]*)`")
	citeRe    = regexp.MustCompile(`docs/[A-Za-z0-9_.-]+\.md`)
	sectionRe = regexp.MustCompile(`(\d+)`)
)

type anchor struct {
	section int
	file    string
	line    int
	tokens  []string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, surfaceDoc)); err != nil {
		t.Fatalf("cannot locate the collaboration surface from %s: %v", root, err)
	}
	return root
}

func readLines(t *testing.T, root, rel string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("anchor cites a file that does not exist: %s (%v)", rel, err)
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
}

// anchorsFrom parses the surface doc's markdown rows: the value cell names the
// tokens the row is about, the anchor cell names where they live.
func anchorsFrom(doc string) []anchor {
	var out []anchor
	section := 0
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = sectionNumber(line)
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		var tokens []string
		for _, cell := range cells {
			for _, match := range tokenRe.FindAllStringSubmatch(cell, -1) {
				if len(match[1]) >= 3 {
					tokens = append(tokens, match[1])
				}
			}
		}
		for _, match := range anchorRe.FindAllStringSubmatch(line, -1) {
			number, err := strconv.Atoi(match[2])
			if err != nil {
				continue
			}
			out = append(out, anchor{section: section, file: match[1], line: number, tokens: tokens})
		}
	}
	return out
}

func sectionNumber(text string) int {
	match := sectionRe.FindStringSubmatch(text)
	if match == nil {
		return 0
	}
	number, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return number
}

// candidates expands a claimed token into what code may plausibly spell it as:
// the token itself, and each dot-separated segment (Spec.WakeMethod is declared
// as WakeMethod).
func candidates(token string) []string {
	out := []string{token}
	for _, part := range strings.Split(token, ".") {
		if len(part) >= 4 && part != token {
			out = append(out, part)
		}
	}
	return out
}

// codeWindow keeps only the lines a reader would call code: a `文件:行` anchor
// is a claim about code, so a pointer that lands on prose must not pass on the
// word alone.
func codeWindow(lines []string, line int) string {
	low, high := line-3, line+2
	if low < 1 {
		low = 1
	}
	if high > len(lines) {
		high = len(lines)
	}
	var code []string
	for _, candidate := range lines[low-1 : min(high, len(lines))] {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		code = append(code, candidate)
	}
	return strings.ToLower(strings.Join(code, "\n"))
}

func window(lines []string, line int) string {
	low, high := line-3, line+2
	if low < 1 {
		low = 1
	}
	if high > len(lines) {
		high = len(lines)
	}
	if low > len(lines) {
		return ""
	}
	return strings.ToLower(strings.Join(lines[low-1:high], "\n"))
}

// TestCollabSurfaceAnchorsResolveToClaimedTokens is the coordinate reading: a
// drifted `文件:行` (the plugin.go 92→95 class of rot) must fail here rather
// than wait for someone to notice by hand.
func TestCollabSurfaceAnchorsResolveToClaimedTokens(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, surfaceDoc))
	if err != nil {
		t.Fatal(err)
	}
	anchors := anchorsFrom(string(data))
	checkable := 0
	cache := map[string][]string{}
	for _, a := range anchors {
		if len(a.tokens) == 0 {
			continue
		}
		checkable++
		lines, ok := cache[a.file]
		if !ok {
			lines = readLines(t, root, a.file)
			cache[a.file] = lines
		}
		if a.line < 1 || a.line > len(lines) {
			t.Errorf("§%d: anchor %s:%d is past the end of the file (%d lines)", a.section, a.file, a.line, len(lines))
			continue
		}
		text := window(lines, a.line)
		matched := false
		for _, token := range a.tokens {
			for _, candidate := range candidates(token) {
				if strings.Contains(codeWindow(lines, a.line), strings.ToLower(candidate)) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		// A frozen contract stated in a comment has no code line to point at, so
		// that row passes on naming several tokens; a pointer drifted onto a
		// comment naming exactly one is the rot this reading catches.
		if !matched {
			hits := 0
			for _, token := range a.tokens {
				for _, candidate := range candidates(token) {
					if strings.Contains(text, strings.ToLower(candidate)) {
						hits++
						break
					}
				}
			}
			matched = hits >= 2
		}
		if !matched {
			t.Errorf("§%d: %s:%d does not carry any of %v outside a comment — coordinate drifted?", a.section, a.file, a.line, a.tokens)
		}
	}
	if checkable < 6 {
		t.Errorf("only %d of %d anchors carry a machine-readable token; the table stopped being checkable", checkable, len(anchors))
	}
}

// TestCollabSurfaceCommandsCoverTheirSectionAnchors ties §0's runnable commands
// to the table rows they are advertised for: the command's range must actually
// contain every anchor of the section it labels.
func TestCollabSurfaceCommandsCoverTheirSectionAnchors(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, surfaceDoc))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	type command struct {
		file    string
		from    int
		to      int
		section int
	}
	var commands []command
	for _, match := range commandRe.FindAllStringSubmatch(doc, -1) {
		from, errFrom := strconv.Atoi(match[1])
		to, errTo := strconv.Atoi(match[2])
		if errFrom != nil || errTo != nil {
			continue
		}
		commands = append(commands, command{file: match[3], from: from, to: to, section: sectionNumber(match[4])})
	}
	if len(commands) < 5 {
		t.Fatalf("§0 advertises %d runnable commands, want at least 5", len(commands))
	}
	anchors := anchorsFrom(doc)
	for _, cmd := range commands {
		lines := readLines(t, root, cmd.file)
		if cmd.from < 1 || cmd.to > len(lines) || cmd.from > cmd.to {
			t.Errorf("§0 command range %d,%d is out of bounds for %s (%d lines)", cmd.from, cmd.to, cmd.file, len(lines))
			continue
		}
		covered := 0
		for _, a := range anchors {
			if a.file != cmd.file || a.section != cmd.section {
				continue
			}
			covered++
			if a.line < cmd.from || a.line > cmd.to {
				t.Errorf("§%d: anchor %s:%d falls outside its advertised command range %d,%d", a.section, a.file, a.line, cmd.from, cmd.to)
			}
		}
		if covered == 0 {
			t.Errorf("§0 labels %s as §%d but no table row in that section anchors into it", cmd.file, cmd.section)
		}
	}
}

// TestDocumentedNamesAndCitedDocPaths covers both directions of A11: a name the
// docs promise must exist in code, and a docs/*.md path cited from non-test Go
// source must exist on disk (a citation to a doc that was never written is the
// same kind of dangling reference as an unpublished rule number).
func TestDocumentedNamesAndCitedDocPaths(t *testing.T) {
	root := repoRoot(t)
	docs := []string{surfaceDoc, "docs/REMOTE_SESSIONS.md", "docs/REMOTE_SESSIONS.zh-CN.md"}
	names := []struct {
		name     string
		codeFile string
		docsMust string
	}{
		{"[remote wake source=", "internal/control/inbox_wake_marker.go", "docs/REMOTE_SESSIONS.md"},
		{"WakeMethod", "internal/plugin/plugin.go", surfaceDoc},
		{"X-Reasonix-Reject-Class", "internal/serve/reject_class.go", "docs/REMOTE_SESSIONS.md"},
		{"steer_accepted", "internal/sessioninbox/types.go", "docs/REMOTE_SESSIONS.md"},
		{"wake_method", "internal/config/plugin_entry.go", surfaceDoc},
	}
	for _, entry := range names {
		code := strings.Join(readLines(t, root, entry.codeFile), "\n")
		if !strings.Contains(code, entry.name) {
			t.Errorf("code lost the documented name %q (expected in %s)", entry.name, entry.codeFile)
		}
		documented := strings.Join(readLines(t, root, entry.docsMust), "\n")
		if !strings.Contains(documented, entry.name) {
			t.Errorf("docs lost %q (expected in %s)", entry.name, entry.docsMust)
		}
	}
	for _, rel := range docs {
		if len(readLines(t, root, rel)) == 0 {
			t.Errorf("%s is empty", rel)
		}
	}
	cited := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "build" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, citation := range citeRe.FindAllString(string(data), -1) {
			cited[citation] = append(cited[citation], filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cited) == 0 {
		t.Fatal("found no docs/*.md citation in non-test Go source; the walk changed shape")
	}
	for citation, files := range cited {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(citation))); err != nil {
			t.Errorf("dangling doc citation %s (cited from %s)", citation, strings.Join(files, ", "))
		}
	}
}
