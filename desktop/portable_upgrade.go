// Portable self-upgrade — at a turn boundary the driver swaps current.json to a
// version a long task just built under versions/ and restarts into that
// version's own reasonix-desktop.exe, keeping the previous one as the rollback.

package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
	"reasonix/internal/installlayout"
)

const (
	portableUpgradeSchemaVersion = 1
	portableUpgradeStateFileName = "desktop-upgrade.json"
	// portableUpgradeReadyFileName is the marker a task writes inside a version
	// directory once its own verification passed. Without it the driver refuses
	// to switch: a half-extracted tree must never become the active version.
	portableUpgradeReadyFileName = ".reasonix-upgrade-ready.json"
	// portableUpgradeRollbackGrace is how long a pending switch may sit before a
	// shell that cannot reach its service rolls it back.
	portableUpgradeRollbackGrace = 10 * time.Minute
)

// portableVersionRE mirrors the layout's version directory names.
var portableVersionRE = regexp.MustCompile(`^v[0-9]+(?:\.[0-9]+){1,3}(?:-[0-9A-Za-z.-]+)?$`)

type portableUpgradeState struct {
	SchemaVersion int    `json:"schemaVersion"`
	Phase         string `json:"phase"` // pending | healthy
	From          string `json:"from,omitempty"`
	To            string `json:"to"`
	ActivatedAt   string `json:"activatedAt,omitempty"`
}

type portableUpgradeReady struct {
	Ready bool `json:"ready"`
}

// portableUpgradeStatePathFunc is a seam for tests; production resolves the user
// state directory, which the Electron shell reads too.
var portableUpgradeStatePathFunc = defaultPortableUpgradeStatePath

func portableUpgradeStatePath() string { return portableUpgradeStatePathFunc() }

func defaultPortableUpgradeStatePath() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, portableUpgradeStateFileName)
}

func readPortableUpgradeState() (portableUpgradeState, bool) {
	path := portableUpgradeStatePath()
	if path == "" {
		return portableUpgradeState{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return portableUpgradeState{}, false
	}
	var state portableUpgradeState
	if err := json.Unmarshal(body, &state); err != nil {
		return portableUpgradeState{}, false
	}
	if state.SchemaVersion > portableUpgradeSchemaVersion {
		return portableUpgradeState{}, false
	}
	return state, true
}

func writePortableUpgradeState(state portableUpgradeState) error {
	path := portableUpgradeStatePath()
	if path == "" {
		return errors.New("no user state directory")
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = portableUpgradeSchemaVersion
	}
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, body, 0o600)
}

// portableInstallRoot returns the install root that owns current.json, or "" for
// an install this feature does not manage (dev builds, flat layouts).
func portableInstallRoot() string { return portableInstallRootFunc() }

// portableInstallRootFunc is a seam for tests; production locates the root from
// the running executable.
var portableInstallRootFunc = defaultPortableInstallRoot

func defaultPortableInstallRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	root, err := installlayout.ResolveInstallRoot(exe)
	if err != nil || root == "" || !installlayout.HasCurrent(root) {
		return ""
	}
	return root
}

// portableRunningVersion is the version directory this binary was started from.
func portableRunningVersion() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Base(filepath.Dir(exe))
	if !portableVersionRE.MatchString(dir) {
		return ""
	}
	return dir
}

// comparePortableVersions orders two version labels: -1, 0 or 1. Numeric
// segments win over anything else, and a release outranks its own pre-release
// (v1.2.0 > v1.2.0-dev.9), which is what "版本号递增" means on disk.
func comparePortableVersions(a, b string) int {
	an, ap := splitPortableVersion(a)
	bn, bp := splitPortableVersion(b)
	for i := 0; i < len(an) || i < len(bn); i++ {
		var av, bv int
		if i < len(an) {
			av = an[i]
		}
		if i < len(bn) {
			bv = bn[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	if ap == bp {
		return 0
	}
	if ap == "" {
		return 1
	}
	if bp == "" {
		return -1
	}
	return comparePortablePrerelease(ap, bp)
}

// comparePortablePrerelease orders pre-release tails segment by segment: a
// numeric segment compares as a number ("dev.9" < "dev.10"), an alphanumeric one
// lexically, and a numeric segment ranks below an alphanumeric one.
func comparePortablePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if i >= len(as) {
			return -1
		}
		if i >= len(bs) {
			return 1
		}
		av, aerr := strconv.Atoi(as[i])
		bv, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if av != bv {
				if av < bv {
					return -1
				}
				return 1
			}
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		default:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

func splitPortableVersion(version string) ([]int, string) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")
	core, pre, _ := strings.Cut(trimmed, "-")
	parts := strings.Split(core, ".")
	nums := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			n = 0
		}
		nums = append(nums, n)
	}
	return nums, pre
}

// portableVersionReady reports whether a version directory may be activated: it
// must carry the task's own ready marker and hold a complete tree.
func portableVersionReady(root, version string) bool {
	if !portableVersionRE.MatchString(version) {
		return false
	}
	dir := filepath.Join(root, installlayout.VersionsDirName, version)
	if !isRegularFile(filepath.Join(dir, portableUpgradeReadyFileName)) {
		return false
	}
	var marker portableUpgradeReady
	if body, err := os.ReadFile(filepath.Join(dir, portableUpgradeReadyFileName)); err != nil {
		return false
	} else if err := json.Unmarshal(body, &marker); err != nil || !marker.Ready {
		return false
	}
	return portableVersionComplete(root, version)
}

func portableVersionComplete(root, version string) bool {
	dir := filepath.Join(root, installlayout.VersionsDirName, version)
	if info, err := os.Stat(filepath.Join(dir, installlayout.AppShellDirName)); err != nil || !info.IsDir() {
		return false
	}
	for _, name := range append(installlayout.AllowedVersionMembers(), installlayout.DesktopBinaryName()) {
		if !isRegularFile(filepath.Join(dir, name)) {
			return false
		}
	}
	return true
}

func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// portableUpgradeTarget picks what to switch to: a current.json already pointing
// at another complete version wins (the task may have swapped it itself), and
// otherwise the newest ready version newer than the running one.
func portableUpgradeTarget(root, running string) (string, bool) {
	if ptr, err := installlayout.ReadCurrent(root); err == nil {
		active := strings.TrimSpace(ptr.ActiveVersion)
		if active != "" && active != running && portableVersionComplete(root, active) {
			return active, true
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, installlayout.VersionsDirName))
	if err != nil {
		return "", false
	}
	best := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if version == running || !portableVersionReady(root, version) {
			continue
		}
		if comparePortableVersions(version, running) <= 0 {
			continue
		}
		if best == "" || comparePortableVersions(version, best) > 0 {
			best = version
		}
	}
	return best, best != ""
}

// portableUpgradeRelaunchPath is the single launch entry of a version: its own
// inner reasonix-desktop.exe.
func portableUpgradeRelaunchPath(root, version string) string {
	return filepath.Join(root, installlayout.VersionsDirName, version, installlayout.DesktopBinaryName())
}

// activatePortableUpgrade publishes the pointer swap and records the rollback
// target before anything restarts.
func activatePortableUpgrade(root, from, to string) error {
	ptr := installlayout.CurrentPointer{
		SchemaVersion: 1,
		ActiveVersion: to,
		ActiveDir:     installlayout.VersionDirRelative(to),
	}
	if err := installlayout.WriteCurrent(root, ptr); err != nil {
		return err
	}
	return writePortableUpgradeState(portableUpgradeState{
		SchemaVersion: portableUpgradeSchemaVersion,
		Phase:         "pending",
		From:          from,
		To:            to,
		ActivatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// markPortableUpgradeHealthy closes a pending switch once this process is the
// version it asked for.
func markPortableUpgradeHealthy() bool {
	state, ok := readPortableUpgradeState()
	if !ok || state.Phase != "pending" {
		return false
	}
	running := portableRunningVersion()
	if running == "" || running != state.To {
		return false
	}
	state.Phase = "healthy"
	if err := writePortableUpgradeState(state); err != nil {
		log.Printf("[upgrade] could not record a healthy start: %v", err)
		return false
	}
	log.Printf("[upgrade] version %s started after the switch from %s", state.To, state.From)
	return true
}

// maybeRelaunchForUpgrade switches versions and restarts into the new one. It
// runs at a turn boundary of any task while unattended driving is on, so the
// interrupted work is the one the next process picks up from its Goal.
func (e *HeartbeatEngine) maybeRelaunchForUpgrade(t *HeartbeatTask) bool {
	if t == nil || !e.unattendedEnabled() {
		return false
	}
	root := portableInstallRoot()
	if root == "" {
		return false
	}
	running := portableRunningVersion()
	target, ok := portableUpgradeTarget(root, running)
	if !ok {
		return false
	}
	if err := activatePortableUpgrade(root, running, target); err != nil {
		log.Printf("[upgrade] unattended %q could not activate %s: %v", t.Title, target, err)
		return false
	}
	exe := portableUpgradeRelaunchPath(root, target)
	log.Printf("[upgrade] unattended %q switching %s -> %s, restarting %s", t.Title, running, target, exe)
	if err := e.app.relaunchIntoVersion(exe); err != nil {
		log.Printf("[upgrade] unattended %q could not restart into %s: %v", t.Title, target, err)
		return false
	}
	return true
}
