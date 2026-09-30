package boot

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

// recapModelsFunc adapts one closure to the recap lane's model contract.
type recapModelsFunc func(context.Context, string) (provider.Provider, string, bool)

func (f recapModelsFunc) Resolve(ctx context.Context, sessionPath string) (provider.Provider, string, bool) {
	return f(ctx, sessionPath)
}

// resolveRecapModel builds an independent provider instance for one model ref.
type resolveRecapModel func(ref string) (provider.Provider, error)

// recapLanes keeps one lane per projection path for the whole process. A lane
// must outlive any single controller: closing several tabs at once calls the same
// close entry several times, and a session that just ended may not wait for its
// recap. The projection itself is opened per attempt, so a lane holds no file
// handle; keying by path keeps isolated test homes from sharing one lane.
var recapLanes = struct {
	mu    sync.Mutex
	byKey map[string]*recap.Runner
}{byKey: map[string]*recap.Runner{}}

// bindRecapLane attaches the process-wide recap lane to this controller's
// session-end seam. The lane is optional: a failure only costs recaps.
func bindRecapLane(ctx context.Context, cfg *config.Config, ctrl *control.Controller, sink event.Sink, resolver provider.Resolver, proxy netclient.ProxySpec) {
	// The lane yields while a turn is in flight: a recap must never compete with
	// the conversation it belongs to. The controller is the only thing that knows,
	// so its Running is what the generator asks.
	busy := func() bool { return ctrl != nil && ctrl.Running() }
	runner, created, err := sharedRecapLane(ctx, cfg, sink, recapResolverFor(resolver, cfg, proxy), busy)
	if err != nil {
		slog.Warn("session recap lane unavailable", "err", err.Error())
		return
	}
	if created {
		pruneRecapProjection(ctx, recap.DefaultPath())
	}
	// /clear and a new chat rotate away content the user dropped, so they owe no
	// recap; only an ordinary close ends a session worth keeping.
	ctrl.SetSessionEndObserver(func(reason, sessionPath string) {
		if reason == "clear" {
			runner.Trace("submit", sessionPath, "rejected: discarded")
			return
		}
		runner.Submit(sessionPath)
	})
}

// recapCompose remembers what a person-triggered preview needs: the same model
// resolution the lane uses and the same prompt directory, so a preview answers under
// the rules the lane would apply rather than its own idea of them.
var recapCompose = struct {
	mu        sync.RWMutex
	resolve   recap.ModelResolver
	promptDir string
	sink      event.Sink
}{}

// PreviewRecapMemory rewrites one note into the memory sentence a person is about
// to store. The caller passes the evidence it wants the rewrite to see.
func PreviewRecapMemory(ctx context.Context, sessionPath, evidence string) (recap.ComposeResult, error) {
	return previewRecapCompose(ctx, sessionPath, evidence, recap.ComposeMemory)
}

// PreviewRecapSkill writes a playbook from the notes of one topic.
func PreviewRecapSkill(ctx context.Context, sessionPath, evidence string) (recap.ComposeResult, error) {
	return previewRecapCompose(ctx, sessionPath, evidence, recap.ComposeSkill)
}

type recapComposeFunc func(context.Context, provider.Provider, string, string, recap.ComposeOptions) (recap.ComposeResult, error)

func previewRecapCompose(ctx context.Context, sessionPath, evidence string, compose recapComposeFunc) (recap.ComposeResult, error) {
	recapCompose.mu.RLock()
	resolve, promptDir, sink := recapCompose.resolve, recapCompose.promptDir, recapCompose.sink
	recapCompose.mu.RUnlock()
	if resolve == nil {
		return recap.ComposeResult{}, fmt.Errorf("the session recap lane is not running yet")
	}
	prov, ref, ok := resolve.Resolve(ctx, sessionPath)
	if !ok || prov == nil {
		return recap.ComposeResult{}, fmt.Errorf("no model is available for this session")
	}
	return compose(ctx, prov, ref, evidence, recap.ComposeOptions{PromptDir: promptDir, Sink: sink})
}

// recapResolverFor builds the lane's independent provider instances, so boot's
// assembly stays one call instead of an inline closure.
func recapResolverFor(resolver provider.Resolver, cfg *config.Config, proxy netclient.ProxySpec) resolveRecapModel {
	return func(ref string) (provider.Provider, error) {
		entry, ok := resolveOptionalEntry(resolver, cfg, strings.TrimSpace(ref))
		if !ok || entry == nil || strings.TrimSpace(entry.Model) == "" {
			return nil, fmt.Errorf("unknown session recap model %q", ref)
		}
		return resolveProvider(resolver, cfg, proxy, provider.Selection{Ref: modelRefFromEntry(entry)})
	}
}

// EnqueueSessionRecap asks the process-wide lane to (re)generate one session's
// recap now — the manual entry the recap page uses. It reuses the close path's
// lane on purpose: a second entry would carry its own concurrency, usage source
// and retry rules. False means no lane is running yet, or the path is already
// queued (the same "not now" the close path treats as harmless).
func EnqueueSessionRecap(sessionPath string) bool {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return false
	}
	recapLanes.mu.Lock()
	runner := recapLanes.byKey[recap.DefaultPath()]
	recapLanes.mu.Unlock()
	if runner == nil {
		return false
	}
	return runner.Submit(sessionPath)
}

// CloseRecapLanes abandons every lane. Work in flight is cancelled rather than
// waited for; the projection is disposable and the next process rebuilds it.
func CloseRecapLanes() {
	recapLanes.mu.Lock()
	lanes := recapLanes.byKey
	recapLanes.byKey = map[string]*recap.Runner{}
	recapLanes.mu.Unlock()
	for _, runner := range lanes {
		runner.Stop()
	}
}

// sharedRecapLane builds the lane for the current projection path once, then
// hands the same one to every later controller. A created lane is reported so the
// caller can run the projection's maintenance exactly once per process.
func sharedRecapLane(ctx context.Context, cfg *config.Config, sink event.Sink, resolve resolveRecapModel, busy func() bool) (*recap.Runner, bool, error) {
	key := recap.DefaultPath()
	recapLanes.mu.Lock()
	defer recapLanes.mu.Unlock()
	if runner, ok := recapLanes.byKey[key]; ok {
		return runner, false, nil
	}
	models := recapModelsFunc(func(ctx context.Context, sessionPath string) (provider.Provider, string, bool) {
		if ref, ok := agent.LoadSessionModel(sessionPath); ok {
			if prov, err := resolve(ref); err == nil && prov != nil {
				return prov, ref, true
			}
		}
		if entry, ok := cfg.ResolveSessionRecapModel(); ok && entry != nil {
			ref := entry.Name + "/" + entry.Model
			if prov, err := resolve(ref); err == nil && prov != nil {
				return prov, ref, true
			}
		}
		return nil, "", false
	})
	generator := recap.NewGenerator(recap.GeneratorOptions{
		// The override lives beside the projection, which is the one directory this
		// lane already knows: a prompt nobody can find is a prompt nobody edits.
		PromptDir: filepath.Dir(key),
		StoreFactory: func(ctx context.Context) (*recap.Store, error) {
			return recap.Open(ctx, recap.Options{Path: key})
		},
		Models:     models,
		Transcript: recap.FileTranscript{},
		Sink:       sink,
		Busy:       busy,
	})
	runner := recap.NewRunner(generator)
	recapLanes.byKey[key] = runner
	recapCompose.mu.Lock()
	recapCompose.resolve, recapCompose.promptDir, recapCompose.sink = models, filepath.Dir(key), sink
	recapCompose.mu.Unlock()
	return runner, true, nil
}

// pruneRecapProjection trims the projection once per process, on the path that
// builds the first lane. A failure only costs disk, so it never blocks the lane.
func pruneRecapProjection(ctx context.Context, path string) {
	store, err := recap.Open(ctx, recap.Options{Path: path})
	if err != nil {
		return
	}
	defer func() { _ = store.Close() }()
	report, err := store.Prune(ctx, time.Now())
	if err != nil {
		slog.Warn("session recap prune failed", "err", err.Error())
		return
	}
	if report.Records+report.Resumes+report.Activity > 0 {
		slog.Info("session recap projection pruned", "records", report.Records,
			"resumes", report.Resumes, "activity", report.Activity)
	}
}

// recapProjectOffers reads what one project carries into a turn: the unfinished
// items a person kept there, and the conclusions earlier sessions reached. The
// projection is opened per call: an offer only happens on a session's first turn
// or on a turn that says it continues something, so a cached handle would buy
// little and hold the file open between them.
func recapProjectOffers(project string) recap.Offers {
	project = strings.TrimSpace(project)
	if project == "" {
		return recap.Offers{}
	}
	ctx := context.Background()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return recap.Offers{}
	}
	defer func() { _ = store.Close() }()
	items, err := store.OpenItemsFor(ctx, project)
	if err != nil {
		items = nil
	}
	prior, err := store.PriorNotes(ctx, project, maxPriorNotesPerProject)
	if err != nil {
		prior = nil
	}
	return recap.Offers{Items: items, Prior: prior}
}

// maxPriorNotesPerProject caps what the projection hands the tail: the injected
// block is capped again, and a long list would only be read to be cut.
const maxPriorNotesPerProject = 8
