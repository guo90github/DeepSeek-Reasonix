package browser

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
)

// scopedExecutor adds the optional ScratchDir capability to the fake.
type scopedExecutor struct {
	*fakeExecutor
	dir string
}

func (s scopedExecutor) ScratchDir(context.Context) (string, error) { return s.dir, nil }

func snapshotFixture() *fakeExecutor {
	return &fakeExecutor{snapshot: Snapshot{
		DocumentToken: "doc-9", URL: "https://a.example", Title: "A", Refs: 2,
		Tree: "document\n  button \"Go\" ref=e1\n  text \"away\"\n",
	}}
}

func TestSnapshotSearchReturnsMatchingLines(t *testing.T) {
	out, err := run(t, snapshotFixture(), "browser_snapshot", `{"tabId":"t1","search":"ref=e"}`)
	if err != nil {
		t.Fatalf("search snapshot: %v", err)
	}
	if !strings.HasPrefix(out, "documentToken: doc-9\n") {
		t.Fatalf("search must keep the documentToken: %q", out)
	}
	if !strings.Contains(out, `search "ref=e": 1 of 4 line(s)`) || !strings.Contains(out, `L2:   button "Go" ref=e1`) {
		t.Fatalf("search out = %q", out)
	}
	if strings.Contains(out, "away") {
		t.Fatalf("search leaked a non-matching line: %q", out)
	}
}

func TestSnapshotSearchWithNoMatchExplainsItself(t *testing.T) {
	out, err := run(t, snapshotFixture(), "browser_snapshot", `{"tabId":"t1","search":"nothing-here"}`)
	if err != nil {
		t.Fatalf("empty search: %v", err)
	}
	if !strings.Contains(out, "no matching line") {
		t.Fatalf("out = %q", out)
	}
}

func TestSnapshotReportsAnInvalidSearchPattern(t *testing.T) {
	out, err := run(t, snapshotFixture(), "browser_snapshot", `{"tabId":"t1","search":"("}`)
	if err != nil {
		t.Fatalf("invalid pattern must be reported, not raised: %v", err)
	}
	if !strings.Contains(out, "invalid search pattern") {
		t.Fatalf("out = %q", out)
	}
}

func TestSnapshotSaveWritesATaskOwnedFile(t *testing.T) {
	dir := t.TempDir()
	fake := scopedExecutor{fakeExecutor: snapshotFixture(), dir: dir}
	out, err := run(t, fake, "browser_snapshot", `{"tabId":"t1","save":true}`)
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	match := regexp.MustCompile(`(?m)^saved: (\S+)$`).FindStringSubmatch(out)
	if match == nil || !strings.HasPrefix(match[1], dir) {
		t.Fatalf("save out = %q (dir %s)", out, dir)
	}
	data, err := os.ReadFile(match[1])
	if err != nil {
		t.Fatalf("read the saved tree: %v", err)
	}
	if string(data) != fake.snapshot.Tree {
		t.Fatalf("saved tree = %q", string(data))
	}
	if !strings.Contains(out, "documentToken: doc-9") {
		t.Fatalf("save must report the token too: %q", out)
	}
}

func TestSnapshotSaveWithoutScratchFallsBackToInline(t *testing.T) {
	out, err := run(t, snapshotFixture(), "browser_snapshot", `{"tabId":"t1","save":true}`)
	if err != nil {
		t.Fatalf("save fallback: %v", err)
	}
	if !strings.Contains(out, "save is unavailable on this host") || !strings.Contains(out, "ref=e1") {
		t.Fatalf("fallback out = %q", out)
	}
}

func TestSnapshotRejectsSearchTogetherWithSave(t *testing.T) {
	_, err := run(t, snapshotFixture(), "browser_snapshot", `{"tabId":"t1","search":"x","save":true}`)
	if err == nil || !strings.Contains(err.Error(), "either search or save") {
		t.Fatalf("err = %v", err)
	}
}

func TestSnapshotHintsWhenTheTreeIsTooLargeToArriveIntact(t *testing.T) {
	fake := &fakeExecutor{snapshot: Snapshot{DocumentToken: "doc-9", Tree: strings.Repeat("  text \"filler\"\n", 2000)}}
	out, err := run(t, fake, "browser_snapshot", `{"tabId":"t1"}`)
	if err != nil {
		t.Fatalf("large snapshot: %v", err)
	}
	if !strings.Contains(out, "a trimmed result loses its middle") {
		t.Fatalf("a 30 KiB tree must carry the hint: %q", out[len(out)-200:])
	}
}
