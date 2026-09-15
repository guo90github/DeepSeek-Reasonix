package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/eventwire"
	"reasonix/internal/serve"
)

// embeddedServeAddr is where the phone reaches this window. The wildcard bind
// is what makes it reachable over LAN or Tailscale; the token below is the only
// authentication, so the machine must stay on a trusted network.
const embeddedServeAddr = "0.0.0.0:8787"

const (
	embeddedServeControllerWait = 60 * time.Second
	embeddedServeReadyPoll      = 250 * time.Millisecond
)

// embeddedServeState is the process's single phone-remote host. The desktop
// window is a per-process singleton (one NewApp, one host service), so a
// package-level pointer leaves App's guarded field count alone.
var embeddedServeState atomic.Pointer[embeddedServe]

// embeddedServe exposes the window's active tab over Serve's HTTP surface from
// inside this process: the phone submits into the controller the user is
// watching, and a same-process lease is never "foreign".
type embeddedServe struct {
	app *App
	bc  *serve.Broadcaster
	srv *serve.Server

	// mirroredOnce logs the first forwarded frame: without it a silent mirror
	// and a working one look identical from outside the process.
	mirroredOnce sync.Once

	mu     sync.Mutex
	ln     net.Listener
	cancel context.CancelFunc
	addr   string
}

func (h *embeddedServe) noteMirroredFrame(session string) {
	h.mirroredOnce.Do(func() {
		slog.Info("embedded serve: mirroring foreground frames", "session", session)
	})
}

// startEmbeddedServe runs from App.startup. It waits for a foreground
// controller because Serve binds one at construction and the phone has nothing
// to talk to before the first tab is ready.
func (a *App) startEmbeddedServe() {
	select {
	case <-a.tabsRestored:
	case <-time.After(embeddedServeControllerWait):
	}
	deadline := time.Now().Add(embeddedServeControllerWait)
	var ctrl control.SessionAPI
	for {
		var ready bool
		if ctrl, ready = a.embeddedServeController(); ready {
			break
		}
		if time.Now().After(deadline) {
			slog.Warn("embedded serve: no ready tab; phone remote stays off")
			return
		}
		time.Sleep(embeddedServeReadyPoll)
	}
	token, err := loadOrCreateServeToken()
	if err != nil {
		slog.Error("embedded serve: token", "err", err)
		return
	}
	ln, err := net.Listen("tcp", embeddedServeAddr)
	if err != nil {
		slog.Error("embedded serve: listen", "addr", embeddedServeAddr, "err", err)
		return
	}
	host := &embeddedServe{app: a, bc: serve.NewBroadcaster()}
	host.srv = serve.New(ctrl, host.bc, config.ServeConfig{AuthMode: "token", Token: token})
	host.srv.SetForegroundProvider(a.embeddedServeController)
	host.srv.SetSessionActivator(a.activateSessionForRemote)
	host.srv.SetSessionLister(a.listSessionsForRemote)
	host.srv.SetSubmitDelegate(a.submitRemoteInput)
	// The session-aware form: a wake that names a session lands in that tab.
	host.srv.SetSubmitDelegateFor(a.submitRemoteInputFor)
	host.ln, host.addr = ln, ln.Addr().String()
	ctx, cancel := context.WithCancel(a.bootContext())
	host.mu.Lock()
	host.cancel = cancel
	host.mu.Unlock()
	embeddedServeState.Store(host)
	go func() {
		if err := host.srv.RunGracefulListener(ctx, ln); err != nil {
			slog.Warn("embedded serve: stopped", "err", err)
		}
	}()
	slog.Info("embedded serve: listening", "addr", host.addr, "tokenFile", serveTokenPath())
}

// embeddedServeController resolves the controller the phone must reach: the
// active tab's, so a submit lands in the conversation the user is reading.
func (a *App) embeddedServeController() (control.SessionAPI, bool) {
	tab, ctrl := a.activeTabAndCtrl()
	if tab == nil || tab.ReadOnly || ctrl == nil {
		return nil, false
	}
	return ctrl, true
}

// submitRemoteInput routes the phone's prompt through the window's own composer
// path. Serve's default submit runs the turn but renders and records no user
// message, so the window showed the answer with no prompt above it.
func (a *App) submitRemoteInput(input string) error {
	tab, ctrl := a.activeTabAndCtrl()
	if tab == nil || ctrl == nil {
		return errors.New("no foreground tab to receive this message")
	}
	return a.submitDisplayToTab(tab.ID, input, input, "")
}

// submitRemoteInputFor is the session-aware form: a wake names the session it
// belongs to, so the message lands there instead of in whichever tab happens to
// be foreground. An empty path means "the foreground tab" — the same as the
// plain form, which is what a host without session plumbing gets.
func (a *App) submitRemoteInputFor(sessionPath, input string) error {
	if strings.TrimSpace(sessionPath) == "" {
		return a.submitRemoteInput(input)
	}
	tab, err := a.tabBySessionPath(sessionPath)
	if err != nil {
		return err
	}
	return a.submitDisplayToTab(tab.ID, input, input, "")
}

// tabBySessionPath resolves an open tab by canonical session path. It is shared
// with activateSessionForRemote so "the same session" means one thing in both
// places (tab.SessionPath can lag a rebind, hence the canonical form).
func (a *App) tabBySessionPath(sessionPath string) (*WorkspaceTab, error) {
	want := agent.CanonicalSessionPath(strings.TrimSpace(sessionPath))
	if want == "" {
		return nil, errors.New("no session path given")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.tabs {
		if tab.ReadOnly || tab.SessionPath == "" {
			continue
		}
		if agent.CanonicalSessionPath(tab.SessionPath) == want {
			return tab, nil
		}
	}
	return nil, errors.New("session is not open in the desktop window; open it there first")
}

// listSessionsForRemote answers Serve's /sessions from the desktop's session
// catalog — the same rows the history panel shows — instead of Serve's
// per-transcript walk. A nil return before the catalog is ready falls back.
func (a *App) listSessionsForRemote() []serve.SessionInfo {
	if a.sessionCatalog.Load() == nil {
		return nil
	}
	metas := a.ListSessions()
	_, ctrl := a.activeTabAndCtrl()
	running := ctrl != nil && ctrl.Running()
	out := make([]serve.SessionInfo, 0, len(metas))
	for _, meta := range metas {
		title := meta.Title
		if title == "" {
			title = meta.Preview
		}
		out = append(out, serve.SessionInfo{
			Name:       strings.TrimSuffix(filepath.Base(meta.Path), ".jsonl"),
			Path:       meta.Path,
			Title:      title,
			Turns:      meta.Turns,
			Current:    meta.Current,
			Running:    meta.Current && running,
			MtimeMilli: meta.LastActivityAt,
		})
	}
	return out
}

// activateSessionForRemote routes the phone's /resume onto the window that
// already owns the session. A session with no open tab stays unavailable rather
// than being opened behind the user's back.
func (a *App) activateSessionForRemote(path string) error {
	want := agent.CanonicalSessionPath(path)
	a.mu.RLock()
	var match *WorkspaceTab
	for _, tab := range a.tabs {
		if tab.ReadOnly || tab.SessionPath == "" {
			continue
		}
		if agent.CanonicalSessionPath(tab.SessionPath) == want {
			match = tab
			break
		}
	}
	active := a.activeTabID
	a.mu.RUnlock()
	if match == nil {
		return fmt.Errorf("session is not open in the desktop window; open it there first")
	}
	if match.ID == active {
		return nil
	}
	return a.SetActiveTab(match.ID)
}

// mirrorEventToEmbeddedServe republishes the foreground tab's frames through
// the embedded broadcaster, which is how the phone renders the same turn live.
// Membership follows the controller's own session path: the tab's persisted
// path and its event-sink id can both lag a rebind, and either one silently
// drops every frame when it does.
func (a *App) mirrorEventToEmbeddedServe(tabID string, e event.Event) {
	host := embeddedServeState.Load()
	if host == nil {
		return
	}
	tab, ctrl := a.activeTabAndCtrl()
	if tab == nil || ctrl == nil {
		return
	}
	foreground := agent.CanonicalSessionPath(ctrl.SessionPath())
	if foreground == "" {
		return
	}
	if e.SessionPath == "" {
		// Untagged frames carry no session of their own, so only the emitting
		// tab being the foreground one keeps them from crossing sessions.
		if tab.ID != tabID {
			return
		}
	} else if agent.CanonicalSessionPath(e.SessionPath) != foreground {
		return
	}
	host.bc.SetCurrentSession(foreground)
	host.bc.EmitWire(eventwire.ToWire(e))
	host.noteMirroredFrame(foreground)
}

func serveTokenPath() string {
	return filepath.Join(config.ReasonixHomeDir(), "serve.token")
}

// loadOrCreateServeToken persists the phone token across restarts so an APK
// build can bake it in once instead of asking the user to retype it.
func loadOrCreateServeToken() (string, error) {
	path := serveTokenPath()
	if raw, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(raw)); token != "" {
			return token, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}
