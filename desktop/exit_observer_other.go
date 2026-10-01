//go:build !windows

package main

// desktopExitObserverAvailable reports whether this platform can hand a watcher
// the exit code of a process it does not own. Only Windows can, by handle, so the
// other platforms spawn no watcher at all rather than one that reports nothing.
func desktopExitObserverAvailable() bool { return false }

func waitDesktopExitCode(int) (uint32, bool) { return 0, false }
