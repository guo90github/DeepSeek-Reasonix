package agent

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"syscall"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// The reason is what a host records and what makes a 429 storm legible: an unattended run
// that only says "retrying (3/10)" cannot tell a rate limit from a flaky link (G6), and a
// wrong guess is worse than none, so anything unclassified stays empty.
func TestRetryReasonNamesTheRetriesAHostMustTellApart(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want event.RetryReason
	}{
		{"a rate limit", &provider.APIError{Status: http.StatusTooManyRequests}, event.RetryReasonRateLimited},
		{"a request timeout", &provider.APIError{Status: http.StatusRequestTimeout}, event.RetryReasonTimeout},
		{"an overloaded provider", &provider.APIError{Status: http.StatusServiceUnavailable}, event.RetryReasonServer},
		{"a non-standard server fault", &provider.APIError{Status: 529}, event.RetryReasonServer},
		{"a dropped connection", syscall.ECONNRESET, event.RetryReasonNetwork},
		{"a deadline this host hit", context.DeadlineExceeded, event.RetryReasonTimeout},
		{"a wrapped rate limit", errors.Join(errors.New("send"), &provider.APIError{Status: http.StatusTooManyRequests}), event.RetryReasonRateLimited},
		{"a client error retrying cannot fix", &provider.APIError{Status: http.StatusBadRequest}, ""},
		{"an unclassified transient", errors.New("temporary"), ""},
		{"no error at all", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryReason(tc.err); got != tc.want {
				t.Fatalf("retryReason(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// retryLogHandler captures the host log so a test reads what an operator would.
type retryLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *retryLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *retryLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *retryLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *retryLogHandler) WithGroup(string) slog.Handler      { return h }

func (h *retryLogHandler) lines() []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]map[string]string, 0, len(h.records))
	for _, r := range h.records {
		line := map[string]string{"msg": r.Message}
		r.Attrs(func(a slog.Attr) bool {
			line[a.Key] = a.Value.String()
			return true
		})
		out = append(out, line)
	}
	return out
}

// A multi-session run has to be able to say *which lane* was throttled: the process total
// answers "is this happening", not "where". The provider instance and its trace id are what
// the error already carries, so reporting them invents nothing — and a field it does not
// carry stays absent (G6).
func TestTheRateLimitLogNamesTheLaneThatThrottled(t *testing.T) {
	handler := &retryLogHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })

	throttled := &provider.APIError{
		Provider: "deepseek", ProviderDisplayName: "DeepSeek", Protocol: "openai",
		Status: http.StatusTooManyRequests, TraceID: "trace-429-1",
	}
	noteRetry(event.RetryReasonRateLimited, provider.RetryInfo{Attempt: 1, Max: 3, Delay: 2 * time.Second, Err: throttled})

	lines := handler.lines()
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want the one a throttled host writes", len(lines))
	}
	line := lines[0]
	for key, want := range map[string]string{
		"provider": "deepseek", "protocol": "openai", "trace_id": "trace-429-1",
		"attempt": "1", "max": "3", "delay_ms": "2000",
	} {
		if got := line[key]; got != want {
			t.Fatalf("%s = %q, want %q (log line: %+v)", key, got, want, line)
		}
	}
	if line["rate_limited_total"] == "" {
		t.Fatalf("the running total has to stay on the line: %+v", line)
	}

	// A retry that is not a rate limit stays quiet, and one without a provider in the error
	// logs the plain line instead of guessing a lane.
	handler.records = nil
	noteRetry(event.RetryReasonServer, provider.RetryInfo{Attempt: 1, Max: 3, Err: throttled})
	if got := handler.lines(); len(got) != 0 {
		t.Fatalf("a non-rate-limit retry logged %+v, want silence", got)
	}
	handler.records = nil
	noteRetry(event.RetryReasonRateLimited, provider.RetryInfo{Attempt: 2, Max: 3, Err: errors.New("no provider here")})
	lines = handler.lines()
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want the plain one", len(lines))
	}
	for _, key := range []string{"provider", "protocol", "trace_id"} {
		if got := lines[0][key]; got != "" {
			t.Fatalf("%s = %q, want it absent when the error does not carry it", key, got)
		}
	}
}
