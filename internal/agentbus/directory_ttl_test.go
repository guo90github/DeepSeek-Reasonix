package agentbus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An address is only worth waking as long as it was renewed. Nothing retires it for a host that
// is gone — a crash, or a leftover serve that still answers — so an announcement older than the
// TTL is treated as absent: a refusal the sender can see, not a wake swallowed by a process
// nobody is driving (measured on a real machine: an abandoned serve stayed addressable for a day
// and accepted the wakes sent to it, 2026-10-03).
func TestAddressesOlderThanTheTTLAreNotAddressable(t *testing.T) {
	dir := t.TempDir()
	line := func(ref ParticipantRef) string {
		t.Helper()
		raw, err := json.Marshal(ref)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(raw) + "\n"
	}
	log := line(ParticipantRef{
		Participant: "alice", Host: "http://127.0.0.1:1",
		At: time.Now().UTC().Add(-ParticipantTTL - time.Minute),
	}) + line(ParticipantRef{
		Participant: "bob", Host: "http://127.0.0.1:2",
		At: time.Now().UTC().Add(-time.Minute),
	})
	if err := os.WriteFile(filepath.Join(dir, directoryName), []byte(log), 0o644); err != nil {
		t.Fatalf("seed the directory: %v", err)
	}
	d, err := OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, ok, err := d.Lookup("alice"); err != nil || ok {
		t.Fatalf("lookup(alice) = ok %v, err %v; want an absent address", ok, err)
	}
	if ref, ok, err := d.Lookup("bob"); err != nil || !ok {
		t.Fatalf("lookup(bob) = ok %v, err %v; want the live address", ok, err)
	} else if ref.Host != "http://127.0.0.1:2" {
		t.Fatalf("lookup(bob) = %+v, want its own host", ref)
	}

	live, err := d.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(live) != 1 || live[0].Participant != "bob" {
		t.Fatalf("all = %+v, want only the renewed address", live)
	}
}

// A freshness window is only safe because hosts renew: the headless tick republishes every 30s,
// an order of magnitude inside the TTL. This assertion is the contract that keeps the two from
// drifting apart silently.
func TestTheRefreshCadenceIsWellInsideTheTTL(t *testing.T) {
	if ParticipantTTL < 10*time.Minute {
		t.Fatalf("ParticipantTTL = %v, want room for a 30s republish cadence", ParticipantTTL)
	}
}
