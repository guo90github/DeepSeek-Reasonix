package serve

// SetSubmitDelegate lets an embedded host own /submit, so a remote prompt enters
// the conversation the way the host's own composer sends it: rendered as a user
// turn and persisted. Serve's default (SubmitHTTPFormat) does neither, which
// leaves the window showing the reply with no prompt above it.
func (s *Server) SetSubmitDelegate(f func(input string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitDelegate = f
}

func (s *Server) submitDelegateFunc() func(string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.submitDelegate
}

// SetSubmitDelegateFor is the session-aware form of SetSubmitDelegate: the host
// receives the session the caller addressed (X-Reasonix-Session-Path) so a
// background wake lands in the conversation it belongs to instead of whichever
// tab happens to be foreground. Hosts that register only the plain form keep
// today's behaviour — the plain delegate is still the one that runs.
func (s *Server) SetSubmitDelegateFor(f func(sessionPath, input string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitDelegateFor = f
}

func (s *Server) submitDelegateForFunc() func(string, string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.submitDelegateFor
}

// submitViaHost routes an ordinary turn through the embedded host's composer
// path, which renders and records the user message; Serve's own
// SubmitHTTPFormat does neither. handled is false when no host is registered.
func (s *Server) submitViaHost(target, input string) (handled bool, err error) {
	if delegateFor := s.submitDelegateForFunc(); delegateFor != nil {
		return true, delegateFor(target, input)
	}
	if delegate := s.submitDelegateFunc(); delegate != nil {
		return true, delegate(input)
	}
	return false, nil
}
