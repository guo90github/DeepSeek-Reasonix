package agentd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agent"
)

const (
	probeInterval = 15 * time.Second
	probeTimeout  = 5 * time.Second
	launchWait    = 45 * time.Second
	readyPoll     = 250 * time.Millisecond
	// shutdownWait outlasts the serve's own drain (10s), so a retirement that
	// does land is never cut short by the manager's own deadline.
	shutdownWait = 20 * time.Second
)

// Manager owns the managed instances of one Reasonix home: it allocates their
// addresses, starts them, keeps the registry current, and restarts what died.
type Manager struct {
	home     string
	registry string

	mu    sync.Mutex
	live  map[string]*Process
	watch map[string]*watchState

	client *http.Client
	now    func() time.Time

	// launchWait, shutdownWait and backoff are the seams tests drive: the window
	// a child gets to answer /status, the window a retiring instance gets to
	// exit on its own, and the delay between restart attempts.
	launchWait   time.Duration
	shutdownWait time.Duration
	backoff      func(int) time.Duration
}

// NewManager manages the instances recorded under home.
func NewManager(home string) *Manager {
	return &Manager{
		home:         home,
		registry:     RegistryPath(home),
		live:         map[string]*Process{},
		watch:        map[string]*watchState{},
		client:       &http.Client{Timeout: probeTimeout},
		now:          func() time.Time { return time.Now().UTC() },
		launchWait:   launchWait,
		shutdownWait: shutdownWait,
		backoff:      backoffDelay,
	}
}

// Up starts one instance and waits for its /status, so the caller gets either a
// reachable address or an error naming what went wrong.
func (m *Manager) Up(ctx context.Context, spec Spec) (Record, error) {
	spec, err := m.normalize(spec)
	if err != nil {
		return Record{}, err
	}
	if err := refuseHeldSession(spec.SessionPath); err != nil {
		return Record{}, err
	}
	if _, err := ensureToken(spec.TokenFile); err != nil {
		return Record{}, err
	}
	m.mu.Lock()
	live := m.live[spec.Name]
	retiring := live != nil && live.retiring
	m.mu.Unlock()
	switch {
	case retiring:
		return Record{}, fmt.Errorf("agentd: %s is being retired; retry once it stops", spec.Name)
	case live != nil:
		return Record{}, fmt.Errorf("agentd: %s is already managed by this process", spec.Name)
	}
	managed, err := Spawn(ctx, spec)
	if err != nil {
		return Record{}, err
	}
	if err := m.awaitReady(ctx, managed); err != nil {
		managed.Stop()
		return Record{}, err
	}
	rec := managed.Record
	// The child's own bind is authoritative: a raced port probe would otherwise
	// record an address nobody listens on.
	if addr, err := ReadPortFile(spec.PortFile); err == nil && addr != "" {
		rec.Addr = addr
		managed.Record.Addr = addr
	}
	rec.LastHealthy = m.now()
	m.mu.Lock()
	m.live[rec.Name] = managed
	m.mu.Unlock()
	if err := m.save(func(reg Registry) Registry { return reg.Upsert(rec) }); err != nil {
		return rec, err
	}
	return rec, nil
}

// Down stops one instance. The name stays managed until the retirement lands, so
// a concurrent Up is told what is happening instead of racing the session lease;
// the registry record leaves first, because a sweep restarts whatever its own
// snapshot still lists.
func (m *Manager) Down(name string) error {
	m.mu.Lock()
	managed := m.live[name]
	switch {
	case managed == nil:
	case managed.retiring:
		// Another call is already retiring it; the stop it asked for is in flight.
		m.mu.Unlock()
		return nil
	default:
		managed.retiring = true
	}
	m.mu.Unlock()
	releaseClaim := func() {
		m.mu.Lock()
		if claimed := m.live[name]; claimed != nil {
			claimed.retiring = false
		}
		m.mu.Unlock()
	}

	reg, err := LoadRegistry(m.registry)
	if err != nil {
		releaseClaim()
		return err
	}
	_, known := reg.Find(name)
	if managed == nil && !known {
		return fmt.Errorf("agentd: %s is not managed here", name)
	}
	pruned, _ := reg.Remove(name)
	saveErr := SaveRegistry(m.registry, pruned)
	m.retire(managed)
	m.mu.Lock()
	delete(m.live, name)
	delete(m.watch, name)
	m.mu.Unlock()
	return saveErr
}

// retire stops one instance in the order that loses no work: ask it to end its
// turn and then itself, and kill the tree only when it does not go in time. A
// serve killed mid-turn loses that turn; a serve that is asked writes it out.
func (m *Manager) retire(managed *Process) {
	if managed == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.shutdownWait)
	defer cancel()
	if err := managed.Retire(ctx, m.client); err != nil {
		slog.Warn("agentd: retiring by request failed; killing the tree", "instance", managed.Record.Name, "err", err)
		managed.Stop()
	}
}

// StopAll kills every instance this manager started, which is what a
// supervising `agents up` does on Ctrl-C: the supervisor is leaving, so its
// kill-on-close job ends them anyway and draining would only stall the interrupt.
func (m *Manager) StopAll() {
	m.mu.Lock()
	live := m.live
	m.live = map[string]*Process{}
	m.watch = map[string]*watchState{}
	m.mu.Unlock()
	for _, managed := range live {
		managed.Stop()
	}
	reg, err := LoadRegistry(m.registry)
	if err != nil {
		return
	}
	pruned := Registry{SchemaVersion: RegistrySchemaVersion}
	for _, rec := range reg.Instances {
		if _, stillLive := live[rec.Name]; stillLive {
			continue
		}
		pruned.Instances = append(pruned.Instances, rec)
	}
	_ = SaveRegistry(m.registry, pruned)
}

// Status probes every recorded instance and refreshes the registry: liveness is
// measured, never inferred from a recorded pid.
func (m *Manager) Status(ctx context.Context) ([]Record, error) {
	reg, err := LoadRegistry(m.registry)
	if err != nil {
		return nil, err
	}
	refreshed, err := m.refresh(ctx, reg)
	if err != nil {
		return nil, err
	}
	return refreshed.Instances, nil
}

// refresh probes each record, rewrites its state, and persists the result.
func (m *Manager) refresh(ctx context.Context, reg Registry) (Registry, error) {
	out := Registry{SchemaVersion: RegistrySchemaVersion}
	for _, rec := range reg.Instances {
		managed := m.liveProcess(rec.Name)
		switch {
		case m.circuitOpen(rec.Name):
			rec.State = StateCircuitOpen
		case managed == nil:
			rec.State = StateStopped
		case m.probe(ctx, managed) == nil:
			rec.State = StateRunning
			rec.LastHealthy = m.now()
			rec.PID = managed.Record.PID
		case m.circuitOpen(rec.Name):
			rec.State = StateCircuitOpen
		default:
			rec.State = StateStopped
		}
		out.Instances = append(out.Instances, rec)
	}
	if err := SaveRegistry(m.registry, out); err != nil {
		return out, err
	}
	return out, nil
}

// normalize fills the defaults a caller may omit.
func (m *Manager) normalize(spec Spec) (Spec, error) {
	if strings.TrimSpace(spec.Name) == "" {
		return spec, errors.New("agentd: instance name is required")
	}
	if strings.TrimSpace(spec.Root) == "" {
		return spec, errors.New("agentd: workspace root is required")
	}
	if spec.Addr == "" {
		addr, err := AllocateAddr()
		if err != nil {
			return spec, err
		}
		spec.Addr = addr
	}
	if spec.TokenFile == "" {
		spec.TokenFile = TokenPath(m.home, spec.Name)
	}
	if spec.PortFile == "" {
		spec.PortFile = filepath.Join(filepath.Dir(spec.TokenFile), spec.Name+".addr")
	}
	if spec.LogPath == "" {
		spec.LogPath = LogPath(m.home, spec.Name)
	}
	return spec, nil
}

// probe checks one managed instance's /status.
func (m *Manager) probe(ctx context.Context, managed *Process) error {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return managed.Probe(probeCtx, m.client)
}

// awaitReady polls /status until the child answers or the launch window closes.
func (m *Manager) awaitReady(ctx context.Context, managed *Process) error {
	deadline := m.now().Add(m.launchWait)
	for {
		if err := m.probe(ctx, managed); err == nil {
			return nil
		} else if m.now().After(deadline) {
			return fmt.Errorf("agentd: %s did not become ready: %w", managed.Record.Name, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPoll):
		}
	}
}

// save applies a registry mutation under the manager lock and the file lock.
func (m *Manager) save(mutate func(Registry) Registry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	reg, err := LoadRegistry(m.registry)
	if err != nil {
		return err
	}
	return SaveRegistry(m.registry, mutate(reg))
}

func (m *Manager) liveProcess(name string) *Process {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.live[name]
}

// specFor rebuilds a restart's launch inputs from a record: the argv stays
// defaulted to this binary's own serve.
func (m *Manager) specFor(rec Record) Spec {
	return Spec{
		Name:        rec.Name,
		Root:        rec.Root,
		Addr:        rec.Addr,
		TokenFile:   rec.TokenFile,
		PortFile:    filepath.Join(filepath.Dir(rec.TokenFile), rec.Name+".addr"),
		SessionPath: rec.SessionPath,
		LogPath:     LogPath(m.home, rec.Name),
		Argv:        rec.Argv,
	}
}

// ensureToken returns the instance's bearer token, minting one when the file is
// missing. An existing token is never rotated: a running child still holds it.
func ensureToken(path string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(raw)); token != "" {
			return token, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf[:])
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

// refuseHeldSession refuses to start a second writer for a session another
// runtime already holds: the lease is what keeps two serves from splitting one
// transcript between them.
func refuseHeldSession(sessionPath string) error {
	if strings.TrimSpace(sessionPath) == "" {
		return nil
	}
	info, held, err := agent.InspectSessionLease(sessionPath)
	if err != nil {
		return fmt.Errorf("agentd: session lease: %w", err)
	}
	if !held {
		return nil
	}
	writer := "another runtime"
	if info != nil && info.WriterID != "" {
		writer = fmt.Sprintf("%s (pid %d)", info.WriterID, info.PID)
	}
	return fmt.Errorf("agentd: session %s is held by %s; stop it there first", filepath.Base(sessionPath), writer)
}
