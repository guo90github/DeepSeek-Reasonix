package doccmdgate

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// quotedPatternRe reads a quoted value whole, whatever spaces it holds: a space inside a
// pattern can never match a Go test name, so a reader that stopped at the blank would let
// exactly the dead command this gate exists to catch look green.
var quotedPatternRe = regexp.MustCompile(`-run(?:=|[ \t]+)['"]([^'"\n]*)['"]`)

// barePatternRe reads the bare and unclosed-quote spellings, stopping at the closing quote
// or inline-code backtick so a line citing many commands stays readable.
var barePatternRe = regexp.MustCompile("-run(?:=|[ \t]+)'?([^'\\s`]+)'?")

// documentedPatterns lists the -run patterns on one line. A quoted value wins over the bare
// reading inside its span, so a quoted two-word pattern is judged whole and not as its first word.
func documentedPatterns(line string) []string {
	spans := quotedPatternRe.FindAllStringSubmatchIndex(line, -1)
	var out []string
	for _, span := range spans {
		out = append(out, line[span[2]:span[3]])
	}
	for _, span := range barePatternRe.FindAllStringSubmatchIndex(line, -1) {
		insideQuoted := false
		for _, quoted := range spans {
			if span[0] >= quoted[0] && span[0] < quoted[1] {
				insideQuoted = true
				break
			}
		}
		if !insideQuoted {
			out = append(out, line[span[2]:span[3]])
		}
	}
	return out
}

var testFuncRe = regexp.MustCompile(`^func ((?:Test|Benchmark|Example|Fuzz)[A-Za-z0-9_]*)`)

// noPattern is the documented way to build a test binary without running anything,
// so it is the one pattern allowed to select nothing.
const noPattern = `^$`

// minPatternsSeen is the sentinel for the scan itself: a walk that silently stops
// finding commands would make this gate pass by saying nothing.
const minPatternsSeen = 20

// exempt records the documents this reading cannot judge, each with the reason.
// A file that records what a past round intended is not a command to run today,
// and a command aimed at a sibling repository has no test to find here.
var exempt = map[string]string{
	"internal/boot/testdata/README.md": "documents how a fixture was produced in the sibling `chatting` repository",
}

var skippedDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "build": true,
	"release": true, "sourcemaps": true, "logs": true,
}

type violation struct {
	file    string
	line    int
	pattern string
	why     string
}

func (v violation) String() string {
	return v.file + ":" + strconv.Itoa(v.line) + "  -run '" + v.pattern + "'  " + v.why
}

// checkDocs reads every `-run` pattern out of the given documents and reports the
// ones that cannot select a test. Inputs are plain maps so the countercase test can
// drive it without a repository.
func checkDocs(docs map[string]string, testNames []string) []violation {
	var out []violation
	for _, file := range slices.Sorted(maps.Keys(docs)) {
		if _, ok := exempt[file]; ok {
			continue
		}
		for i, line := range strings.Split(docs[file], "\n") {
			if !strings.Contains(line, "go test") {
				continue
			}
			for _, pattern := range documentedPatterns(line) {
				if pattern == noPattern {
					continue
				}
				if why := refusing(pattern, testNames); why != "" {
					out = append(out, violation{file: file, line: i + 1, pattern: pattern, why: why})
				}
			}
		}
	}
	return out
}

// refusing says why a pattern cannot select a test, or "" when it can.
func refusing(pattern string, testNames []string) string {
	if strings.Contains(pattern, `\|`) {
		return "grep-style `\\|` is a literal pipe in RE2: this selects nothing"
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "not a valid regexp: " + err.Error()
	}
	if slices.ContainsFunc(testNames, re.MatchString) {
		return ""
	}
	return "selects no test declared in this repo"
}

// docsIn collects the markdown this reading covers: generated trees and dated round
// logs are skipped, because a log records what was run once.
func docsIn(t *testing.T, root string) map[string]string {
	t.Helper()
	docs := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skippedDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
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
		docs[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
	return docs
}

func testNamesIn(t *testing.T, root string) []string {
	t.Helper()
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skippedDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			if match := testFuncRe.FindStringSubmatch(line); match != nil {
				seen[match[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk tests: %v", err)
	}
	return slices.Sorted(maps.Keys(seen))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("cannot locate the repository root from %s: %v", root, err)
	}
	return root
}

// A documented command is an instruction: when its pattern selects no test, the
// reader gets a green run that proves nothing. Both traps this repo has walked into
// are checked here — a dead pattern, and the grep-style `\|` that RE2 reads as a
// literal pipe (which is how `-run 'Wake\|Dispatch'` was once recorded as passing
// while it ran zero tests).
func TestEveryDocumentedRunPatternSelectsATest(t *testing.T) {
	root := repoRoot(t)
	testNames := testNamesIn(t, root)
	if len(testNames) < 1000 {
		t.Fatalf("found only %d test functions: the scan is broken, so the result is meaningless", len(testNames))
	}
	docs := docsIn(t, root)
	var seen int
	for _, text := range docs {
		seen += len(documentedPatterns(text))
	}
	if seen < minPatternsSeen {
		t.Fatalf("found only %d documented -run patterns (want at least %d): the scan is broken", seen, minPatternsSeen)
	}
	for file, why := range exempt {
		if _, ok := docs[file]; !ok {
			t.Errorf("an exemption outlived its file: %s (%s)", file, why)
		} else {
			t.Logf("exempt: %s (%s)", file, why)
		}
	}

	violations := checkDocs(docs, testNames)
	if len(violations) == 0 {
		t.Logf("%d documented -run patterns across %d markdown files all select tests", seen, len(docs))
		return
	}
	for _, v := range violations {
		t.Errorf("documented command probably runs nothing: %s", v)
	}
	t.Errorf("fix the pattern (or drop the command) instead of widening this gate: a green run that ran no test is the failure this gate exists to catch")
}

// The countercase: a gate nobody has seen fail is a gate nobody should trust.
func TestTheGateRefusesADeadPatternAndALiteralPipe(t *testing.T) {
	docs := map[string]string{
		"good.md": "go test -count=1 -run 'Live|AlsoLive' ./internal/live\n" +
			"go test -count=1 -run '^$' ./internal/build-only\n" +
			"a prose line about per-run control and `reasonix run --dry-run`\n",
		"dead.md": "go test -run 'Gone|AlsoGone' ./internal/live\ngo test -run 'Live\\|AlsoLive' .\n",
		"spaced.md": "go test -run 'Live AlsoLive' .",
	}
	violations := checkDocs(docs, []string{"Live", "AlsoLive", "TestLive"})

	if len(violations) != 3 {
		t.Fatalf("violations = %d (%v), want the two dead patterns and the literal pipe only", len(violations), violations)
	}
	if violations[0].file != "dead.md" || violations[0].line != 1 || violations[0].pattern != "Gone|AlsoGone" {
		t.Fatalf("first violation = %+v, want dead.md:1 'Gone|AlsoGone'", violations[0])
	}
	if !strings.Contains(violations[0].why, "selects no test") {
		t.Fatalf("first violation why = %q, want the dead-pattern reason", violations[0].why)
	}
	if violations[1].line != 2 || !strings.Contains(violations[1].why, "literal pipe") {
		t.Fatalf("second violation = %+v, want dead.md:2 named as the literal pipe", violations[1])
	}
	if violations[2].file != "spaced.md" || violations[2].pattern != "Live AlsoLive" {
		t.Fatalf("third violation = %+v, want spaced.md:1 'Live AlsoLive' read whole", violations[2])
	}
}
