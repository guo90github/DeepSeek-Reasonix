package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepoWithOneCommit seeds a real repository: /git-log is the host's own git
// read, so a stubbed source would prove nothing about what the phone receives.
func gitRepoWithOneCommit(t *testing.T, message string) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("-c", "user.name=Reasonix Test", "-c", "user.email=reasonix@example.invalid",
		"commit", "-q", "-m", message)
	return repo
}

// TestGitLogForRemoteReadsTheOfferedWorkspace pins the host half of the phone's
// 「最近提交」: the commits come from that workspace's own git through the same
// reader the desktop history panel uses, with every field the client renders.
func TestGitLogForRemoteReadsTheOfferedWorkspace(t *testing.T) {
	isolateDesktopUserDirs(t)
	repo := gitRepoWithOneCommit(t, "添加 OAuth 登录")
	rememberWorkspace(repo)

	rows, err := NewApp().gitLogForRemote(repo)
	if err != nil {
		t.Fatalf("gitLogForRemote: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("commits = %+v, want exactly the one commit", rows)
	}
	if rows[0].Message != "添加 OAuth 登录" {
		t.Fatalf("message = %q, want the commit subject", rows[0].Message)
	}
	if rows[0].Author != "Reasonix Test" || rows[0].Hash == "" || rows[0].Date == "" {
		t.Fatalf("commit = %+v, want author, hash and date populated", rows[0])
	}
}

// TestGitLogForRemoteRefusesWorkspacesItNeverOffered is the capability boundary:
// the caller names the root, so without the check an authenticated phone could
// read any repository on the desktop's disk by naming its path.
func TestGitLogForRemoteRefusesWorkspacesItNeverOffered(t *testing.T) {
	isolateDesktopUserDirs(t)
	repo := gitRepoWithOneCommit(t, "not offered to anyone")
	app := NewApp()

	if _, err := app.gitLogForRemote(repo); err == nil {
		t.Fatalf("gitLogForRemote(%q) succeeded, want a refusal", repo)
	}
	if _, err := app.gitLogForRemote(filepath.Dir(repo)); err == nil {
		t.Fatalf("gitLogForRemote(%q) succeeded, want a refusal", filepath.Dir(repo))
	}
}

// TestGitLogForRemoteSurfacesANonRepository covers the failure the phone must be
// able to tell apart from "no commits": a workspace without git has no history
// to report, and an empty list would render as "this workspace has no commits".
func TestGitLogForRemoteSurfacesANonRepository(t *testing.T) {
	isolateDesktopUserDirs(t)
	plain := t.TempDir()
	rememberWorkspace(plain)

	if _, err := NewApp().gitLogForRemote(plain); err == nil {
		t.Fatal("gitLogForRemote returned no error for a workspace without git")
	}
}
