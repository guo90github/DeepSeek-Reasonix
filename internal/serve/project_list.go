package serve

import (
	"net/http"
	"strings"
)

// ProjectEntry is one workspace a remote caller can bind a new session to.
// Scope "global" carries the host's global workspace — the scope a session
// with no project belongs to — and everything else is a project root.
type ProjectEntry struct {
	Root  string `json:"root"`
	Name  string `json:"name,omitempty"`
	Scope string `json:"scope,omitempty"`
}

// SetProjectLister installs the embedded host's workspace index for
// GET /projects. Only a host can enumerate it: a standalone serve is pinned to
// one working directory for the life of the process, so it has no list to
// offer and a remote caller falls back to the projects it sees in /sessions.
func (s *Server) SetProjectLister(f func() []ProjectEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectLister = f
}

func (s *Server) projectListerFunc() func() []ProjectEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projectLister
}

// projects answers GET /projects: the workspaces POST /new can target, in the
// host's own order (a client's first entry is its default).
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	list := s.projectListerFunc()
	if list == nil {
		http.Error(w, "this serve has no host that can list projects; bind a session in the desktop app",
			http.StatusNotImplemented)
		return
	}
	rows := list()
	out := make([]ProjectEntry, 0, len(rows))
	for _, row := range rows {
		row.Root = strings.TrimSpace(row.Root)
		if row.Root == "" {
			continue
		}
		out = append(out, row)
	}
	writeJSON(w, out)
}
