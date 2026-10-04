package serve

import (
	"net/http"
	"strings"
)

// GitCommit is one workspace commit a remote caller reads back. It carries no
// diff: the phone lists commits, and a diff is a separate request.
type GitCommit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// SetGitLogReader installs the embedded host's git reader for GET /git-log. A
// standalone serve has no filesystem the caller could be asking about.
func (s *Server) SetGitLogReader(f func(root string) ([]GitCommit, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gitLogReader = f
}

func (s *Server) gitLogReaderFunc() func(string) ([]GitCommit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gitLogReader
}

// gitLog answers GET /git-log?root=<absolute workspace root>: the workspace's
// recent commits, newest first. An empty root means the host's own foreground
// workspace, which is what a client that knows only its session path sends.
func (s *Server) gitLog(w http.ResponseWriter, r *http.Request) {
	read := s.gitLogReaderFunc()
	if read == nil {
		http.Error(w, "this serve has no host that can read a workspace; bind a session in the desktop app",
			http.StatusNotImplemented)
		return
	}
	rows, err := read(strings.TrimSpace(r.URL.Query().Get("root")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if rows == nil {
		rows = []GitCommit{}
	}
	writeJSON(w, rows)
}
