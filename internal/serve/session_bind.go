// Session binding: moving Serve's foreground onto another session file. Split
// out of serve.go, which is at its size ceiling.
package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/control"
)

// resume loads a previous session from a JSONL file.
func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	realPath, err := s.resolveSessionPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), resolveSessionPathStatus(err))
		return
	}
	// An embedded host already owns a controller for every session its UI
	// shows: /resume is its routing decision, not Serve's, or the transcript
	// would gain a second writer.
	if activate := s.sessionActivatorFunc(); activate != nil {
		if err := activate(realPath); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set(sessionPathHeader, agent.CanonicalSessionPath(realPath))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// A mirrored or foreign-held session belongs to a local runtime: mount the
	// client as a read-only spectator rather than switching onto Serve's frozen
	// copy. The reclaim path lives in session_ownership.go.
	if s.sessionMirrored(realPath) || leaseHeldByForeignRuntime(realPath) {
		w.Header().Set(sessionPathHeader, agent.CanonicalSessionPath(realPath))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Serialize with /new, /fork, and switchModel so the controller and lease
	// cannot land on different sessions. Validate first to avoid slow holders.
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	s.resumeSession(w, r, realPath)
}

// resolveSessionPathStatus keeps resume's historical status codes for the
// shared validation helper.
func resolveSessionPathStatus(err error) int {
	if err != nil && err.Error() == "path outside session dir" {
		return http.StatusForbidden
	}
	return http.StatusBadRequest
}

// resumeSession moves the foreground to realPath. Callers hold bindMu.
func (s *Server) resumeSession(w http.ResponseWriter, r *http.Request, realPath string) {
	cur := s.ctl()
	if s.resumeActiveSession(w, r, cur, realPath) {
		return
	}
	// Snapshot the current session before switching away — while this process
	// still holds its lease (skipped when a local writer owns it).
	s.snapshotForeground(cur)
	// Refuse to bind a session another runtime is writing (a desktop window,
	// another CLI); on success the lease now guards the resume target.
	if s.leases != nil {
		if err := s.leases.Rebind(realPath); err != nil {
			if errors.Is(err, agent.ErrSessionLeaseHeld) {
				http.Error(w, sessionInUseError(err), http.StatusConflict)
			} else {
				http.Error(w, "session lease: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	loaded, err := agent.LoadSession(realPath)
	if err != nil {
		// The lease already moved to the target; re-point it at the session the
		// controller still owns (best-effort).
		_ = s.rebindSessionLease(cur.SessionPath())
		http.Error(w, "load session: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.commitLoadedResume(w, cur, loaded, realPath) {
		return
	}
	s.bc.ResetSessionPath(realPath)
	s.announceSessionChanged(realPath, false)
	w.WriteHeader(http.StatusNoContent)
	s.replayPendingPromptsBroadcast()
}

// SetForegroundProvider installs the host's foreground resolver. Serve's own
// foreground tracking stays authoritative whenever the provider answers false.
func (s *Server) SetForegroundProvider(p func() (control.SessionAPI, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.foreground = p
}

// SetSessionActivator installs the host's session router for /resume.
func (s *Server) SetSessionActivator(f func(path string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionActivator = f
}

func (s *Server) sessionActivatorFunc() func(string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionActivator
}

// activateSubmitTarget brings the session a remote caller addressed to the
// foreground so admission and the composer path act on it. Callers hold bindMu;
// a refusal is already written to w.
func (s *Server) activateSubmitTarget(w http.ResponseWriter, r *http.Request) (string, bool) {
	target := agent.CanonicalSessionPath(strings.TrimSpace(r.Header.Get(sessionPathHeader)))
	if target == "" || agent.CanonicalSessionPath(strings.TrimSpace(s.ctl().SessionPath())) == target {
		return target, true
	}
	activate := s.sessionActivatorFunc()
	if activate == nil {
		http.Error(w, "session is not the foreground one and this host cannot switch to it", http.StatusConflict)
		return target, false
	}
	if err := activate(target); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return target, false
	}
	return target, true
}
