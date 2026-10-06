package control

// beginTurn advances the turn ordinal at the start of a turn. It must not sit behind
// the hook gate: when it did, a hook-free run left the counter at 0 and every
// turn-keyed sidecar record (recall, skill, outcome) collapsed onto 0.
func (c *Controller) beginTurn() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn++
	return c.turn
}
