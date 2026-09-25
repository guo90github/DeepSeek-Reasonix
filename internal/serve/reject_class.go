package serve

import (
	"net/http"
	"strconv"
)

// The class names the caller's retry policy so no sender has to match the
// prose in the body: the three sources once shared one 409 with nothing to
// tell them apart (measured 2026-09-25 against a real chatting room).
const rejectClassHeader = "X-Reasonix-Reject-Class"

const (
	// The named session cannot be delivered to here: another runtime owns it,
	// or activation could not move the foreground onto it. Retrying cannot help.
	rejectTargetUnreachable = "target_unreachable"
	// The queue refuses right now (capacity, pause). Retrying may help; the
	// Retry-After header states the floor the host asks for.
	rejectNotAccepting = "not_accepting"
	// The request cannot be honored as sent: stale state, a consumed
	// idempotency key, a missing item. Retrying the same request cannot help.
	rejectInvalidRequest = "invalid_request"
)

// rejectRetryAfterSeconds is a floor, not a promise. The queue's own
// re-attempts run on a sub-second ladder, so a caller that waits this long
// cannot outrun them; the host cannot state when the queue will actually drain.
const rejectRetryAfterSeconds = 30

func reject(w http.ResponseWriter, class, message string) {
	w.Header().Set(rejectClassHeader, class)
	if class == rejectNotAccepting {
		w.Header().Set("Retry-After", strconv.Itoa(rejectRetryAfterSeconds))
	}
	http.Error(w, message, http.StatusConflict)
}
