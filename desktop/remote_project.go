package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/serve"
)

// createProjectForRemote is the embedded host's half of POST /new-project. A
// phone cannot browse this machine's filesystem, so it sends either an absolute
// path that already exists or a parent folder plus a folder name to create
// below it. Creating reuses the desktop's own blank-project rule (one child
// directory, never overwrite); opening reuses the workspace navigation path,
// which is what registers the project for the sidebar and binds a session to
// it — a standalone serve cannot move its workspace root at all.
func (a *App) createProjectForRemote(req serve.NewProjectRequest) (serve.NewProjectResult, error) {
	root := strings.TrimSpace(req.Root)
	if root == "" {
		created, err := createBlankProject(req.Parent, req.Name)
		if err != nil {
			return serve.NewProjectResult{}, err
		}
		root = created
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return serve.NewProjectResult{}, fmt.Errorf("resolve project folder: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return serve.NewProjectResult{}, fmt.Errorf("open project folder: %w", err)
	}
	if !info.IsDir() {
		return serve.NewProjectResult{}, fmt.Errorf("project path is not a directory: %s", abs)
	}
	meta, err := a.OpenProjectTab(abs, "")
	if err != nil {
		return serve.NewProjectResult{}, fmt.Errorf("open project: %w", err)
	}
	if strings.TrimSpace(meta.SessionPath) == "" {
		return serve.NewProjectResult{}, errors.New("the new project has no session the phone can address")
	}
	// A caller that switches to this session right away would otherwise race the
	// tab's controller build and land on whichever tab was ready first.
	a.waitForTabBuildReady(meta.ID)
	return serve.NewProjectResult{Root: abs, Name: filepath.Base(abs), SessionPath: meta.SessionPath}, nil
}
