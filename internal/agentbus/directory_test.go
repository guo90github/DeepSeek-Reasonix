package agentbus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryAnnouncesLooksUpAndReplacesAddresses(t *testing.T) {
	ctx := context.Background()
	d, err := OpenParticipantDirectory(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	first := ParticipantRef{Participant: "bob", Host: "http://127.0.0.1:7331", SessionPath: "/sessions/bob.jsonl"}
	if _, err := d.Announce(ctx, first); err != nil {
		t.Fatalf("announce: %v", err)
	}
	got, ok, err := d.Lookup("bob")
	if err != nil || !ok {
		t.Fatalf("lookup = (%v, %v), want the announced address", ok, err)
	}
	if got.Host != first.Host || got.SessionPath != first.SessionPath {
		t.Fatalf("lookup = %+v, want %+v", got, first)
	}

	// The same session on another host replaces its own address, not somebody else's.
	moved := ParticipantRef{Participant: "bob", Host: "http://10.0.0.5:7331", SessionPath: "/srv/bob.jsonl"}
	if _, err := d.Announce(ctx, moved); err != nil {
		t.Fatalf("re-announce: %v", err)
	}
	if _, err := d.Announce(ctx, ParticipantRef{Participant: "alice", Host: "http://127.0.0.1:7331"}); err != nil {
		t.Fatalf("announce alice: %v", err)
	}
	if got, _, _ := d.Lookup("bob"); got.Host != moved.Host || got.SessionPath != moved.SessionPath {
		t.Fatalf("lookup after a move = %+v, want %+v", got, moved)
	}
	all, err := d.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 2 || all[0].Participant != "alice" || all[1].Participant != "bob" {
		t.Fatalf("all = %+v, want both participants sorted", all)
	}

	// Leaving the board must not leave an address behind that still gets woken.
	if err := d.Withdraw(ctx, "bob"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if _, ok, err := d.Lookup("bob"); ok || err != nil {
		t.Fatalf("lookup after withdrawal = (%v, %v), want absent", ok, err)
	}
	if all, _ := d.All(); len(all) != 1 || all[0].Participant != "alice" {
		t.Fatalf("all = %+v, want only the participant that stayed", all)
	}
}

func TestDirectoryRefusesAddressesItCouldNeverDeliver(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := d.Announce(ctx, ParticipantRef{Host: "http://127.0.0.1:7331"}); err == nil {
		t.Fatal("an announcement without a participant has no address to keep")
	}
	reason, ok := IsDirectoryReject(refuseOf(d.Announce(ctx, ParticipantRef{Participant: "bob"})))
	if ok {
		t.Fatalf("an announcement without a token is legitimate (local board), got %q", reason)
	}
	missing := filepath.Join(dir, "no-such-token")
	reason, ok = IsDirectoryReject(refuseOf(d.Announce(ctx, ParticipantRef{Participant: "bob", TokenFile: missing})))
	if !ok || reason != RefuseDirectoryTokenNotFile {
		t.Fatalf("a token that is not a file = (%v, %q), want it refused at announce time", ok, reason)
	}
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	if _, err := d.Announce(ctx, ParticipantRef{
		Participant: "bob", Host: "http://127.0.0.1:7331", SessionPath: "/sessions/bob.jsonl", TokenFile: token,
	}); err != nil {
		t.Fatalf("announce with a real token file: %v", err)
	}

	// A fresh reader sees exactly what was announced: the address book is durable.
	reader, err := OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok, err := reader.Lookup("bob")
	if err != nil || !ok {
		t.Fatalf("reopen lookup = (%v, %v)", ok, err)
	}
	if got.TokenFile != token {
		t.Fatalf("token file = %q, want %q", got.TokenFile, token)
	}
}

// refuseOf adapts Announce's two-value result to the error the reject reader wants.
func refuseOf(_ ParticipantRef, err error) error { return err }
