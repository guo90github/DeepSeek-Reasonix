package control

import "strings"

// SetSessionEndObserver registers a first-party observer that runs after a
// session ends (closed or rotated) with the path that ended. A derived
// projection follows the close path through this seam instead of a frontend
// scraping controller state; hooks stay user-facing shell commands.
func (c *Controller) SetSessionEndObserver(observer func(reason, sessionPath string)) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.sessionEndObserver = observer
	c.mu.Unlock()
}

// notifySessionEnd reports one ended session. Empty paths are dropped: a topic
// archive clears the session path before closing, and such a session is not
// recappable.
func (c *Controller) notifySessionEnd(reason, sessionPath string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	observer := c.sessionEndObserver
	c.mu.Unlock()
	if observer == nil || strings.TrimSpace(sessionPath) == "" {
		return
	}
	observer(reason, sessionPath)
}
