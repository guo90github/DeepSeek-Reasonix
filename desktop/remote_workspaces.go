package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/serve"
)

// remoteNewSessionTarget resolves the workspace POST /new asked for. A caller
// that names none keeps the window's own default — the foreground tab's scope —
// and so still needs a writable foreground tab; one that names a workspace does
// not, which is the whole point of naming it.
func (a *App) remoteNewSessionTarget(req serve.NewSessionRequest) (string, string, error) {
	scope := strings.TrimSpace(req.Scope)
	root := strings.TrimSpace(req.ProjectRoot)
	switch scope {
	case "":
		tab, _ := a.activeTabAndCtrl()
		if tab == nil || tab.ReadOnly {
			return "", "", errors.New("no writable tab is foreground in the desktop window")
		}
		if tab.Scope == "project" && strings.TrimSpace(tab.WorkspaceRoot) != "" {
			return "project", tab.WorkspaceRoot, nil
		}
		return "global", "", nil
	case "global":
		return "global", "", nil
	case "project":
	default:
		return "", "", fmt.Errorf("unknown scope %q", scope)
	}
	if root == "" {
		return "", "", errors.New("projectRoot is required for the project scope")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve project folder: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("open project folder: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("project path is not a directory: %s", abs)
	}
	return "project", abs, nil
}

// listProjectsForRemote answers GET /projects: the workspaces a remote caller
// can bind a new session to. The remembered workspace list carries projects
// that have no session yet, the session index carries roots that list has
// dropped, and the global workspace leads as the unbound scope.
func (a *App) listProjectsForRemote() []serve.ProjectEntry {
	out := make([]serve.ProjectEntry, 0, 8)
	seen := map[string]bool{}
	add := func(root, scope, name string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		// Windows paths differing only by case are one workspace.
		if key := strings.ToLower(filepath.Clean(root)); !seen[key] {
			seen[key] = true
			if name == "" {
				name = filepath.Base(root)
			}
			out = append(out, serve.ProjectEntry{Root: root, Name: name, Scope: scope})
		}
	}
	add(globalWorkspaceRoot(), "global", "global")
	for _, root := range loadWorkspaces() {
		add(root, "project", "")
	}
	for _, row := range a.listSessionsForRemote(true) {
		add(row.ProjectRoot, "project", "")
	}
	return out
}
