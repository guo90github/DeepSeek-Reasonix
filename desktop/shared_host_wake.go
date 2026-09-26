package main

import (
	"strings"

	"reasonix/internal/boot"
	"reasonix/internal/control"
	"reasonix/internal/plugin"
)

// armSharedHostWake gives a shared plugin host the sink its server wakes land
// in. A shared host outlives any one tab, so the wake is routed by the session
// the child last saw call it; without this sink an armed server's wake is only
// logged and dropped.
func (a *App) armSharedHostWake(host *plugin.Host) {
	if a == nil || host == nil {
		return
	}
	host.SetWakeHandler(boot.SharedWakeHandler(a.controllerForSessionPath))
}

// controllerForSessionPath resolves the tab that owns a session path. It never
// falls back to the selected tab: a wake naming no reachable session must stay
// a visible drop. Takes a.mu.RLock, so callers must not hold that lock.
func (a *App) controllerForSessionPath(sessionPath string) *control.Controller {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.tabs {
		if tab.SessionPath != path {
			continue
		}
		if ctrl, ok := tab.Ctrl.(*control.Controller); ok {
			return ctrl
		}
	}
	return nil
}
