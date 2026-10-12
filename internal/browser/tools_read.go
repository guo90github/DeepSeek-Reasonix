package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"reasonix/internal/tool"
)

const screenshotMaxBytes = 8 << 20

func tabsTool(exec Executor) tool.Tool {
	return readTool{base: base{exec: exec, name: "browser_tabs",
		description: "List this task's browser tabs with ID, URL, title, and loading state. Start here to find a tabId, or call browser_open when the task has no tab yet.",
		schema:      objectSchema(nil),
		snip:        listSnip,
	}, run: runTabs}
}

func runTabs(ctx context.Context, exec Executor, args json.RawMessage) (string, error) {
	var p struct{}
	if err := decode(args, &p); err != nil {
		return "", err
	}
	tabs, err := exec.Tabs(ctx)
	if err != nil {
		return "", translate(err, "browser_tabs")
	}
	if len(tabs) == 0 {
		return "no tabs are open for this task; call browser_open to create one", nil
	}
	lines := make([]string, 0, len(tabs)+1)
	lines = append(lines, fmt.Sprintf("%d tab(s)", len(tabs)))
	for _, t := range tabs {
		lines = append(lines, formatTab(t))
	}
	return strings.Join(lines, "\n"), nil
}

func formatTab(t Tab) string {
	s := fmt.Sprintf("tab %s: %s", t.ID, t.URL)
	if t.Title != "" {
		s += fmt.Sprintf(" %q", t.Title)
	}
	var flags []string
	if t.Loading {
		flags = append(flags, "loading")
	}
	if t.Temporary {
		flags = append(flags, "temporary")
	}
	if len(flags) > 0 {
		s += " [" + strings.Join(flags, ", ") + "]"
	}
	return s
}

func snapshotTool(exec Executor) tool.Tool {
	return readTool{base: base{exec: exec, name: "browser_snapshot",
		description: "Capture the structural snapshot of a tab: an accessibility-style tree where each interactive element carries a ref such as ref=e12, plus the documentToken that binds those refs to the current document. Take a snapshot before browser_click, browser_type, browser_press, browser_scroll, browser_select, or browser_upload, and again after anything changes the page: refs and the token expire on navigation, page replacement, or user take-over. Pass selector to scope the tree to one subtree, search to keep only the lines matching a regular expression, or save to write the whole tree to a task-owned file you can read with offsets.",
		schema:      objectSchema([]string{"tabId"}, tabIDProp(), str("selector", "Optional CSS selector; only the matching subtree is captured."), str("search", "Optional regular expression (case-insensitive); only matching lines come back."), boolean("save", "Write the whole tree to a task-owned file and return its path instead of the tree.")),
		snip:        treeSnip,
	}, run: runSnapshot}
}

// treeHintLimit is where an inline tree stops arriving intact: long tool
// results are trimmed from the middle, which reads as a missing page.
const treeHintLimit = 16 << 10

func runSnapshot(ctx context.Context, exec Executor, args json.RawMessage) (string, error) {
	var p struct {
		TabID    string `json:"tabId"`
		Selector string `json:"selector"`
		Search   string `json:"search"`
		Save     bool   `json:"save"`
	}
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := requireTab(p.TabID); err != nil {
		return "", err
	}
	if p.Search != "" && p.Save {
		return "", fmt.Errorf("pass either search or save, not both: search returns matching lines, save writes the whole tree")
	}
	snap, err := exec.Snapshot(ctx, SnapshotRequest{TabID: p.TabID, Selector: p.Selector})
	if err != nil {
		return "", translate(err, "browser_snapshot")
	}
	header := fmt.Sprintf("documentToken: %s\nurl: %s\ntitle: %s\nrefs: %d", snap.DocumentToken, snap.URL, snap.Title, snap.Refs)
	switch {
	case p.Search != "":
		return header + "\n\n" + searchTree(snap.Tree, p.Search), nil
	case p.Save:
		return saveTree(ctx, exec, snap, header), nil
	}
	return header + "\n\n" + snap.Tree + sizeHint(snap.Tree), nil
}

func sizeHint(tree string) string {
	if len(tree) <= treeHintLimit {
		return ""
	}
	return fmt.Sprintf("\n\n[tree is %d bytes; a trimmed result loses its middle - pass search to keep matching lines, save to write it to a file, or selector to scope it]", len(tree))
}

func searchTree(tree, pattern string) string {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Sprintf("invalid search pattern %q: %v", pattern, err)
	}
	lines := strings.Split(tree, "\n")
	hits := make([]string, 0, 16)
	for i, line := range lines {
		if re.MatchString(line) {
			hits = append(hits, fmt.Sprintf("L%d: %s", i+1, line))
		}
	}
	if len(hits) == 0 {
		return fmt.Sprintf("search %q: no matching line (%d line(s) in the tree); pass save to write the tree out and read it yourself", pattern, len(lines))
	}
	return fmt.Sprintf("search %q: %d of %d line(s)\n%s", pattern, len(hits), len(lines), strings.Join(hits, "\n"))
}

func saveTree(ctx context.Context, exec Executor, snap Snapshot, header string) string {
	sc, ok := exec.(ScratchDir)
	if !ok {
		return header + "\n\nsave is unavailable on this host (no task scratch directory); pass search or selector instead\n\n" + snap.Tree
	}
	dir, err := sc.ScratchDir(ctx)
	if err == nil {
		path := filepath.Join(dir, fmt.Sprintf("snapshot-%d.txt", time.Now().UnixNano()))
		if err = os.WriteFile(path, []byte(snap.Tree), 0o600); err == nil {
			return fmt.Sprintf("%s\nlines: %d\nbytes: %d\nsaved: %s\n\nRead that file with offsets for the parts you need; the refs above come from this tree.", header, strings.Count(snap.Tree, "\n")+1, len(snap.Tree), path)
		}
	}
	return header + fmt.Sprintf("\n\nsave failed: %v\n\n%s", err, snap.Tree)
}

func downloadTool(exec Executor) tool.Tool {
	return readTool{base: base{exec: exec, name: "browser_download",
		description: "List the downloads of a tab, or wait up to waitSeconds for an in-progress one to finish. Each entry reports ID, URL, saved path, state, and size; the path is a task-owned file the file tools can read.",
		schema:      objectSchema([]string{"tabId"}, tabIDProp(), bounded(integer("waitSeconds", "Seconds to wait for an in-progress download to complete; 0 lists immediately."), 0, 300)),
		snip:        listSnip,
	}, run: runDownloads}
}

func runDownloads(ctx context.Context, exec Executor, args json.RawMessage) (string, error) {
	var p struct {
		TabID       string `json:"tabId"`
		WaitSeconds int    `json:"waitSeconds"`
	}
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := requireTab(p.TabID); err != nil {
		return "", err
	}
	if p.WaitSeconds < 0 || p.WaitSeconds > 300 {
		return "", fmt.Errorf("waitSeconds must be between 0 and 300")
	}
	downloads, err := exec.Downloads(ctx, DownloadsRequest{TabID: p.TabID, WaitFor: time.Duration(p.WaitSeconds) * time.Second})
	if err != nil {
		return "", translate(err, "browser_download")
	}
	if len(downloads) == 0 {
		return "no downloads for tab " + p.TabID, nil
	}
	lines := make([]string, 0, len(downloads)+1)
	lines = append(lines, fmt.Sprintf("%d download(s) for tab %s", len(downloads), p.TabID))
	for _, d := range downloads {
		lines = append(lines, fmt.Sprintf("download %s: %s %s -> %s (%d bytes)", d.ID, d.State, d.URL, d.Path, d.Bytes))
	}
	return strings.Join(lines, "\n"), nil
}

type screenshot struct{ base }

func consoleTool(exec Executor) tool.Tool {
	return readTool{base: base{exec: exec, name: "browser_console",
		description: "Read what the page logged: console messages and uncaught exceptions, newest last, held in memory only. Recording starts when this is first called for a tab, because attaching the debugger is visible to the page.",
		schema:      objectSchema([]string{"tabId"}, tabIDProp(), bounded(integer("limit", "Return at most this many newest entries; 0 returns all held."), 0, 200)),
		snip:        listSnip,
	}, run: runConsole}
}

func runConsole(ctx context.Context, exec Executor, args json.RawMessage) (string, error) {
	var p struct {
		TabID string `json:"tabId"`
		Limit int    `json:"limit"`
	}
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := requireTab(p.TabID); err != nil {
		return "", err
	}
	logs, ok := exec.(Logs)
	if !ok {
		return "", fmt.Errorf("reading console output is unavailable on this host")
	}
	entries, err := logs.Console(ctx, LogsRequest{TabID: p.TabID, Limit: p.Limit})
	if err != nil {
		return "", translate(err, "browser_console")
	}
	if len(entries) == 0 {
		return fmt.Sprintf("no console output recorded for tab %s; recording starts with the first browser_console call", p.TabID), nil
	}
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, fmt.Sprintf("%d console entr(y/ies) for tab %s", len(entries), p.TabID))
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%s: %s", entry.Level, entry.Text))
	}
	return strings.Join(lines, "\n"), nil
}

func networkTool(exec Executor) tool.Tool {
	return readTool{base: base{exec: exec, name: "browser_network",
		description: "Read the requests the page made: method, URL, status, MIME type and failure reason, newest last, held in memory only. Recording starts when this is first called for a tab, because attaching the debugger is visible to the page. Response bodies are never read.",
		schema:      objectSchema([]string{"tabId"}, tabIDProp(), bounded(integer("limit", "Return at most this many newest requests; 0 returns all held."), 0, 200)),
		snip:        listSnip,
	}, run: runNetwork}
}

func runNetwork(ctx context.Context, exec Executor, args json.RawMessage) (string, error) {
	var p struct {
		TabID string `json:"tabId"`
		Limit int    `json:"limit"`
	}
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := requireTab(p.TabID); err != nil {
		return "", err
	}
	logs, ok := exec.(Logs)
	if !ok {
		return "", fmt.Errorf("reading network traffic is unavailable on this host")
	}
	entries, err := logs.Network(ctx, LogsRequest{TabID: p.TabID, Limit: p.Limit})
	if err != nil {
		return "", translate(err, "browser_network")
	}
	if len(entries) == 0 {
		return fmt.Sprintf("no requests recorded for tab %s; recording starts with the first browser_network call", p.TabID), nil
	}
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, fmt.Sprintf("%d request(s) for tab %s", len(entries), p.TabID))
	for _, entry := range entries {
		line := fmt.Sprintf("%s %s", entry.Method, entry.URL)
		if entry.Status != 0 {
			line += fmt.Sprintf(" %d", entry.Status)
		}
		if entry.MIME != "" {
			line += " " + entry.MIME
		}
		if entry.Failed != "" {
			line += " failed: " + entry.Failed
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func waitTool(exec Executor) tool.Tool {
	return readTool{base: base{exec: exec, name: "browser_wait",
		description: "Wait for a tab to settle: with text, until that text appears in the rendered page, or disappears when gone is set; without text, until the tab stops loading. Timeout defaults to 10 seconds and is capped at 60. Waiting beats guessing a sleep; take a new browser_snapshot afterwards to see what arrived.",
		schema:      objectSchema([]string{"tabId"}, tabIDProp(), str("text", "Optional text to wait for (case-insensitive); empty waits for the tab to stop loading."), boolean("gone", "Wait for the text to disappear instead of appear."), bounded(integer("seconds", "Timeout in seconds; 0 uses 10."), 0, 60)),
		snip:        shortSnip,
	}, run: runWait}
}

func runWait(ctx context.Context, exec Executor, args json.RawMessage) (string, error) {
	var p struct {
		TabID   string `json:"tabId"`
		Text    string `json:"text"`
		Gone    bool   `json:"gone"`
		Seconds int    `json:"seconds"`
	}
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := requireTab(p.TabID); err != nil {
		return "", err
	}
	if p.Seconds < 0 || p.Seconds > 60 {
		return "", fmt.Errorf("seconds must be between 0 and 60")
	}
	waiter, ok := exec.(Waiter)
	if !ok {
		return "", fmt.Errorf("waiting is unavailable on this host; take a fresh browser_snapshot instead")
	}
	timeout := time.Duration(p.Seconds) * time.Second
	if p.Seconds == 0 {
		timeout = 10 * time.Second
	}
	res, err := waiter.Wait(ctx, WaitRequest{TabID: p.TabID, Text: p.Text, Gone: p.Gone, Timeout: timeout})
	if err != nil {
		return "", translate(err, "browser_wait")
	}
	what := "the tab to stop loading"
	switch {
	case p.Text != "" && p.Gone:
		what = fmt.Sprintf("text %q to disappear", p.Text)
	case p.Text != "":
		what = fmt.Sprintf("text %q", p.Text)
	}
	if res.Found {
		return fmt.Sprintf("waited %dms for %s", res.Elapsed.Milliseconds(), what), nil
	}
	return fmt.Sprintf("timeout after %dms waiting for %s; the page may still be settling - take a browser_snapshot to see where it is", res.Elapsed.Milliseconds(), what), nil
}

func screenshotTool(exec Executor) tool.Tool {
	return screenshot{base{exec: exec, name: "browser_screenshot",
		description: "Capture a PNG of a tab's viewport, of one element by ref, or of the full page, and return it as an image. Use browser_snapshot for structure and refs; use this to see layout, images, or rendering. Images over 8 MiB are not returned; capture an element or the viewport instead.",
		schema:      objectSchema([]string{"tabId"}, tabIDProp(), refProp("Optional element ref from browser_snapshot; captures only that element."), boolean("fullPage", "Capture the whole scrollable page instead of the viewport.")),
		snip:        shortSnip,
	}}
}

func (screenshot) ReadOnly() bool     { return true }
func (screenshot) PlanModeSafe() bool { return true }

func (t screenshot) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	text, images, err := t.ExecuteWithImages(ctx, args)
	if err == nil && len(images) > 0 {
		text += "\nVisual content requires a structured image channel and an image-capable model."
	}
	return text, err
}

func (t screenshot) ExecuteWithImages(ctx context.Context, args json.RawMessage) (string, []string, error) {
	if err := t.ready(ctx); err != nil {
		return "", nil, err
	}
	var p struct {
		TabID    string `json:"tabId"`
		Ref      string `json:"ref"`
		FullPage bool   `json:"fullPage"`
	}
	if err := decode(args, &p); err != nil {
		return "", nil, err
	}
	if err := requireTab(p.TabID); err != nil {
		return "", nil, err
	}
	shot, err := t.exec.Screenshot(ctx, ScreenshotRequest{TabID: p.TabID, Ref: p.Ref, FullPage: p.FullPage})
	if err != nil {
		return "", nil, translate(err, "browser_screenshot")
	}
	return encodeScreenshot(p.TabID, shot)
}

func encodeScreenshot(tabID string, shot Screenshot) (string, []string, error) {
	info, err := os.Stat(shot.Path)
	if err != nil {
		return "", nil, fmt.Errorf("read screenshot %s: %w", shot.Path, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("screenshot %s is not a regular file", shot.Path)
	}
	if info.Size() > screenshotMaxBytes {
		return oversizeText(shot.Path, info.Size()), nil, nil
	}
	f, err := os.Open(shot.Path)
	if err != nil {
		return "", nil, fmt.Errorf("read screenshot %s: %w", shot.Path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, screenshotMaxBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("read screenshot %s: %w", shot.Path, err)
	}
	if len(data) > screenshotMaxBytes {
		return oversizeText(shot.Path, int64(len(data))), nil, nil
	}
	// A page that has not painted yet yields a zero-byte capture; embedding it
	// would fail the whole provider request with HTTP 400.
	if len(data) == 0 {
		return "", nil, fmt.Errorf("screenshot of tab %s is empty (the page had not painted yet); take a fresh browser_snapshot and retry", tabID)
	}
	mime := shot.MIME
	if mime == "" {
		mime = "image/png"
	}
	text := fmt.Sprintf("[image: %s, %dx%d] screenshot of tab %s saved at %s", mime, shot.Width, shot.Height, tabID, shot.Path)
	return text, []string{"data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}, nil
}

func oversizeText(path string, size int64) string {
	return fmt.Sprintf("screenshot %s is %d bytes, over the %d MiB limit for an inline image, so it was not returned. Capture one element with ref, or the viewport without fullPage, to get a smaller image.", path, size, screenshotMaxBytes>>20)
}
