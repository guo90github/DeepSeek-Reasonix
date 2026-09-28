package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The picker gets the global scope first and then every workspace the desktop
// remembers — a project with no session yet must still be selectable — with no
// workspace offered twice.
func TestListProjectsForRemoteOffersRememberedWorkspaces(t *testing.T) {
	isolateDesktopUserDirs(t)
	remembered := t.TempDir()
	rememberWorkspace(remembered)
	rememberWorkspace(remembered)

	app := NewApp()
	rows := app.listProjectsForRemote()
	if len(rows) < 2 {
		t.Fatalf("projects = %+v, want the global scope and the remembered workspace", rows)
	}
	if rows[0].Scope != "global" {
		t.Fatalf("first entry = %+v, want the global scope", rows[0])
	}
	wanted := strings.ToLower(filepath.Clean(remembered))
	seen := 0
	for _, row := range rows {
		if row.Root == "" || row.Name == "" {
			t.Fatalf("entry %+v is missing a root or a name", row)
		}
		if strings.ToLower(filepath.Clean(row.Root)) == wanted {
			seen++
			if row.Scope != "project" {
				t.Fatalf("remembered workspace scope = %q, want project", row.Scope)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("remembered workspace appears %d times, want exactly once (%+v)", seen, rows)
	}
}
