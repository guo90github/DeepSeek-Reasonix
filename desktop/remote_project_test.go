package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/serve"
)

// The hook's happy path opens a real tab and needs a booted window, so these
// cover the half that decides whether a remote caller's request may proceed.
func TestCreateProjectForRemoteRejectsUnusablePaths(t *testing.T) {
	var a App
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-folder.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		req  serve.NewProjectRequest
		want string
	}{
		{"missing folder", serve.NewProjectRequest{Root: filepath.Join(dir, "missing")}, "open project folder"},
		{"path is a file", serve.NewProjectRequest{Root: file}, "not a directory"},
		{"folder name missing", serve.NewProjectRequest{Parent: dir}, "project name is required"},
		{"parent missing", serve.NewProjectRequest{Name: "demo"}, "parent folder is required"},
		{"separator in name", serve.NewProjectRequest{Parent: dir, Name: "a/b"}, "single folder name"},
	}
	for _, tc := range cases {
		_, err := a.createProjectForRemote(tc.req)
		if err == nil {
			t.Errorf("%s: the request was accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %q, want it to mention %q", tc.name, err, tc.want)
		}
	}
}
