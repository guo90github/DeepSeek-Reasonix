package serve

// SessionInfo is one session row an embedded host supplies for /sessions.
type SessionInfo struct {
	Name       string
	Path       string
	Title      string
	Turns      int
	Current    bool
	Running    bool
	TakenOver  bool
	MtimeMilli int64
}

// SetSessionLister lets an embedded host answer /sessions from its own index.
// Serve's own walk pays a lease probe and a preview parse per transcript, which
// is seconds of wall clock in a directory with hundreds of sessions.
func (s *Server) SetSessionLister(f func() []SessionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionLister = f
}

func (s *Server) sessionListerFunc() func() []SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionLister
}
