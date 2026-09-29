package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

func init() {
	registerCatalogCommand(catalogCommand{
		name: "session-recap", path: recap.DefaultPath, reindex: reindexSessionRecap,
		completionFlags: []cliCompletionFlag{
			completionFlag("--json", cliCompletionNoValue),
			completionFlag("--dir", cliCompletionPathValue),
		},
	})
}

// sessionRecapReport is the three-state tally the backfill command prints: a
// fresh recap, one already current (or skipped as inadmissible), or one the
// lane could not produce.
type sessionRecapReport struct {
	Directories int `json:"directories"`
	Generated   int `json:"generated"`
	Skipped     int `json:"skipped"`
	Failed      int `json:"failed"`
}

// Seams overridden by tests so the command can run against an in-temp store and
// a fake model resolver; production opens the disposable recap projection and
// resolves providers from the user's config.
var (
	openSessionRecapStore = func(ctx context.Context) (*recap.Store, error) {
		path := recap.DefaultPath()
		if path == "" {
			fmt.Fprintln(os.Stderr, "warning: recap cache path unavailable; using an in-memory projection, recaps will not persist")
		}
		return recap.Open(ctx, recap.Options{Path: path})
	}
	newSessionRecapModelResolver = func() (recap.ModelResolver, error) {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		return sessionRecapModels{cfg: cfg}, nil
	}
)

func reindexSessionRecap(args []string) int {
	fs := flag.NewFlagSet("catalogs reindex session-recap", flag.ContinueOnError)
	var dirs stringListFlag
	jsonOut := fs.Bool("json", false, "print status as JSON")
	fs.Var(&dirs, "dir", "session directory to scan; repeat for multiple directories")
	if code, ok := parseCommandFlags(fs, args); !ok {
		return code
	}

	ctx := context.Background()
	store, err := openSessionRecapStore(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	models, err := newSessionRecapModelResolver()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: failed to resolve recap model:", err)
		return 1
	}
	generator := recap.NewGenerator(recap.GeneratorOptions{
		Store:      store,
		Models:     models,
		Transcript: recap.FileTranscript{},
		// Busy stays nil: the batch makes way for no session turn.
		Busy: nil,
	})

	report := sessionRecapReport{}
	for _, dir := range sessionRecapRoots(dirs) {
		report.Directories++
		sessions, err := agent.ListSessionOrder(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
			continue
		}
		for _, session := range sessions {
			// Admissible is the recap package's own rule: primary transcript,
			// not a recovery copy, not pending cleanup.
			if !recap.Admissible(session.Path, nil) {
				report.Skipped++
				continue
			}
			// Generate is idempotent per session, so an interrupted run resumes
			// by skipping the recaps already stored.
			result, err := generator.Generate(ctx, session.Path)
			switch {
			case err != nil:
				report.Failed++
			case result.Stored:
				report.Generated++
			case result.Reason == "current":
				report.Skipped++
			default:
				report.Failed++
			}
		}
	}
	return printCatalogStatus(report, *jsonOut)
}

// sessionRecapRoots enumerates the authoritative session roots: the user-global
// and per-project session directories plus the compacted-history archive. When
// dirs is non-empty the scan is restricted to them, mirroring `catalogs reindex
// history`.
func sessionRecapRoots(dirs []string) []string {
	if len(dirs) > 0 {
		out := make([]string, 0, len(dirs))
		for _, dir := range dirs {
			out = append(out, filepath.Clean(dir))
		}
		return out
	}
	out := make([]string, 0)
	for _, target := range defaultSessionCatalogTargets() {
		if target.Path != "" {
			out = append(out, target.Path)
		}
	}
	if archive := config.ArchiveDir(); archive != "" {
		out = append(out, archive)
	}
	return out
}

// sessionRecapModels resolves the provider behind one recap: the model the
// session itself recorded, then the configured session-recap fallback. Returning
// ok=false lets the lane mark the session pending instead of guessing a model.
type sessionRecapModels struct {
	cfg *config.Config
}

func (m sessionRecapModels) Resolve(_ context.Context, sessionPath string) (provider.Provider, string, bool) {
	if ref, ok := agent.LoadSessionModel(sessionPath); ok {
		if entry, ok := m.cfg.ResolveModel(ref); ok && entry.Configured() {
			if prov, err := boot.NewProviderWithProxy(entry, m.cfg.NetworkProxySpec()); err == nil {
				return prov, ref, true
			}
		}
	}
	if entry, ok := m.cfg.ResolveSessionRecapModel(); ok {
		if prov, err := boot.NewProviderWithProxy(entry, m.cfg.NetworkProxySpec()); err == nil {
			return prov, entry.Name + "/" + entry.Model, true
		}
	}
	return nil, "", false
}
