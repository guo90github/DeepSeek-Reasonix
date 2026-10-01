//go:build windows

package main

import "golang.org/x/sys/windows"

// desktopExitObserverAvailable reports whether this platform can hand a watcher
// the exit code of a process it does not own. Windows can, by handle.
func desktopExitObserverAvailable() bool { return true }

// waitDesktopExitCode blocks until the process is gone and returns the code the
// OS recorded. Opening the handle while the process is alive is what makes the
// code readable afterwards; a watcher that started too late gets nothing.
func waitDesktopExitCode(pid int) (uint32, bool) {
	handle, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(handle)
	if _, err := windows.WaitForSingleObject(handle, windows.INFINITE); err != nil {
		return 0, false
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return 0, false
	}
	if code == desktopStillActiveExitCode {
		return 0, false
	}
	return code, true
}
