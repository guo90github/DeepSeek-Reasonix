package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// seedPortableInstall writes a versioned install root with one published version
// per argument. ready marks which of them carry the task's ready marker.
func seedPortableInstall(t *testing.T, ready map[string]bool) (root string, active string) {
	t.Helper()
	root = t.TempDir()
	versions := filepath.Join(root, "versions")
	names := make([]string, 0, len(ready))
	for name := range ready {
		names = append(names, name)
	}
	for _, name := range names {
		dir := filepath.Join(versions, name)
		if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		for _, member := range []string{"reasonix-desktop.exe", "reasonix-cli.exe", "reasonix-update-helper.exe"} {
			if err := os.WriteFile(filepath.Join(dir, member), []byte(member), 0o755); err != nil {
				t.Fatalf("write %s: %v", member, err)
			}
		}
		if ready[name] {
			body, err := json.Marshal(portableUpgradeReady{Ready: true})
			if err != nil {
				t.Fatalf("marshal ready marker: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, portableUpgradeReadyFileName), body, 0o644); err != nil {
				t.Fatalf("write ready marker: %v", err)
			}
		}
	}
	return root, versions
}

func pointCurrentAt(t *testing.T, root, version string, schema int) {
	t.Helper()
	body := []byte(`{"schemaVersion":` + itoa(schema) + `,"activeVersion":"` + version + `","activeDir":"versions/` + version + `"}`)
	if err := os.WriteFile(filepath.Join(root, "current.json"), body, 0o644); err != nil {
		t.Fatalf("write current.json: %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestComparePortableVersionsOrdersNumericSegmentsAndPrerelease(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.0.0-dev.91", "v0.0.0-dev.91", 0},
		{"v0.0.0-dev.92", "v0.0.0-dev.91", 1},
		{"v0.0.0-dev.9", "v0.0.0-dev.10", -1},
		{"v0.0.1-dev.1", "v0.0.0-dev.99", 1},
		{"v1.2.0", "v1.2.0-dev.9", 1},
		{"v1.2.0-dev.9", "v1.2.0", -1},
		{"v1.2.0", "v1.10.0", -1},
	}
	for _, tc := range cases {
		if got := comparePortableVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPortableUpgradeTargetNeedsAReadyMarker(t *testing.T) {
	root, _ := seedPortableInstall(t, map[string]bool{"v0.0.0-dev.91": false, "v0.0.0-dev.92": true})
	// current.json still points at the running version.
	pointCurrentAt(t, root, "v0.0.0-dev.90", 1)
	target, ok := portableUpgradeTarget(root, "v0.0.0-dev.90")
	if !ok || target != "v0.0.0-dev.92" {
		t.Fatalf("target = (%q, %v), want the ready newer version", target, ok)
	}
	// The unready version must never be picked, even alone.
	root2, _ := seedPortableInstall(t, map[string]bool{"v0.0.0-dev.91": false})
	pointCurrentAt(t, root2, "v0.0.0-dev.90", 1)
	if target, ok := portableUpgradeTarget(root2, "v0.0.0-dev.90"); ok {
		t.Fatalf("target = %q, want none: a tree without the ready marker is not switchable", target)
	}
}

func TestPortableUpgradeTargetPrefersTheNewestReadyVersion(t *testing.T) {
	root, _ := seedPortableInstall(t, map[string]bool{
		"v0.0.0-dev.91": true,
		"v0.0.0-dev.92": true,
		"v0.0.0-dev.93": true,
	})
	pointCurrentAt(t, root, "v0.0.0-dev.90", 1)
	target, ok := portableUpgradeTarget(root, "v0.0.0-dev.90")
	if !ok || target != "v0.0.0-dev.93" {
		t.Fatalf("target = (%q, %v), want the greatest ready version", target, ok)
	}
}

func TestPortableUpgradeTargetHonoursAPointerTheTaskAlreadyMoved(t *testing.T) {
	// The task extracted and pointed current.json itself, without a ready marker.
	root, _ := seedPortableInstall(t, map[string]bool{"v0.0.0-dev.92": false})
	pointCurrentAt(t, root, "v0.0.0-dev.92", 1)
	target, ok := portableUpgradeTarget(root, "v0.0.0-dev.91")
	if !ok || target != "v0.0.0-dev.92" {
		t.Fatalf("target = (%q, %v), want the version the pointer names", target, ok)
	}
}

func TestPortableUpgradeTargetIgnoresOlderAndRunningVersions(t *testing.T) {
	root, _ := seedPortableInstall(t, map[string]bool{
		"v0.0.0-dev.89": true,
		"v0.0.0-dev.90": true,
	})
	pointCurrentAt(t, root, "v0.0.0-dev.90", 1)
	if target, ok := portableUpgradeTarget(root, "v0.0.0-dev.90"); ok {
		t.Fatalf("target = %q, want none: nothing newer than the running version", target)
	}
}

func TestPortableVersionCompleteRejectsAHalfExtractedTree(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "versions", "v0.0.0-dev.95")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if portableVersionComplete(root, "v0.0.0-dev.95") {
		t.Fatal("a version directory without its shell tree and binaries is not complete")
	}
	if err := os.WriteFile(filepath.Join(dir, "reasonix-desktop.exe"), []byte("x"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if portableVersionComplete(root, "v0.0.0-dev.95") {
		t.Fatal("a version directory missing its siblings is not complete")
	}
}

func TestPortableUpgradeStateRoundTripAndHealthiness(t *testing.T) {
	stateDir := t.TempDir()
	previous := portableUpgradeStatePathFunc
	portableUpgradeStatePathFunc = func() string { return filepath.Join(stateDir, portableUpgradeStateFileName) }
	t.Cleanup(func() { portableUpgradeStatePathFunc = previous })

	if _, ok := readPortableUpgradeState(); ok {
		t.Fatal("no switch has happened yet")
	}
	if err := writePortableUpgradeState(portableUpgradeState{
		SchemaVersion: portableUpgradeSchemaVersion,
		Phase:         "pending",
		From:          "v0.0.0-dev.91",
		To:            "v0.0.0-dev.92",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	state, ok := readPortableUpgradeState()
	if !ok || state.Phase != "pending" || state.To != "v0.0.0-dev.92" {
		t.Fatalf("state = %+v, want a pending switch", state)
	}
	// A future schema is not ours to interpret.
	if err := os.WriteFile(portableUpgradeStatePath(), []byte(`{"schemaVersion":99,"phase":"pending","to":"v9"}`), 0o600); err != nil {
		t.Fatalf("write future state: %v", err)
	}
	if _, ok := readPortableUpgradeState(); ok {
		t.Fatal("a future schema must read as no switch")
	}
}

func TestPortableUpgradeRelaunchPathIsTheInnerDesktop(t *testing.T) {
	got := portableUpgradeRelaunchPath(filepath.Join("root"), "v0.0.0-dev.92")
	want := filepath.Join("root", "versions", "v0.0.0-dev.92", "reasonix-desktop.exe")
	if got != want {
		t.Fatalf("relaunch path = %q, want %q", got, want)
	}
}

func TestMaybeRelaunchForUpgradeNeedsOnlyTheSwitch(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	if engine.maybeRelaunchForUpgrade(nil) {
		t.Fatal("no task, no version switch")
	}
	// The switch is the gate, not the Goal: a plain scheduled task may carry it
	// too — this host simply has no versioned install to switch.
	if engine.maybeRelaunchForUpgrade(&HeartbeatTask{ID: "t"}) {
		t.Fatal("there is no versioned install to switch in this test")
	}
	engine.unattended = false
	if engine.maybeRelaunchForUpgrade(&HeartbeatTask{ID: "t", Goal: "ship"}) {
		t.Fatal("the master switch is off: nothing restarts the app")
	}
}
