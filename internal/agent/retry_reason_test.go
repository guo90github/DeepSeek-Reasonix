package agent

import (
	"context"
	"errors"
	"net/http"
	"syscall"
	"testing"

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
