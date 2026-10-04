package control

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus"
)

// A roster record nobody rewrites goes stale and the participant silently drops off it. The
// tick renews presence once the record has aged past half its TTL, and leaves it alone while it
// is still fresh — an address book rewritten every tick is churn, not presence
// (F57, 2026-10-05).
func TestTheTickRenewsPresenceOnlyOnceItHasAged(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "bob")
	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatal(err)
	}
	directory, err := agentbus.OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh, found, err := directory.Lookup("bob")
	if err != nil || !found {
		t.Fatalf("lookup = (%+v, %t, %v), want the announcement", fresh, found, err)
	}

	// A tick right after announcing leaves the record alone.
	c.refreshAgentBusPresence(ctx, fresh.At.Add(time.Minute))
	if again, _, _ := directory.Lookup("bob"); !again.At.Equal(fresh.At) {
		t.Fatalf("a fresh record was rewritten: %s -> %s", fresh.At, again.At)
	}

	// Once it has aged past half the TTL, the tick renews it.
	c.refreshAgentBusPresence(ctx, fresh.At.Add(agentbus.ParticipantTTL/2+time.Minute))
	renewed, found, err := directory.Lookup("bob")
	if err != nil || !found {
		t.Fatalf("lookup after refresh = (%+v, %t, %v)", renewed, found, err)
	}
	if !renewed.At.After(fresh.At) {
		t.Fatalf("presence was not renewed: %s", renewed.At)
	}
}
