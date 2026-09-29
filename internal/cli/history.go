package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/history"
	"reasonix/internal/recap"
)

// Seams overridden by tests so the command can search a temp session directory
// and read a temp recap projection without touching the user's config or cache.
var (
	historySearchOptions = func() history.Options {
		sessionDir := config.SessionDir()
		return history.Options{SessionDir: sessionDir, GlobalSessionDir: sessionDir, ArchiveDir: config.ArchiveDir()}
	}
	openHistoryRecapStore = func(ctx context.Context) (*recap.Store, error) {
		return recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	}
)

type historyRecapView struct {
	Goal        string `json:"goal"`
	Actions     string `json:"actions"`
	Conclusion  string `json:"conclusion"`
	FollowUps   string `json:"followUps,omitempty"`
	GeneratedAt string `json:"generatedAt,omitempty"`
	Model       string `json:"model,omitempty"`
}

type historyHitView struct {
	SessionID   string            `json:"sessionId"`
	SessionPath string            `json:"sessionPath"`
	Title       string            `json:"title,omitempty"`
	Time        string            `json:"time,omitempty"`
	Score       float64           `json:"score"`
	Snippet     string            `json:"snippet"`
	Recap       *historyRecapView `json:"recap,omitempty"`
}

type historyReport struct {
	Query string           `json:"query"`
	Hits  []historyHitView `json:"hits"`
}

func printHistoryUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: reasonix history <关键词> [--limit N] [--json]")
}

type historyArgs struct {
	query string
	limit int
	json  bool
	help  bool
}

// parseHistoryArgs scans by hand because the documented form puts the keyword
// first (`reasonix history <关键词> --json`), which Go's flag package would stop
// parsing at.
func parseHistoryArgs(args []string) (historyArgs, error) {
	var out historyArgs
	parts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			out.json = true
		case arg == "--limit":
			if i+1 >= len(args) {
				return historyArgs{}, fmt.Errorf("--limit needs a value")
			}
			n, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
			if err != nil {
				return historyArgs{}, fmt.Errorf("invalid --limit value %q", args[i+1])
			}
			out.limit = n
			i++
		case strings.HasPrefix(arg, "--limit="):
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--limit=")))
			if err != nil {
				return historyArgs{}, fmt.Errorf("invalid --limit value %q", strings.TrimPrefix(arg, "--limit="))
			}
			out.limit = n
		case arg == "-h" || arg == "--help":
			out.help = true
		case arg == "--":
			parts = append(parts, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(arg, "--"):
			return historyArgs{}, fmt.Errorf("unknown flag %q", arg)
		default:
			parts = append(parts, arg)
		}
	}
	out.query = strings.TrimSpace(strings.Join(parts, " "))
	return out, nil
}

// historyCommand is the user entry `reasonix history <关键词>`. Matching and
// ordering are the existing BM25 searcher's; the session recap is display-only
// and never participates in ranking (decision O4).
func historyCommand(args []string) int {
	parsed, err := parseHistoryArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHistoryUsage(os.Stderr)
		return 2
	}
	if parsed.help {
		printHistoryUsage(os.Stdout)
		return 0
	}
	if parsed.query == "" {
		printHistoryUsage(os.Stderr)
		return 2
	}

	ctx := context.Background()
	opts := historySearchOptions()
	// Scope global so archived conversations are searchable, matching the
	// "find a session from months ago" intent.
	hits, err := history.NewSearcher(opts).Search(ctx, history.SearchRequest{
		Query: parsed.query, Scope: "global", Limit: parsed.limit,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	store, err := openHistoryRecapStore(ctx)
	if err != nil {
		// A missing projection only means no recaps to show; hits still print.
		store = nil
	}
	defer func() { _ = store.Close() }()

	report := historyReport{Query: parsed.query, Hits: historyHitViews(ctx, hits, opts, store)}
	if parsed.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	printHistoryReport(report, os.Stdout)
	return 0
}

// historyHitViews collapses the per-message hits into one entry per session,
// keeping the highest-scoring hit's snippet, and joins each session's recap by
// its path (the agreed association key).
func historyHitViews(ctx context.Context, hits []history.Hit, opts history.Options, store *recap.Store) []historyHitView {
	titles := historyTitleIndex(opts)
	recaps := map[string]*historyRecapView{}
	seen := map[string]bool{}
	views := make([]historyHitView, 0, len(hits))
	for _, hit := range hits {
		if seen[hit.SessionPath] {
			continue
		}
		seen[hit.SessionPath] = true
		view := historyHitView{SessionID: hit.SessionID, SessionPath: hit.SessionPath, Score: hit.Score, Snippet: hit.Snippet}
		if info, ok := titles[historyPathKey(hit.SessionPath)]; ok {
			view.Title = historySessionTitle(info)
			view.Time = historySessionTime(info)
		}
		view.Recap = historyRecapFor(ctx, store, hit.SessionPath, recaps)
		views = append(views, view)
	}
	return views
}

// historyTitleIndex reads each searched root's lightweight sidecars once so a
// hit can show a human title and time without replaying its transcript.
func historyTitleIndex(opts history.Options) map[string]agent.SessionOrderInfo {
	index := map[string]agent.SessionOrderInfo{}
	seen := map[string]bool{}
	for _, dir := range []string{opts.SessionDir, opts.GlobalSessionDir, opts.ArchiveDir} {
		dir = strings.TrimSpace(dir)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		sessions, err := agent.ListSessionOrder(dir)
		if err != nil {
			continue
		}
		for _, session := range sessions {
			key := historyPathKey(session.Path)
			if _, ok := index[key]; !ok {
				index[key] = session
			}
		}
	}
	return index
}

func historyPathKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func historySessionTitle(info agent.SessionOrderInfo) string {
	for _, candidate := range []string{info.CustomTitle, info.TopicTitle, info.Preview} {
		if title := strings.TrimSpace(candidate); title != "" {
			return title
		}
	}
	return ""
}

func historySessionTime(info agent.SessionOrderInfo) string {
	at := info.LastActivityAt
	if at.IsZero() {
		at = info.ModTime
	}
	if at.IsZero() {
		return ""
	}
	return at.Format("2006-01-02 15:04")
}

func historyRecapFor(ctx context.Context, store *recap.Store, path string, cache map[string]*historyRecapView) *historyRecapView {
	if view, ok := cache[path]; ok {
		return view
	}
	var view *historyRecapView
	if rec, ok, err := store.Get(ctx, path); err == nil && ok {
		out := historyRecapView{
			Goal: rec.Goal, Actions: rec.Actions, Conclusion: rec.Conclusion,
			FollowUps: rec.FollowUps, Model: rec.Model,
		}
		if !rec.GeneratedAt.IsZero() {
			out.GeneratedAt = rec.GeneratedAt.Format("2006-01-02 15:04")
		}
		view = &out
	}
	cache[path] = view
	return view
}

func printHistoryReport(report historyReport, w io.Writer) {
	if len(report.Hits) == 0 {
		fmt.Fprintln(w, "无相关记录")
		return
	}
	fmt.Fprintf(w, "找到 %d 条相关会话记录（关键词 %q）：\n", len(report.Hits), report.Query)
	for i, hit := range report.Hits {
		label := hit.Title
		if label == "" {
			label = hit.SessionID
		}
		fmt.Fprintf(w, "\n%d. %s", i+1, label)
		if hit.Time != "" {
			fmt.Fprintf(w, " · %s", hit.Time)
		}
		fmt.Fprintf(w, "\n   路径: %s", hit.SessionPath)
		if strings.TrimSpace(hit.Snippet) != "" {
			fmt.Fprintf(w, "\n   命中: %s", hit.Snippet)
		}
		if hit.Recap == nil {
			fmt.Fprintf(w, "\n   会话回顾: （尚无回顾）")
		} else {
			fmt.Fprintf(w, "\n   会话回顾:")
			fmt.Fprintf(w, "\n     目标: %s", hit.Recap.Goal)
			fmt.Fprintf(w, "\n     关键动作: %s", hit.Recap.Actions)
			fmt.Fprintf(w, "\n     结论: %s", hit.Recap.Conclusion)
			if strings.TrimSpace(hit.Recap.FollowUps) != "" {
				fmt.Fprintf(w, "\n     待办: %s", hit.Recap.FollowUps)
			}
		}
		fmt.Fprintln(w)
	}
}
