// Package agentd runs one managed `reasonix serve` per workspace and keeps a
// registry of them, so a coordinator can wake a workspace's session instead of
// guessing which one is foreground. Durable state lives under the Reasonix
// home: agents.json plus one token file per instance.
package agentd

import (
	"path/filepath"
	"time"
)

// RegistrySchemaVersion gates reads of agents.json: an unknown version is
// refused rather than reinterpreted, so a newer manager cannot be mistaken for
// a corrupt file.
const RegistrySchemaVersion = 1

// FirstPort is where allocation starts: the desktop window's embedded serve
// owns 8787, and a managed instance must never fight it for the address.
const FirstPort = 8788

// LastPort bounds the scan so a saturated machine fails loudly instead of
// walking the whole range.
const LastPort = 8888

// State is an instance's lifecycle as the registry records it.
type State string

const (
	StateRunning     State = "running"
	StateStopped     State = "stopped"
	StateCircuitOpen State = "circuit-open"
)

// Record is one managed instance as agents.json stores it. Everything here is
// either supplied at spawn or read back from the process, never inferred.
type Record struct {
	Name        string    `json:"name"`
	Root        string    `json:"root"`
	Addr        string    `json:"addr"`
	PID         int       `json:"pid"`
	TokenFile   string    `json:"tokenFile"`
	SessionPath string    `json:"sessionPath,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	LastHealthy time.Time `json:"lastHealthy,omitempty"`
	State       State     `json:"state"`
}

// URL is the loopback base a coordinator posts to.
func (r Record) URL() string { return "http://" + r.Addr }

// Registry is the durable list of managed instances.
type Registry struct {
	SchemaVersion int      `json:"schemaVersion"`
	Instances     []Record `json:"instances"`
}

// RegistryPath is <home>/agents.json.
func RegistryPath(home string) string { return filepath.Join(home, "agents.json") }

// TokenPath is <home>/agents/<name>.token. One token per instance keeps a
// revoked workspace from affecting any other.
func TokenPath(home, name string) string {
	return filepath.Join(home, "agents", name+".token")
}

// LogPath is <home>/agents/<name>.log, where the child's output lands so a
// crash leaves evidence the manager itself never sees.
func LogPath(home, name string) string {
	return filepath.Join(home, "agents", name+".log")
}
