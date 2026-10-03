package serve

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

// A process on its way out retires the addresses it hosted: a wake sent to a session whose
// host is gone is accepted and then dropped, which is worse than having no address at all.
func TestWithdrawAgentBusRetiresEveryHostedAddress(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(t.TempDir(), "serve.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("token file: %v", err)
	}
	enrolled := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	enrolled.SetAgentBus(dir, "alice")
	if err := enrolled.AgentBusAnnounce("http://127.0.0.1:8977", tokenFile); err != nil {
		t.Fatalf("announce: %v", err)
	}
	// A hosted session that never joined the board is withdrawn too — as a no-op, not an
	// error: retiring an address that was never published is not a failure.
	unenrolled := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	s := &Server{tags: map[*control.Controller]*sessionTagSink{enrolled: nil, unenrolled: nil}}

	directory, err := agentbus.OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	if _, ok, err := directory.Lookup("alice"); err != nil || !ok {
		t.Fatalf("lookup before the host leaves = (%v, %v), want the announced address", ok, err)
	}

	s.WithdrawAgentBus()

	if _, ok, err := directory.Lookup("alice"); err != nil || ok {
		t.Fatalf("lookup after the host leaves = (%v, %v), want the address retired", ok, err)
	}
}
