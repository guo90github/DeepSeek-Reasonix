package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

// NewSessionRequest is what POST /new may ask for: the workspace the new
// session must bind to. The zero value keeps the host's own default (the
// window's foreground scope), which is what a caller with no preference sends.
type NewSessionRequest struct {
	Scope       string `json:"scope,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
}

// decodeNewSessionRequest reads POST /new's optional target. An empty body and
// an empty object both mean "host default", so every pre-existing caller keeps
// working.
func decodeNewSessionRequest(r *http.Request) (NewSessionRequest, error) {
	var req NewSessionRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			return req, errors.New("bad body")
		}
	}
	req.Scope = strings.TrimSpace(req.Scope)
	req.ProjectRoot = strings.TrimSpace(req.ProjectRoot)
	switch req.Scope {
	case "":
		if req.ProjectRoot != "" {
			return req, errors.New("projectRoot needs scope=project")
		}
	case "global":
		req.ProjectRoot = ""
	case "project":
		if req.ProjectRoot == "" {
			return req, errors.New("projectRoot is required for scope=project")
		}
	default:
		return req, fmt.Errorf("unknown scope %q", req.Scope)
	}
	return req, nil
}

// SetSessionCreator installs the embedded host's new-session factory for
// POST /new. A host owns every controller this serve's foreground points at,
// so rotation belongs to it: only the host can open a session without demoting
// a controller that has a turn in flight.
func (s *Server) SetSessionCreator(f func(ctx context.Context, req NewSessionRequest) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionCreator = f
}

func (s *Server) sessionCreatorFunc() func(context.Context, NewSessionRequest) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionCreator
}

// newSessionFromHost publishes the session an embedded host just opened. The
// expected-session fence guards rotations of the session a caller displayed;
// opening a new one misroutes nothing, so a stale pin is not a refusal here.
func (s *Server) newSessionFromHost(w http.ResponseWriter, r *http.Request, req NewSessionRequest, create func(context.Context, NewSessionRequest) (string, error), emitNotice bool) {
	path, err := create(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	path = agent.CanonicalSessionPath(strings.TrimSpace(path))
	if path == "" {
		http.Error(w, "host opened no session", http.StatusConflict)
		return
	}
	s.bc.ResetSessionPath(path)
	w.Header().Set(sessionPathHeader, path)
	s.announceSessionChanged(path, true)
	if emitNotice {
		s.bc.Emit(event.Event{Kind: event.Notice, Text: "new session", SessionPath: path})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) planDecision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string                     `json:"id"`
		Action   control.PlanDecisionAction `json:"action"`
		Feedback string                     `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if err := s.ctl().ResolvePlanDecisionWithFeedback(body.ID, body.Action, body.Feedback); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	s.ctl().SetPlanMode(body.On)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCommand(w, r, false)
}

func (s *Server) clearSessionFromSubmit(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCommand(w, r, true)
}

func (s *Server) clearSessionCommand(w http.ResponseWriter, r *http.Request, emitNotice bool) {
	// Clear rotates the session path just like /new, but also removes the old
	// transcript artifacts. Keep controller mutation and lease rebinding under
	// one binding lock so remote clients never observe split ownership.
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if !s.validateSwitchExpectedLocked(w, r) {
		return
	}
	// A mirrored foreground cannot rotate in place (its write authority is
	// gone and its artifacts belong to the local writer). Publish a fresh
	// replacement instead, and keep the mirrored transcript on disk.
	if s.foregroundMirroredLocked() {
		s.mirroredForegroundReplacement(w, r, emitNotice, "cleared (mirrored session kept)")
		return
	}
	if err := s.ctl().ClearSession(); err != nil {
		if control.IsSessionRotationBusy(err) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ctrl, ok := s.ctl().(*control.Controller); ok {
		ctrl.EnsureSessionPath()
		s.setControllerPath(ctrl, ctrl.SessionPath())
	}
	s.bc.ResetSessionPath(s.ctl().SessionPath())
	if err := s.rebindSessionLease(s.ctl().SessionPath()); err != nil {
		http.Error(w, sessionInUseError(err), http.StatusConflict)
		return
	}
	path := s.ctl().SessionPath()
	w.Header().Set(sessionPathHeader, path)
	s.announceSessionChanged(path, true)
	if emitNotice {
		s.bc.Emit(event.Event{Kind: event.Notice, Text: "context cleared", SessionPath: path})
	}
	w.WriteHeader(http.StatusNoContent)
}

// mirroredForegroundReplacement swaps the open-but-released foreground
// controller for a fresh session. Used by /new and /clear when the current
// session was handed to a local writer: the outgoing controller cannot
// snapshot or rotate (its write authority is gone), so publish a replacement
// the same way a busy switch does and retire the old controller in the
// background. The mirrored transcript stays untouched on disk. Callers hold
// bindMu and pass the notice text for the newly created session.
func (s *Server) mirroredForegroundReplacement(w http.ResponseWriter, r *http.Request, emitNotice bool, noticeText string) {
	curCtrl, ok := s.ctl().(*control.Controller)
	if !ok {
		http.Error(w, "cannot rotate a mirrored session for this controller implementation", http.StatusConflict)
		return
	}
	if err := s.busyDetach(r.Context(), curCtrl, "", nil); err != nil {
		s.renderBindError(w, err)
		return
	}
	path := s.ctl().SessionPath()
	w.Header().Set(sessionPathHeader, path)
	s.announceSessionChanged(path, true)
	if emitNotice {
		s.bc.Emit(event.Event{Kind: event.Notice, Text: noticeText, SessionPath: path})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request) {
	s.newSessionCommand(w, r, false)
}

func (s *Server) newSessionFromSubmit(w http.ResponseWriter, r *http.Request) {
	s.newSessionCommand(w, r, true)
}

func (s *Server) newSessionCommand(w http.ResponseWriter, r *http.Request, emitNotice bool) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	// An embedded host owns the controllers this foreground resolves to, and it
	// decides what "new session" means there. Rotation below cannot serve that
	// case: a host controller carries no session tag to detach.
	if create := s.sessionCreatorFunc(); create != nil {
		req, err := decodeNewSessionRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.newSessionFromHost(w, r, req, create, emitNotice)
		return
	}
	if !s.validateSwitchExpectedLocked(w, r) {
		return
	}
	cur := s.ctl()
	if controllerHasActiveRuntimeWork(cur) {
		curCtrl, ok := cur.(*control.Controller)
		if !ok {
			http.Error(w, "cannot start a new session while active work or background jobs are running", http.StatusConflict)
			return
		}
		if err := s.busyDetach(r.Context(), curCtrl, "", nil); err != nil {
			s.renderBindError(w, err)
			return
		}
		path := s.ctl().SessionPath()
		w.Header().Set(sessionPathHeader, path)
		s.announceSessionChanged(path, true)
		if emitNotice {
			s.bc.Emit(event.Event{Kind: event.Notice, Text: "new session", SessionPath: path})
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// A mirrored foreground cannot NewSession() in place — the rotation
	// snapshots the old session first, which fails closed without write
	// authority. Publish a fresh replacement controller instead.
	if s.foregroundMirroredLocked() {
		s.mirroredForegroundReplacement(w, r, emitNotice, "new session")
		return
	}
	if err := cur.NewSession(); err != nil {
		if control.IsSessionRotationBusy(err) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ctrl, ok := cur.(*control.Controller); ok {
		ctrl.EnsureSessionPath()
		s.setControllerPath(ctrl, ctrl.SessionPath())
	}
	s.bc.ResetSessionPath(cur.SessionPath())
	if err := s.rebindSessionLease(cur.SessionPath()); err != nil {
		http.Error(w, sessionInUseError(err), http.StatusConflict)
		return
	}
	path := cur.SessionPath()
	w.Header().Set(sessionPathHeader, path)
	s.announceSessionChanged(path, true)
	if emitNotice {
		s.bc.Emit(event.Event{Kind: event.Notice, Text: "new session", SessionPath: path})
	}
	w.WriteHeader(http.StatusNoContent)
}
