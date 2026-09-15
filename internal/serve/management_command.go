package serve

import (
	"net/http"
	"strings"
)

func isServeManagementCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return false
	}
	switch strings.ToLower(fields[0]) {
	case "/compact", "/context", "/new", "/clear", "/goal", "/memory", "/remember",
		"/migrate", "/migration", "/skill", "/skills", "/plugin", "/plugins",
		"/reload-cmd", "/hooks", "/mcp", "/provider", "/tree", "/branch",
		"/switch", "/rewind":
		return true
	default:
		return false
	}
}

// handleSubmitCommand runs the slash commands Serve intercepts before a turn.
// The caller holds bindMu: session rotations must complete under it, because
// Controller.Submit would dispatch a following verb across the rotation.
func (s *Server) handleSubmitCommand(w http.ResponseWriter, r *http.Request, trimmed string) bool {
	if strings.HasPrefix(trimmed, "!") {
		http.Error(w, "shell commands are unavailable over HTTP", http.StatusForbidden)
		return true
	}
	switch trimmed {
	case "/new":
		s.newSessionFromSubmit(w, r)
		return true
	case "/clear":
		s.clearSessionFromSubmit(w, r)
		return true
	}
	// The controller's Submit path only lists models; switching is frontend-specific.
	if s.submitModelCommand(w, r, trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, "/effort ") {
		level := strings.TrimSpace(strings.TrimPrefix(trimmed, "/effort"))
		if level != "" {
			if err := s.switchEffortExpected(r.Context(), level, r.Header.Get(expectedSessionPathHeader)); err != nil {
				http.Error(w, err.Error(), runtimeSwitchErrorStatus(err))
				return true
			}
			w.WriteHeader(http.StatusNoContent)
			return true
		}
	}
	return false
}
