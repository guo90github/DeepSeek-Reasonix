package browser

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// loggingExecutor adds the optional Logs capability to the fake.
type loggingExecutor struct {
	*fakeExecutor
	console []ConsoleEntry
	network []NetworkEntry
	reqs    []LogsRequest
}

func (l *loggingExecutor) Console(_ context.Context, req LogsRequest) ([]ConsoleEntry, error) {
	l.reqs = append(l.reqs, req)
	return l.console, nil
}

func (l *loggingExecutor) Network(_ context.Context, req LogsRequest) ([]NetworkEntry, error) {
	l.reqs = append(l.reqs, req)
	return l.network, nil
}

func TestConsoleReadsTheNewestEntries(t *testing.T) {
	exec := &loggingExecutor{fakeExecutor: &fakeExecutor{}, console: []ConsoleEntry{
		{Level: "error", Text: "boom"},
		{Level: "warning", Text: "slow"},
	}}
	out, err := run(t, exec, "browser_console", `{"tabId":"t1","limit":5}`)
	if err != nil {
		t.Fatalf("console: %v", err)
	}
	if !strings.Contains(out, "2 console entr(y/ies) for tab t1") || !strings.Contains(out, "error: boom") || !strings.Contains(out, "warning: slow") {
		t.Fatalf("console out = %q", out)
	}
	if !reflect.DeepEqual(exec.reqs, []LogsRequest{{TabID: "t1", Limit: 5}}) {
		t.Fatalf("console requests = %+v", exec.reqs)
	}
}

func TestConsoleSaysWhyItIsEmpty(t *testing.T) {
	out, err := run(t, &loggingExecutor{fakeExecutor: &fakeExecutor{}}, "browser_console", `{"tabId":"t1"}`)
	if err != nil {
		t.Fatalf("console: %v", err)
	}
	if !strings.Contains(out, "no console output recorded") || !strings.Contains(out, "recording starts") {
		t.Fatalf("console out = %q", out)
	}
}

func TestNetworkPrintsStatusMimeAndFailure(t *testing.T) {
	exec := &loggingExecutor{fakeExecutor: &fakeExecutor{}, network: []NetworkEntry{
		{Method: "POST", URL: "https://a.test/api", Status: 500, MIME: "application/json"},
		{Method: "GET", URL: "https://a.test/x", Failed: "net::ERR_FAILED"},
	}}
	out, err := run(t, exec, "browser_network", `{"tabId":"t1"}`)
	if err != nil {
		t.Fatalf("network: %v", err)
	}
	for _, want := range []string{
		"2 request(s) for tab t1",
		"POST https://a.test/api 500 application/json",
		"GET https://a.test/x failed: net::ERR_FAILED",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("network out = %q, missing %q", out, want)
		}
	}
}

func TestLogsAreUnavailableWithoutTheCapability(t *testing.T) {
	if _, err := run(t, &fakeExecutor{}, "browser_console", `{"tabId":"t1"}`); err == nil || !strings.Contains(err.Error(), "console output is unavailable") {
		t.Fatalf("console err = %v", err)
	}
	if _, err := run(t, &fakeExecutor{}, "browser_network", `{"tabId":"t1"}`); err == nil || !strings.Contains(err.Error(), "network traffic is unavailable") {
		t.Fatalf("network err = %v", err)
	}
}
