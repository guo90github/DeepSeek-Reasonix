package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"reasonix/internal/filelock"
)

// LoadRegistry reads agents.json. A missing file is an empty registry, which is
// what a first run looks like.
func LoadRegistry(path string) (Registry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{SchemaVersion: RegistrySchemaVersion}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		return Registry{}, fmt.Errorf("agents.json: %w", err)
	}
	if reg.SchemaVersion != RegistrySchemaVersion {
		return Registry{}, fmt.Errorf("agents.json: schemaVersion %d is not %d", reg.SchemaVersion, RegistrySchemaVersion)
	}
	return reg, nil
}

// SaveRegistry replaces the registry while holding the file lock, so two
// managers cannot interleave read-modify-write. The write is not atomic: the
// lock is what serializes writers, and every record is recoverable by probing,
// so a torn write costs a rescan rather than data.
func SaveRegistry(path string, reg Registry) error {
	reg.SchemaVersion = RegistrySchemaVersion
	release, err := filelock.Acquire(context.Background(), path+".lock")
	if err != nil {
		return err
	}
	defer release()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Upsert replaces the record with the same name, or appends it.
func (r Registry) Upsert(rec Record) Registry {
	for i, existing := range r.Instances {
		if existing.Name == rec.Name {
			r.Instances[i] = rec
			return r
		}
	}
	r.Instances = append(r.Instances, rec)
	return r
}

// Remove drops a record by name and reports whether it was there.
func (r Registry) Remove(name string) (Registry, bool) {
	for i, existing := range r.Instances {
		if existing.Name == name {
			r.Instances = append(r.Instances[:i], r.Instances[i+1:]...)
			return r, true
		}
	}
	return r, false
}

// Find returns the record for a name.
func (r Registry) Find(name string) (Record, bool) {
	for _, existing := range r.Instances {
		if existing.Name == name {
			return existing, true
		}
	}
	return Record{}, false
}

// AllocateAddr returns the first loopback address in [FirstPort, LastPort] that
// is free right now. The probe releases the port immediately, so the launching
// serve's own bind is what actually reserves it; --port-file reports where it
// really landed, which is why the child's address is read back, not assumed.
func AllocateAddr() (string, error) {
	return AllocateAddrIn(FirstPort, LastPort)
}

// AllocateAddrIn is AllocateAddr with an explicit range, for tests.
func AllocateAddrIn(first, last int) (string, error) {
	for port := first; port <= last; port++ {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		_ = ln.Close()
		return addr, nil
	}
	return "", fmt.Errorf("agentd: no free port in %d-%d", first, last)
}

// ReadPortFile reads the address the serve child published after binding.
func ReadPortFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if addr := strings.TrimSpace(string(raw)); addr != "" {
		return addr, nil
	}
	return "", fmt.Errorf("agentd: %s is empty", filepath.Base(path))
}
