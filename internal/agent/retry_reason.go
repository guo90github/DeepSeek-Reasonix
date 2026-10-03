package agent

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

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

// noteRetry records the one retry a host has to be able to count, and says it in the host's own
// log while nothing else about it is legible. A multi-session run needs the lane, not only the
// process total: the log names the provider instance that throttled, and its trace id when the
// response carried one. No policy is invented here — the only decision is what to report (G6).
func noteRetry(reason event.RetryReason, info provider.RetryInfo) {
	if reason != event.RetryReasonRateLimited {
		return
	}
	attrs := []any{
		"attempt", info.Attempt, "max", info.Max, "delay_ms", info.Delay.Milliseconds(),
		"rate_limited_total", provider.NoteRateLimitRetry(),
	}
	if id, protocol, trace := retryAttribution(info.Err); id != "" {
		attrs = append(attrs, "provider", id, "protocol", protocol)
		if trace != "" {
			attrs = append(attrs, "trace_id", trace)
		}
	}
	slog.Warn("agent: provider rate limited this host, backing off", attrs...)
}

// retryAttribution names the lane that throttled, when the error says: which provider instance,
// which protocol, and the provider's trace id for a support case. A field the error does not
// carry stays absent — an unnamed lane beats a guessed one (G6).
func retryAttribution(err error) (providerID, protocol, traceID string) {
	var api *provider.APIError
	if err == nil || !errors.As(err, &api) || api == nil {
		return "", "", ""
	}
	return strings.TrimSpace(api.Provider), strings.TrimSpace(api.Protocol), strings.TrimSpace(api.TraceID)
}

// emitRetrying reports one header-phase retry, with its reason, to whoever is watching: the
// reason is what makes a 429 legible instead of one more "retrying (n/m)" (G6).
func emitRetrying(sink event.Sink, info provider.RetryInfo) {
	reason := retryReason(info.Err)
	sink.Emit(event.Event{Kind: event.Retrying, RetryAttempt: info.Attempt, RetryMax: info.Max, RetryScope: event.RetryScopeHeaders, RetryReason: reason})
	noteRetry(reason, info)
}
