package boot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

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
	runner, err := sharedRecapLane(ctx, cfg, sink, recapResolverFor(resolver, cfg, proxy))
	if err != nil {
		slog.Warn("session recap lane unavailable", "err", err.Error())
		return
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
// hands the same one to every later controller.
func sharedRecapLane(ctx context.Context, cfg *config.Config, sink event.Sink, resolve resolveRecapModel) (*recap.Runner, error) {
	key := recap.DefaultPath()
	recapLanes.mu.Lock()
	defer recapLanes.mu.Unlock()
	if runner, ok := recapLanes.byKey[key]; ok {
		return runner, nil
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
		StoreFactory: func(ctx context.Context) (*recap.Store, error) {
			return recap.Open(ctx, recap.Options{Path: key})
		},
		Models:     models,
		Transcript: recap.FileTranscript{},
		Sink:       sink,
	})
	runner := recap.NewRunner(generator)
	recapLanes.byKey[key] = runner
	return runner, nil
}

// recapOpenHandoffs reads one project's unfinished items for the controller's
// turn-tail offer. The projection is opened per call: an offer only happens on a
// session's first turn or on a turn that says it continues something, so a
// cached handle would buy little and hold the file open between them.
func recapOpenHandoffs(project string) []recap.OpenItem {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil
	}
	ctx := context.Background()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	items, err := store.OpenItemsFor(ctx, project)
	if err != nil {
		return nil
	}
	return items
}
