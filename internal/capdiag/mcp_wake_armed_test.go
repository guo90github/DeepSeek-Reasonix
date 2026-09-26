package capdiag_test

import (
	"path/filepath"
	"testing"

	"reasonix/internal/capdiag"
)

// Waking an idle session is host-only policy: a server may start work here only
// when the config declares the notification it may use. Diagnostics have to say
// which servers can, otherwise a closed wake channel is visible only in
// behavior — a mention that arrives as nothing at all.
func TestCollectReportsWakeArmedServers(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
	write(t, filepath.Join(root, "reasonix.toml"), `
[[plugins]]
name = "room"
type = "stdio"
command = "room.exe"
wake_method = "notifications/room/room_message"

[[plugins]]
name = "quiet"
type = "stdio"
command = "quiet.exe"
`)

	r := capdiag.Collect(capdiag.Options{
		Root:            root,
		HomeDir:         home,
		ReasonixHomeDir: filepath.Join(home, ".reasonix"),
	})

	byName := map[string]capdiag.MCPServerInfo{}
	for _, s := range r.MCP.Servers {
		byName[s.Name] = s
	}
	room, ok := byName["room"]
	if !ok {
		t.Fatalf("room server missing from %+v", r.MCP.Servers)
	}
	if !room.WakeArmed || room.WakeMethod != "notifications/room/room_message" {
		t.Fatalf("room wake = %+v, want the declared method reported as armed", room)
	}
	quiet := byName["quiet"]
	if quiet.WakeArmed || quiet.WakeMethod != "" {
		t.Fatalf("quiet wake = %+v, want an unarmed server reported as unarmed", quiet)
	}
}
