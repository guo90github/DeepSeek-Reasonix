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
	"slices"
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
	a.attachEmbeddedServeHooks(host.srv)
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

// attachEmbeddedServeHooks points a Serve instance at this window's own
// controllers and catalogs. Kept in one place so the remote HTTP surface is
// wired identically wherever it runs: a hook that is not registered here is a
// hook the phone never reaches.
func (a *App) attachEmbeddedServeHooks(srv *serve.Server) {
	srv.SetForegroundProvider(a.embeddedServeController)
	srv.SetSessionActivator(a.activateSessionForRemote)
	// /new is the window's own move: its active tab is a controller this
	// process built, which carries no serve-side session tag to rotate.
	srv.SetSessionCreator(a.createSessionForRemote)
	srv.SetSessionLister(a.listSessionsForRemote)
	srv.SetProjectLister(a.listProjectsForRemote)
	srv.SetProjectCreator(a.createProjectForRemote)
	srv.SetSubmitDelegate(a.submitRemoteInput)
	// The session-aware form: a wake that names a session lands in that tab.
	srv.SetSubmitDelegateFor(a.submitRemoteInputFor)
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

// createSessionForRemote answers POST /new with the window's own move: it opens
// a blank surface for the workspace the caller named — the active tab's scope
// when it named none — exactly as the app shell's "new session" does, and
// reports the session the phone should switch to.
func (a *App) createSessionForRemote(ctx context.Context, req serve.NewSessionRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	scope, root, err := a.remoteNewSessionTarget(req)
	if err != nil {
		return "", err
	}
	// Split reuses or adds a surface without collapsing the tabs the user has
	// open; the single-surface styles keep exactly one — the same branch the app
	// shell's own "new session" takes (desktopNavigationOwner.ts openBlank).
	var meta TabMeta
	if a.singleSurfaceLayoutEnabled() {
		meta, err = a.EnsureBlankSurface(scope, root)
	} else {
		meta, err = a.EnsureBlankTab(scope, root)
	}
	if err != nil {
		return "", err
	}
	if path := strings.TrimSpace(meta.SessionPath); path != "" {
		return path, nil
	}
	return "", errors.New("the new session is not ready yet; retry once the window shows it")
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
func (a *App) listSessionsForRemote(all bool) []serve.SessionInfo {
	if a.sessionCatalog.Load() == nil {
		return nil
	}
	metas := a.ListSessions()
	if all {
		metas = append(metas, a.listAllWorkspaceSessions(metas)...)
	}
	_, ctrl := a.activeTabAndCtrl()
	foregroundRunning := ctrl != nil && ctrl.Running()
	_, sessionOverlays := a.catalogRuntimeOverlays()
	out := make([]serve.SessionInfo, 0, len(metas))
	for _, meta := range metas {
		title := remoteSessionTitle(meta)
		// Running is a property of the session, not of the window: a detached
		// runtime keeps running after the foreground moved on, and the phone
		// counts its badges from these rows.
		running := sessionOverlays[sessionRuntimeKey(meta.Path)].running ||
			(meta.Current && foregroundRunning)
		out = append(out, serve.SessionInfo{
			Name:        strings.TrimSuffix(filepath.Base(meta.Path), ".jsonl"),
			Path:        meta.Path,
			Title:       title,
			Turns:       meta.Turns,
			Current:     meta.Current,
			Running:     running,
			MtimeMilli:  meta.LastActivityAt,
			ProjectRoot: meta.WorkspaceRoot,
		})
	}
	return out
}

// remoteSessionTitle 沿用桌面端自己的会话名优先级（title → topicTitle → preview）：
// 远端要显示窗口里的同一个名字，漏掉 topicTitle 就会把"新的会话"那类占位名显示成首条消息。
func remoteSessionTitle(meta SessionMeta) string {
	switch {
	case meta.Title != "":
		return meta.Title
	case meta.TopicTitle != "":
		return meta.TopicTitle
	default:
		return meta.Preview
	}
}

// listAllWorkspaceSessions 把侧边栏里每个工作区的会话都取来（跳过已在前台清单里的），
// 让远端看到的项目数与桌面一致；每行带 WorkspaceRoot，手机才能显示可读的项目名。
func (a *App) listAllWorkspaceSessions(seen []SessionMeta) []SessionMeta {
	have := make(map[string]bool, len(seen))
	for _, meta := range seen {
		have[strings.ToLower(filepath.Clean(meta.Path))] = true
	}
	active := filepath.Clean(a.activeSessionDir())
	out := []SessionMeta{}
	for _, ws := range a.ListWorkspaces() {
		dir := config.ProjectSessionDir(ws.Path)
		if filepath.Clean(dir) == active {
			continue
		}
		for _, meta := range a.listSessionsFromDir(dir, "") {
			if have[strings.ToLower(filepath.Clean(meta.Path))] {
				continue
			}
			have[strings.ToLower(filepath.Clean(meta.Path))] = true
			out = append(out, meta)
		}
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
	wired := eventwire.ToWire(e)
	if wired.SessionPath == "" {
		// Remote SSE clients (the phone reads /events?all=1) route frames by
		// SessionPath and drop untagged ones as unattributable, so an unstamped
		// turn_started/tool_*/turn_done left their turn state frozen.
		wired.SessionPath = foreground
	}
	host.bc.EmitWire(wired)
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

// hostProcessEnvForRoot tells a workspace's MCP children which serve endpoint
// and session own them. Without it a remote wake can only address whichever
// tab happens to be foreground, never the workspace that asked.
//
// One root shares one child, so this session value is a default rather than an
// identity: a server that must address the session that asked reads the caller
// session on each call instead (internal/plugin call_meta.go).
//
// The values must not depend on the listener being up. Children spawn while
// tabs build, and startup starts the listener only after a foreground tab is
// ready — a child born in that window keeps the missing env for its whole life
// (plugin.Host resolves the provider once per spawn, and one root reuses one
// child). A remote wake then names no session and lands in the foreground tab.
func (a *App) hostProcessEnvForRoot(root string) map[string]string {
	env := map[string]string{
		"REASONIX_SERVE_URL":        embeddedServeURL(nil),
		"REASONIX_SERVE_TOKEN_FILE": serveTokenPath(),
	}
	if host := embeddedServeState.Load(); host != nil {
		env["REASONIX_SERVE_URL"] = embeddedServeURL(host)
	}
	path := a.sessionPathForRoot(root)
	if path == "" {
		slog.Warn("host process env: no session path for root; a child reading only this env names no session", "root", root)
		return env
	}
	env["REASONIX_SESSION_PATH"] = path
	return env
}

// embeddedServeURL is the loopback form of the window's serve address: the
// listener binds the wildcard so a phone can reach it, while a child running on
// this machine wants 127.0.0.1. A nil host means the listener is not up yet.
func embeddedServeURL(host *embeddedServe) string {
	if host == nil {
		return loopbackServeURL(embeddedServeAddr)
	}
	return loopbackServeURL(host.addr)
}

func loopbackServeURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + embeddedServeAddr
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port)
}

// sessionPathForRoot names the session a workspace's MCP child belongs to: the
// active tab's when that tab is in the root, else the newest tab the window
// holds for it. Tabs of one root share a plugin host, hence one child and one
// name.
//
// The scan follows tabOrder rather than ranging the tab map: this pick decides
// which session a remote wake addresses, and "whichever tab the range happened
// to end on" is not a rule anyone can rely on.
func (a *App) sessionPathForRoot(root string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	active, newest := "", ""
	for _, id := range a.tabIDsInWindowOrder() {
		tab := a.tabs[id]
		if !tabMatchesRoot(tab, root) {
			continue
		}
		path := strings.TrimSpace(tab.currentSessionPath())
		if path == "" {
			continue
		}
		newest = path
		if tab.ID == a.activeTabID {
			active = path
		}
	}
	if active != "" {
		return agent.CanonicalSessionPath(active)
	}
	return agent.CanonicalSessionPath(newest)
}

// tabMatchesRoot compares a tab against a shared-host key: the key global tabs
// carry is not their (empty) WorkspaceRoot, and a project root can differ from
// the tab's spelling, so neither side survives a bare string compare.
func tabMatchesRoot(tab *WorkspaceTab, root string) bool {
	if tab == nil || tab.ReadOnly {
		return false
	}
	if root == globalSharedHostKey {
		return strings.TrimSpace(tab.WorkspaceRoot) == ""
	}
	return sameProjectRoot(tab.WorkspaceRoot, root)
}

// tabIDsInWindowOrder lists the window's tabs in tabOrder, then any tab the
// order has not published yet (sorted), so the scan above stays deterministic.
func (a *App) tabIDsInWindowOrder() []string {
	ordered := append([]string(nil), a.tabOrder...)
	seen := make(map[string]struct{}, len(ordered))
	for _, id := range ordered {
		seen[id] = struct{}{}
	}
	rest := make([]string, 0, len(a.tabs))
	for id := range a.tabs {
		if _, ok := seen[id]; !ok {
			rest = append(rest, id)
		}
	}
	slices.Sort(rest)
	return append(ordered, rest...)
}
