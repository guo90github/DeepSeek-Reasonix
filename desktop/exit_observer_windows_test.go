//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// The observation exists for the exit the process cannot write down itself. Only
// a real child, terminated from outside, tests that honestly: the value asserted
// below is what Windows reports for a killed process, not what a stub returns.
func TestExitObserverReportsAnExternalKill(t *testing.T) {
	exitObservationDirForTest(t)

	cmd := exec.Command(os.Args[0], "-test.run=^TestExitObserverChildProcess$")
	cmd.Env = append(os.Environ(), exitObserverChildEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the child: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// The watcher has to open its handle while the process is still alive, which is
	// what makes the code readable after the kill; that ordering is the thing here.
	type observed struct {
		code  uint32
		known bool
	}
	reported := make(chan observed, 1)
	go func() {
		code, known := waitDesktopExitCode(pid)
		reported <- observed{code: code, known: known}
	}()
	time.Sleep(300 * time.Millisecond)

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill the child: %v", err)
	}
	_ = cmd.Wait()

	select {
	case got := <-reported:
		if !got.known {
			t.Fatal("a killed process must still have a reportable exit code")
		}
		if got.code == 0 {
			t.Fatalf("a killed process reported code 0, which is what a clean quit looks like")
		}
		note := desktopExitNote{RunID: "killed-run", Kind: exitKindRunning}
		observation := desktopExitObservation{PID: pid, ExitCode: got.code, CodeKnown: true}
		kind, reason := classifyDesktopRun(note, true, observation, true)
		if kind != exitKindKilled {
			t.Fatalf("an externally killed run classified as %q (%s)", kind, reason)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the watcher never reported the exit")
	}
}

const exitObserverChildEnv = "REASONIX_EXIT_OBSERVER_CHILD"

// TestExitObserverChildProcess is the process the test above kills. It is only a
// child when the environment says so.
func TestExitObserverChildProcess(t *testing.T) {
	if os.Getenv(exitObserverChildEnv) != "1" {
		t.Skip("helper process for the external-kill test")
	}
	time.Sleep(30 * time.Second)
}
