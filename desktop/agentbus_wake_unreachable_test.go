package main

import (
	"strings"
	"testing"
)

// The row must not read as a delivery failure: those participants are not failing to receive, they
// are not here at all, because a board outlives the sessions that wrote to it.
func TestWakeUnreachableSignalNamesWhoIsNotHere(t *testing.T) {
	if _, row := wakeUnreachableSignal(nil); row {
		t.Fatal("a host with every participant reachable must add no row")
	}
	departed := "20261003-081116.799903600-deepseek-deepseek-flash"
	signal, row := wakeUnreachableSignal([]string{"bob", departed})
	if !row {
		t.Fatal("work waiting on participants this host cannot reach must be visible")
	}
	if signal.Kind != "wake_unreachable" {
		t.Fatalf("kind = %q, want the unreachable row rather than the failure row", signal.Kind)
	}
	for _, want := range []string{"2", "bob", departed} {
		if !strings.Contains(signal.Detail, want) {
			t.Fatalf("detail = %q, want it to name %q", signal.Detail, want)
		}
	}
}
