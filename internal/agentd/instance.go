package agentd

import (
	"context"
	"errors"
	"fmt"
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
	return &Process{Record: rec, cmd: cmd, job: job, log: logFile}, nil
}

// Probe asks the instance's /status with the instance's own token: the same
// credential a coordinator must present, so a probe proves that path too.
func (p *Process) Probe(ctx context.Context, client *http.Client) error {
	token, err := readToken(p.Record.TokenFile)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Record.URL()+"/status", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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

// Stop kills the instance and its whole tree: a serve's MCP children must not
// outlive it, and killing by process name would be unsafe under pid reuse.
func (p *Process) Stop() {
	if p == nil {
		return
	}
	if p.job != 0 {
		proc.KillTracked(p.cmd, p.job)
		proc.FinishTracked(p.job)
		p.job = 0
	} else {
		proc.KillTree(p.cmd)
	}
	if p.log != nil {
		_ = p.log.Close()
	}
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
