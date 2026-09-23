package agentd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"reasonix/internal/proc"
)

// Spec is one instance's launch inputs. Argv overrides the default command
// (this binary's own `serve`), which is what tests and alternate builds need.
type Spec struct {
	Name        string
	Root        string
	Addr        string
	TokenFile   string
	PortFile    string
	SessionPath string
	LogPath     string
	Argv        []string
}

// ServeEnv is what a managed serve hands down to its own MCP children: they
// inherit the process env, so each workspace learns which serve endpoint and
// session own it. The desktop cannot work this way — one process serves every
// root — which is why it installs a host-level provider instead.
func ServeEnv(rec Record) map[string]string {
	env := map[string]string{"REASONIX_SERVE_URL": rec.URL()}
	if rec.TokenFile != "" {
		env["REASONIX_SERVE_TOKEN_FILE"] = rec.TokenFile
	}
	if rec.SessionPath != "" {
		env["REASONIX_SESSION_PATH"] = rec.SessionPath
	}
	return env
}

// Process is a live child the manager probes, restarts and stops.
type Process struct {
	Record Record
	cmd    *exec.Cmd
	job    uintptr
	log    *os.File
	// exited closes when the child ends on its own, which is what a deliberate
	// retirement waits for instead of killing the tree.
	exited chan struct{}
	// retiring is set under the manager's lock while a stop is in flight: a
	// second Down joins it instead of retiring twice, and Up refuses instead of
	// racing the session lease with a stop that has not landed yet.
	retiring bool
}

// Spawn starts one serve process. It does not wait for health: the caller
// probes, so a slow first turn is not mistaken for a failed launch.
func Spawn(ctx context.Context, spec Spec) (*Process, error) {
	argv := spec.Argv
	if len(argv) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		argv = []string{exe, "serve",
			"--addr", spec.Addr,
			"--auth", "token",
			"--token-file", spec.TokenFile,
			"--port-file", spec.PortFile,
			"--no-open",
		}
	}
	rec := Record{
		Name: spec.Name, Root: spec.Root, Addr: spec.Addr, TokenFile: spec.TokenFile,
		SessionPath: spec.SessionPath, StartedAt: time.Now().UTC(), State: StateRunning,
	}
	rec.Argv = argv
	cmd := proc.CommandContext(ctx, argv[0], argv[1:]...)
	proc.HideWindow(cmd)
	cmd.Dir = spec.Root
	cmd.Env = append(os.Environ(), envPairs(ServeEnv(rec))...)
	logFile := openLogFile(spec.LogPath)
	if logFile != nil {
		cmd.Stdout, cmd.Stderr = logFile, logFile
	}
	job, err := proc.StartTracked(cmd)
	if err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, fmt.Errorf("agentd: start %s: %w", spec.Name, err)
	}
	rec.PID = cmd.Process.Pid
	exited := make(chan struct{})
	// Reaping is owed whoever ends the child; it also releases a process handle
	// that a retirement would otherwise leak.
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	return &Process{Record: rec, cmd: cmd, job: job, log: logFile, exited: exited}, nil
}

// Probe asks the instance's /status with the instance's own token: the same
// credential a coordinator must present, so a probe proves that path too.
func (p *Process) Probe(ctx context.Context, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Record.URL()+"/status", nil)
	if err != nil {
		return err
	}
	if err := p.authorize(req); err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agentd: %s status %s", p.Record.Name, resp.Status)
	}
	return nil
}

// Retire stops the instance in the order that loses no work: end the running
// turn, ask the serve to shut itself down, then wait for the child to exit. A
// refused cancel is reported rather than fatal: ending this instance is still
// the caller's intent, and only a refused shutdown falls back to the tree kill.
func (p *Process) Retire(ctx context.Context, client *http.Client) error {
	if err := p.command(ctx, client, "/cancel"); err != nil {
		slog.Warn("agentd: cancel refused; retiring anyway", "instance", p.Record.Name, "err", err)
	}
	if err := p.command(ctx, client, "/shutdown"); err != nil {
		return err
	}
	select {
	case <-p.exited:
		p.release()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// command posts one authenticated command with the JSON body csrfGuard demands
// on every POST. The bearer token is how a non-browser caller authenticates to
// a token-mode serve, which hands no cookie to anyone.
func (p *Process) command(ctx context.Context, client *http.Client, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Record.URL()+path, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	if err := p.authorize(req); err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("agentd: %s %s returned %s", p.Record.Name, path, resp.Status)
	}
	return nil
}

// authorize attaches the token of the instance this call belongs to.
func (p *Process) authorize(req *http.Request) error {
	token, err := readToken(p.Record.TokenFile)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// Stop kills the instance and its whole tree: a serve's MCP children must not
// outlive it, and killing by process name would be unsafe under pid reuse.
func (p *Process) Stop() {
	if p == nil {
		return
	}
	if p.job != 0 {
		// KillTracked releases the job handle itself; releasing it twice could
		// close whatever handle reused the value.
		proc.KillTracked(p.cmd, p.job)
		p.job = 0
	} else {
		proc.KillTree(p.cmd)
	}
	p.closeLog()
}

// release retires a child that has already exited: closing the kill-on-close
// job still fells the descendants it kept, so none outlives the instance.
func (p *Process) release() {
	if p.job != 0 {
		proc.FinishTracked(p.job)
		p.job = 0
	}
	p.closeLog()
}

// closeLog releases the child's log handle. Stop closes it on the normal path;
// a child that died on its own is replaced without Stop, so the manager has to
// hand the handle back explicitly.
func (p *Process) closeLog() {
	if p == nil || p.log == nil {
		return
	}
	_ = p.log.Close()
	p.log = nil
}

// openLogFile gives the child a place to leave evidence; a failed open is not
// fatal, the child simply keeps its default streams.
func openLogFile(path string) *os.File {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil
	}
	return file
}

// envPairs renders extra env as KEY=value entries, sorted for a stable child
// environment across runs.
func envPairs(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		out = append(out, key+"="+env[key])
	}
	return out
}

// readToken reads a serve token file, trimming the newline it is written with.
func readToken(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("agentd: instance has no token file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("agentd: %s is empty", filepath.Base(path))
	}
	return token, nil
}
