package cli

import (
	"log/slog"
	"net"
	"strings"
	"sync"
)

// serveMCPEnvState carries what a CLI-hosted Serve tells its MCP children: the
// endpoint a remote wake should reach, the token file they can read, and the
// session that owns them. Values resolve once per spawn, so a listener that
// binds after the first controller still reaches children born later.
type serveMCPEnvState struct {
	mu         sync.RWMutex
	url        string
	tokenFile  string
	tokenAuth  bool
	sessions   func(root string) string
	warnedKeys map[string]bool
}

func newServeMCPEnvState(addr, tokenFile string) *serveMCPEnvState {
	return &serveMCPEnvState{
		url:        loopbackServeURL(addr),
		tokenFile:  strings.TrimSpace(tokenFile),
		warnedKeys: map[string]bool{},
	}
}

// setBoundAddr tightens the URL to the address the listener actually took,
// which differs from --addr when Web picks another port.
func (st *serveMCPEnvState) setBoundAddr(addr string) {
	if st == nil || strings.TrimSpace(addr) == "" {
		return
	}
	st.mu.Lock()
	st.url = loopbackServeURL(addr)
	st.mu.Unlock()
}

// setServe wires the lookups that exist only once the Server is built.
func (st *serveMCPEnvState) setServe(sessions func(root string) string, tokenAuth bool) {
	if st == nil {
		return
	}
	st.mu.Lock()
	st.sessions = sessions
	st.tokenAuth = tokenAuth
	st.mu.Unlock()
}

// envForRoot is the provider boot installs on the hosts it creates.
func (st *serveMCPEnvState) envForRoot(root string) map[string]string {
	if st == nil {
		return nil
	}
	st.mu.RLock()
	url, tokenFile, tokenAuth, sessions := st.url, st.tokenFile, st.tokenAuth, st.sessions
	st.mu.RUnlock()
	if url == "" {
		return nil
	}
	env := map[string]string{"REASONIX_SERVE_URL": url}
	if tokenFile != "" {
		env["REASONIX_SERVE_TOKEN_FILE"] = tokenFile
	} else if tokenAuth {
		st.warnOnce("token-file",
			"no token file for MCP children, so a remote wake cannot authenticate; start Serve with --token-file to make it wakeable")
	}
	if sessions == nil {
		return env
	}
	if path := strings.TrimSpace(sessions(root)); path != "" {
		env["REASONIX_SESSION_PATH"] = path
	} else {
		st.warnOnce("session:"+root,
			"no single session owns this workspace, so a remote wake for it lands in the foreground session", "root", root)
	}
	return env
}

// warnOnce reports a boundary once per key: the provider runs on every spawn.
func (st *serveMCPEnvState) warnOnce(key, msg string, args ...any) {
	st.mu.Lock()
	seen := st.warnedKeys[key]
	st.warnedKeys[key] = true
	st.mu.Unlock()
	if !seen {
		slog.Warn("serve: "+msg, args...)
	}
}

// loopbackServeURL is the form a child on this machine dials when the listener
// bound the wildcard so a phone can reach it.
func loopbackServeURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	switch host {
	case "", "0.0.0.0", "::", "*":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
