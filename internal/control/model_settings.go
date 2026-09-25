package control

// controllerModelSettings keeps the immutable snapshot and its host admission
// callback together for the lifetime of one controller. The callback, its last
// answer, and anything else mutated after construction are guarded by
// Controller.mu; the snapshot fields are immutable.
type controllerModelSettings struct {
	revision            string
	sourceRevision      string
	current             func() (string, error)
	beforeInboxDispatch func(*Controller) (func(), error)
	// inboxHostRefusal is the host's last refusal, kept next to the hook so a
	// receipt for that item can relay why instead of only naming the gate.
	inboxHostRefusal *inboxHostRefusal
}

func newControllerModelSettings(opts Options) controllerModelSettings {
	return controllerModelSettings{
		revision:            opts.ModelSettingsRevision,
		sourceRevision:      opts.ModelSettingsSourceRevision,
		current:             opts.ModelSettingsCurrent,
		beforeInboxDispatch: opts.BeforeInboxDispatch,
	}
}
