package event

// RetryScope distinguishes connection+header retries, body-phase stream
// retries, and host-classified protocol recovery. Older clients ignore an
// unknown value and still render the generic retry state.
type RetryScope string

const (
	RetryScopeHeaders  RetryScope = "headers"
	RetryScopeStream   RetryScope = "stream"
	RetryScopeProtocol RetryScope = "protocol"
)

// RetryReason is why a retry is happening, in the closed set a host records. It is empty
// for an emitter that does not classify, and an unknown value is ignored, so a 429 storm
// can be counted separately from a flaky link without inventing a throttling policy (G6).
type RetryReason string

const (
	RetryReasonRateLimited RetryReason = "rate_limited" // 429: the provider asked this host to slow down
	RetryReasonServer      RetryReason = "server_error" // 5xx: the provider is unwell, not this host
	RetryReasonTimeout     RetryReason = "timeout"      // 408 or a deadline the host hit
	RetryReasonNetwork     RetryReason = "network"      // connection reset, dial failure
)

// RecoveryStatus is a local UI projection, never provider-visible metadata.
type RecoveryStatus struct {
	// State is the durable tool-recovery state (for example recovery_required).
	// It is local UI metadata and never provider-visible.
	State                string `json:"state,omitempty"`
	CallID               string `json:"call_id,omitempty"`
	AttemptID            string `json:"attempt_id,omitempty"`
	RequiresUserDecision bool   `json:"requires_user_decision,omitempty"`
	ReadOnly             bool   `json:"read_only,omitempty"`
	Phase                string `json:"phase,omitempty"`
	Reason               string `json:"reason,omitempty"`
	NextAttemptAt        int64  `json:"next_attempt_at,omitempty"`
	WaitedMs             int64  `json:"waited_ms,omitempty"`
	WaitBudgetMs         int64  `json:"wait_budget_ms,omitempty"`
	Waiting              bool   `json:"waiting,omitempty"`
}
