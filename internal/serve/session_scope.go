package serve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/provider"
	"reasonix/internal/store"
)

// projectsRoot is <home>/projects, the tree holding every workspace's sessions.
func projectsRoot() (string, error) {
	home := config.ReasonixHomeDir()
	if home == "" {
		return "", errors.New("sessions disabled")
	}
	root := filepath.Join(home, "projects")
	if _, err := os.Stat(root); err != nil {
		return "", errors.New("sessions disabled")
	}
	return root, nil
}

// allProjectSessionDirs returns every project's sessions directory. A remote
// client needs the whole tree: the foreground project alone is not the list the
// desktop sidebar shows, so /sessions would look like "only one project".
func allProjectSessionDirs() []string {
	root, err := projectsRoot()
	if err != nil {
		return nil
	}
	projects, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.IsDir() {
			dirs = append(dirs, filepath.Join(root, p.Name(), "sessions"))
		}
	}
	return dirs
}

// sessionPathAnywhere validates a client-supplied transcript path against the
// whole projects tree. resolveSessionPath is deliberately narrower (the
// foreground session dir) and rejects every other project's session.
func sessionPathAnywhere(raw string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(raw))
	if err != nil || !store.IsSessionTranscriptName(filepath.Base(abs)) {
		return "", errors.New("invalid session path")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errors.New("invalid session path")
	}
	root, err := projectsRoot()
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("sessions disabled")
	}
	if !strings.HasPrefix(real, realRoot+string(os.PathSeparator)) {
		return "", errors.New("path outside projects")
	}
	if agent.IsCleanupPending(real) {
		return "", errors.New("session is pending cleanup")
	}
	return real, nil
}

// readAnySession reads a session's transcript from disk. The file is the
// authority; the caller must not fall back to Serve's own history, which is a
// different session whenever the requested one is not the foreground.
func (s *Server) readAnySession(raw string) ([]provider.Message, bool) {
	path, err := sessionPathAnywhere(raw)
	if err != nil {
		return nil, false
	}
	return s.mirroredHistory(path)
}
