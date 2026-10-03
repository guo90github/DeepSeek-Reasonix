package main

import "testing"

// The switch and this launch's real state are two facts: a crash-degraded launch keeps driving
// off while the switch stays on, so the watchdog still pulls the host back up. The panel has to
// be able to tell them apart — a toggle reading "on" while nothing ran is a promise with nothing
// behind it (2026-10-04).
func TestUnattendedDrivingSeparatesTheSwitchFromThisLaunch(t *testing.T) {
	if driving, hold := unattendedDriving(false, false); driving || hold != "" {
		t.Fatalf("switch off = (%v, %q), want no driving and no hold", driving, hold)
	}
	if driving, hold := unattendedDriving(true, false); !driving || hold != "" {
		t.Fatalf("switch on and healthy = (%v, %q), want driving and no hold", driving, hold)
	}
	driving, hold := unattendedDriving(true, true)
	if driving {
		t.Fatal("a crash-degraded launch must not drive")
	}
	if hold == "" {
		t.Fatal("a held launch must say why, or the panel has nothing to show")
	}
}
