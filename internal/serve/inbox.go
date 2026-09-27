package serve

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/sessioninbox"
)

func (s *Server) registerInboxRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /inbox", s.inboxList)
	mux.HandleFunc("GET /inbox/gate", s.inboxGate)
	mux.HandleFunc("GET /inbox/receipt", s.inboxReceipt)
	mux.HandleFunc("GET /inbox/room-line", s.inboxRoomLine)
	mux.HandleFunc("POST /inbox/items", s.foregroundMutation(s.inboxEnqueue))
	mux.HandleFunc("GET /inbox/items/{id}", s.inboxGet)
	mux.HandleFunc("PATCH /inbox/items/{id}", s.foregroundMutation(s.inboxUpdate))
	mux.HandleFunc("DELETE /inbox/items/{id}", s.foregroundMutation(s.inboxDelete))
	mux.HandleFunc("POST /inbox/move", s.foregroundMutation(s.inboxMove))
	mux.HandleFunc("POST /inbox/pause", s.foregroundMutation(s.inboxPause))
	mux.HandleFunc("POST /inbox/resume", s.foregroundMutation(s.inboxResume))
	mux.HandleFunc("POST /inbox/items/{id}/retry", s.foregroundMutation(s.inboxRetry))
	mux.HandleFunc("POST /inbox/items/{id}/refresh", s.foregroundMutation(s.inboxRefresh))
}

func (s *Server) inboxAPI() control.SessionAPI {
	return s.ctl()
}

func writeInboxError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sessioninbox.ErrItemTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge) // 413
	case errors.Is(err, sessioninbox.ErrCapacityItems), errors.Is(err, sessioninbox.ErrCapacityBytes),
		errors.Is(err, sessioninbox.ErrPaused):
		reject(w, rejectNotAccepting, err.Error()) // 409
	case errors.Is(err, sessioninbox.ErrInvalidState), errors.Is(err, sessioninbox.ErrNotFound),
		errors.Is(err, sessioninbox.ErrIdempotencyConflict):
		reject(w, rejectInvalidRequest, err.Error()) // 409
	case errors.Is(err, sessioninbox.ErrEmpty):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) inboxList(w http.ResponseWriter, r *http.Request) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if !s.validateInboxReadSessionLocked(w, r) {
		return
	}
	snap := s.inboxAPI().InboxSnapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

// inboxGate answers the hold on this queue read-only: what is keeping queued
// lines from running right now, without a seq and without posting anything. A
// wake's receipt carries the same gate, but only for a line the sender already
// pushed — this is how a room asks whether it is being held at all.
func (s *Server) inboxGate(w http.ResponseWriter, r *http.Request) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if !s.validateInboxReadSessionLocked(w, r) {
		return
	}
	reader, ok := s.inboxAPI().(interface {
		InboxGate() control.InboxGate
	})
	if !ok {
		http.Error(w, "this host cannot answer the queue gate", http.StatusNotImplemented)
		return
	}
	writeJSON(w, reader.InboxGate())
}

// inboxRoomLine answers what this session holds for one room line, by the seq the
// room prints. A push wake has no reply channel, so a sender asks here instead of
// assuming the line landed.
func (s *Server) inboxRoomLine(w http.ResponseWriter, r *http.Request) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if !s.validateInboxReadSessionLocked(w, r) {
		return
	}
	seq, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("seq")), 10, 64)
	if err != nil || seq <= 0 {
		http.Error(w, "seq must be a positive integer", http.StatusBadRequest)
		return
	}
	lookup, ok := s.inboxAPI().(interface {
		InboxRoomLineFor(int64) (control.InboxRoomLine, bool)
	})
	if !ok {
		http.Error(w, "this host cannot answer room lines", http.StatusNotImplemented)
		return
	}
	line, found := lookup.InboxRoomLineFor(seq)
	w.Header().Set("Content-Type", "application/json")
	// A line that already left the queue is still an answer: found stays false, and
	// the ending it carries is what separates "ran, then was cancelled" from "never
	// took it in". Dropping it here is how that distinction died on the wire before.
	if !found && line.Settled == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"found": false})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"found": found, "line": line})
}

// validateInboxReadSessionLocked keeps legacy unscoped reads compatible while
// fencing modern Desktop reads against a concurrent foreground replacement.
func (s *Server) validateInboxReadSessionLocked(w http.ResponseWriter, r *http.Request) bool {
	if !s.validateExpectedSessionLocked(w, r) {
		return false
	}
	if err := s.expectedSessionPathErrorLocked(r.URL.Query().Get("session")); err != nil {
		reject(w, rejectTargetUnreachable, err.Error())
		return false
	}
	return true
}

func (s *Server) inboxEnqueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Input          string                      `json:"input"`
		Display        string                      `json:"display"`
		Invocations    []control.InvocationRequest `json:"invocations"`
		Intent         string                      `json:"intent"`
		IdempotencyKey string                      `json:"idempotencyKey"`
		// Optional and additive: a producer that omits it queues as before.
		Seq string `json:"seq"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Input) == "" {
		http.Error(w, "missing input", http.StatusBadRequest)
		return
	}
	intent := sessioninbox.IntentFollowup
	if strings.EqualFold(body.Intent, "steer") {
		intent = sessioninbox.IntentSteer
	}
	// An addressed wake lands in the session it names or is refused. Answering
	// 202 after landing in the foreground would hide a misdelivery from the
	// sender; only a wake that named no session may use the foreground.
	requested := agent.CanonicalSessionPath(strings.TrimSpace(r.Header.Get(sessionPathHeader)))
	if requested != "" {
		if err := s.activateAddressedSession(requested); err != nil {
			reject(w, rejectTargetUnreachable, "addressed session cannot receive this here: "+err.Error())
			return
		}
	} else if intent == sessioninbox.IntentSteer {
		slog.Warn("inbox: steer without a session path; it lands in the foreground session", "source", "http")
	}
	api := s.inboxAPI()
	if ensurer, ok := any(api).(interface{ EnsureSessionPath() }); ok {
		ensurer.EnsureSessionPath()
	}
	landed := agent.CanonicalSessionPath(strings.TrimSpace(s.ctl().SessionPath()))
	if requested != "" && landed != requested {
		// The address could not be honored even though activation reported
		// success: refuse before admitting anything rather than deliver elsewhere.
		slog.Warn("inbox: addressed session is not the delivery target; refusing instead of misdelivering",
			"requested", requested, "landed", landed)
		reject(w, rejectTargetUnreachable, "addressed session cannot receive this here")
		return
	}
	roomExtra := roomSeqExtra(body.Seq)
	req := control.InboxRequest{
		Intent:      intent,
		Display:     body.Display,
		Raw:         body.Input,
		Submit:      body.Input,
		Source:      control.WakeSourceFor(roomExtra, "http"),
		Idempotency: body.IdempotencyKey,
		Invocations: body.Invocations,
		Extra:       roomExtra,
	}
	if req.Display == "" {
		req.Display = body.Input
	}
	var rec sessioninbox.InboxReceipt
	var err error
	if intent == sessioninbox.IntentSteer {
		rec, err = api.TryEnqueueAndSteer(req)
	} else {
		rec, err = api.TryEnqueueFollowup(req)
	}
	if err != nil {
		writeInboxError(w, err)
		return
	}
	rec.SessionPath = landed
	rec.RequestedSessionPath = requested
	w.Header().Set(sessionPathHeader, landed)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(rec)
}

// roomSeqExtra carries the chat room's line number when the producer names one,
// under the key the boot wake handler already writes, so RoomMeta assembles
// identically on both routes. An unusable seq stays absent: the item is then a
// plain http wake rather than one claiming a line nobody can reconcile.
func roomSeqExtra(raw string) map[string]string {
	seq, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || seq <= 0 {
		return nil
	}
	return map[string]string{"room.seq": strconv.FormatInt(seq, 10)}
}

func (s *Server) inboxReceipt(w http.ResponseWriter, r *http.Request) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if !s.validateInboxReadSessionLocked(w, r) {
		return
	}
	ctrl := s.ctl()
	reader, ok := ctrl.(interface {
		LookupInboxReceipt(string) (sessioninbox.InboxReceipt, bool, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	receipt, found, err := reader.LookupInboxReceipt(r.URL.Query().Get("key"))
	if err != nil {
		writeInboxError(w, err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, receipt)
}

func (s *Server) inboxGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	meta, env, err := s.inboxAPI().ReadInboxItem(id)
	if err != nil {
		writeInboxError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta, "envelope": env})
}

func (s *Server) inboxUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Input) == "" {
		http.Error(w, "missing input", http.StatusBadRequest)
		return
	}
	meta, err := s.inboxAPI().UpdateInboxItem(id, body.Input, body.Input, body.Input)
	if err != nil {
		writeInboxError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

func (s *Server) inboxDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.inboxAPI().DeleteInboxItem(id); err != nil {
		writeInboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inboxMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		ToIndex int    `json:"toIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if err := s.inboxAPI().MoveInboxItem(body.ID, body.ToIndex); err != nil {
		writeInboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inboxPause(w http.ResponseWriter, r *http.Request) {
	_ = r
	if err := s.inboxAPI().SetInboxPaused(true); err != nil {
		writeInboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inboxResume(w http.ResponseWriter, r *http.Request) {
	_ = r
	if err := s.inboxAPI().SetInboxPaused(false); err != nil {
		writeInboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inboxRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.inboxAPI().RetryInboxItem(id); err != nil {
		writeInboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inboxRefresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.inboxAPI().RefreshInboxReferences(id); err != nil {
		writeInboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
