package serve

import (
	"encoding/json"
	"net/http"
	"strings"
)

// NewProjectRequest is what a caller that cannot browse this machine's
// filesystem can say: an absolute path that already exists, or a parent folder
// plus a folder name to create below it.
type NewProjectRequest struct {
	Root   string `json:"root"`
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

// NewProjectResult names the registered project and the session the caller
// should switch to; every later remote call addresses that session path.
type NewProjectResult struct {
	Root        string `json:"root"`
	Name        string `json:"name,omitempty"`
	SessionPath string `json:"sessionPath"`
}

// SetProjectCreator installs the embedded host's project factory for
// POST /new-project. Only a host can answer it: registering a workspace means
// opening it in the window and giving it a session, and a standalone serve has
// no window to open it in — the foreground workspace root is fixed for the life
// of the process.
func (s *Server) SetProjectCreator(f func(NewProjectRequest) (NewProjectResult, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectCreator = f
}

func (s *Server) projectCreatorFunc() func(NewProjectRequest) (NewProjectResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projectCreator
}

func (s *Server) newProject(w http.ResponseWriter, r *http.Request) {
	create := s.projectCreatorFunc()
	if create == nil {
		http.Error(w, "this serve has no host that can create projects; create it in the desktop app",
			http.StatusNotImplemented)
		return
	}
	var req NewProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	req.Root, req.Parent, req.Name = strings.TrimSpace(req.Root), strings.TrimSpace(req.Parent), strings.TrimSpace(req.Name)
	switch {
	case req.Root == "" && (req.Parent == "" || req.Name == ""):
		http.Error(w, "root, or parent and name, is required", http.StatusBadRequest)
		return
	case req.Root != "" && (req.Parent != "" || req.Name != ""):
		http.Error(w, "pass either root or parent with name, not both", http.StatusBadRequest)
		return
	}
	result, err := create(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if result.SessionPath == "" {
		http.Error(w, "host registered no session for the project", http.StatusConflict)
		return
	}
	writeJSON(w, result)
}
