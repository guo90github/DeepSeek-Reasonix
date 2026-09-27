package main

import (
	"encoding/json"
	"fmt"
	"net/url"

	"reasonix/internal/control"
)

// InboxRoomLineView is the bridge answer for one room line. found=false means the
// line is not in the queue — never "no such line": when the ending is known the
// line comes back carrying it, and only a bare found=false says nobody took it in.
type InboxRoomLineView struct {
	Found bool                   `json:"found"`
	Line  *control.InboxRoomLine `json:"line,omitempty"`
}

// InboxRoomLine answers what one tab's session holds for a room line, by the seq
// the room prints. A local tab asks its own controller; a remote tab forwards to
// the host that owns it. Either way the same query decides — this only carries it.
func (a *App) InboxRoomLine(tabID string, seq int64) (InboxRoomLineView, error) {
	if a.isRemoteTab(tabID) {
		return a.remoteInboxRoomLine(tabID, seq)
	}
	ctrl, err := a.inboxCtrl(tabID)
	if err != nil {
		return InboxRoomLineView{}, err
	}
	lookup, ok := ctrl.(interface {
		InboxRoomLineFor(int64) (control.InboxRoomLine, bool)
	})
	if !ok {
		return InboxRoomLineView{}, fmt.Errorf("this host cannot answer room lines")
	}
	line, found := lookup.InboxRoomLineFor(seq)
	// A line that already left the queue is still an answer: found stays false and
	// the ending it carries separates "ran, then cancelled" from "never took it
	// in". Dropping it here is how these two surfaces told a caller different things.
	if !found && line.Settled == "" {
		return InboxRoomLineView{Found: false}, nil
	}
	return InboxRoomLineView{Found: found, Line: &line}, nil
}

// remoteInboxRoomLine forwards the question to the host that owns a remote tab,
// with the same route identity and staleness fencing as the other inbox reads.
func (a *App) remoteInboxRoomLine(tabID string, seq int64) (InboxRoomLineView, error) {
	a.remoteTabMu.Lock()
	tab := a.remoteTabs[tabID]
	if tab == nil || tab.client == nil || tab.state != "ready" || tab.routing.currentPath == "" || tab.routing.rehydratingPath != "" {
		a.remoteTabMu.Unlock()
		return InboxRoomLineView{}, fmt.Errorf("remote tab %q is not ready for inbox reads", tabID)
	}
	client, base, path := tab.client, tab.base, tab.routing.currentPath
	gen, selection := tab.gen, tab.selectionRevision
	a.remoteTabMu.Unlock()

	ctx, cancel := commandContext(a)
	defer cancel()
	route := fmt.Sprintf("/inbox/room-line?seq=%d&session=%s", seq, url.QueryEscape(path))
	data, err := serveGet(ctx, client, serveURL(base, route))
	if err != nil {
		return InboxRoomLineView{}, err
	}
	var view InboxRoomLineView
	if err := json.Unmarshal(data, &view); err != nil {
		return InboxRoomLineView{}, err
	}
	a.remoteTabMu.Lock()
	current := a.remoteTabs[tabID]
	stale := current != tab || current.gen != gen || current.selectionRevision != selection ||
		current.routing.currentPath != path || current.routing.rehydratingPath != ""
	a.remoteTabMu.Unlock()
	if stale {
		return InboxRoomLineView{}, fmt.Errorf("remote inbox route changed during read")
	}
	return view, nil
}
