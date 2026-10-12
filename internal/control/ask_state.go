package control

import "reasonix/internal/event"

// registerQueuedAskPrompt registers an ask and commits the runtime snapshot, so
// the queued count is visible the moment a question waits behind another prompt
// instead of only at the next lifecycle boundary.
func (c *Controller) registerQueuedAskPrompt(id string, kind PromptKind) {
	c.registerOwnedPrompt(id, kind)
	c.refreshRuntimeState(event.Event{})
}

// markAskEmitted clears the queued flag and re-commits the snapshot, so the
// state surface stops counting a question the moment it reaches the frontend.
func (c *Controller) markAskEmitted(id string) {
	c.approval.markAskEmitted(id)
	c.refreshRuntimeState(event.Event{})
}
