package agent

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// retryReason names why a retry happened, so a host can tell a 429 storm from a flaky link
// while the retry itself is still invisible elsewhere (G6). It reads the retry's error —
// the only place the HTTP status survives — and an unclassified retry says nothing rather
// than something wrong.
func retryReason(err error) event.RetryReason {
	var api *provider.APIError
	if errors.As(err, &api) {
		switch {
		case api.Status == http.StatusTooManyRequests:
			return event.RetryReasonRateLimited
		case api.Status == http.StatusRequestTimeout:
			return event.RetryReasonTimeout
		case api.Status >= 500 && api.Status <= 599:
			return event.RetryReasonServer
		}
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return event.RetryReasonTimeout
	}
	if provider.IsConnReset(err) {
		return event.RetryReasonNetwork
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return event.RetryReasonNetwork
	}
	return ""
}

// retryReasonForFailure names why the stream phase is being retried, from the classification
// the recovery path already made: the HTTP status only exists there, and a 429 that arrives
// mid-stream is the same fact as one that arrives with the headers (G6).
func retryReasonForFailure(f provider.RecoveryFailure) event.RetryReason {
	switch {
	case f.Status == http.StatusTooManyRequests:
		return event.RetryReasonRateLimited
	case f.Status == http.StatusRequestTimeout:
		return event.RetryReasonTimeout
	case f.Status >= 500 && f.Status <= 599:
		return event.RetryReasonServer
	case f.Phase == "connect":
		return event.RetryReasonNetwork
	}
	return ""
}

// noteRetry records the one retry a host has to be able to count, and says it in the host's
// own log while nothing else about it is legible. No policy is invented here: the only
// decision taken is what to report (G6).
func noteRetry(reason event.RetryReason, attempt, max int, delay time.Duration) {
	if reason != event.RetryReasonRateLimited {
		return
	}
	slog.Warn("agent: provider rate limited this host, backing off",
		"attempt", attempt, "max", max, "delay_ms", delay.Milliseconds(),
		"rate_limited_total", provider.NoteRateLimitRetry())
}

// emitRetrying reports one header-phase retry, with its reason, to whoever is watching: the
// reason is what makes a 429 legible instead of one more "retrying (n/m)" (G6).
func emitRetrying(sink event.Sink, info provider.RetryInfo) {
	reason := retryReason(info.Err)
	sink.Emit(event.Event{Kind: event.Retrying, RetryAttempt: info.Attempt, RetryMax: info.Max, RetryScope: event.RetryScopeHeaders, RetryReason: reason})
	noteRetry(reason, info.Attempt, info.Max, info.Delay)
}
