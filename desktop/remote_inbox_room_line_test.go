package main

import (
	"net/http"
	"strings"
	"testing"
)

// The by-seq question asked of a remote tab must be forwarded with the same route
// identity as every other remote inbox read, and a host that cannot answer must
// never be read as "this session never took that line in".
func TestRemoteRoomLineForwardsWithRouteIdentity(t *testing.T) {
	var seenPath string
	var seenSeq, seenSession string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenPath = req.URL.Path
		seenSeq = req.URL.Query().Get("seq")
		seenSession = req.URL.Query().Get("session")
		body := `{"found":true,"line":{"itemId":"it-1","state":"queued","source":"push","gate":"dispatch","reason":"忙","refused":true,"queuedForMs":65000}}`
		return remoteRuntimeTestResponse(req, http.StatusOK, body), nil
	})}
	a, tab := remoteRuntimeTestApp(client)

	view, err := a.InboxRoomLine(tab.id, 43)
	if err != nil {
		t.Fatalf("InboxRoomLine: %v", err)
	}
	if seenPath != "/inbox/room-line" || seenSeq != "43" || seenSession != runtimeRemoteTestPath {
		t.Fatalf("forwarded %q seq=%q session=%q，想要带路线身份的 /inbox/room-line", seenPath, seenSeq, seenSession)
	}
	if !view.Found || view.Line == nil || view.Line.ItemID != "it-1" || view.Line.State != "queued" || view.Line.Reason != "忙" || !view.Line.Refused || view.Line.QueuedForMs != 65000 {
		t.Fatalf("view = %+v，远程宿主的回答没被逐字带回来", view)
	}

	// Not found on the remote host is an answer, not an error and not a zero view.
	missing := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return remoteRuntimeTestResponse(req, http.StatusOK, `{"found":false}`), nil
	})}
	am, tabm := remoteRuntimeTestApp(missing)
	view, err = am.InboxRoomLine(tabm.id, 44)
	if err != nil || view.Found || view.Line != nil {
		t.Fatalf("view = %+v, err = %v，远端答“没有这条”应是 found=false 而不是错误", view, err)
	}
}

func TestRemoteRoomLineRefusesToGuess(t *testing.T) {
	// A tab that is not ready for inbox reads must say so instead of asking.
	a, tab := remoteRuntimeTestApp(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("an unready tab must not be asked: %s", req.URL)
		return nil, nil
	})})
	tab.routing.currentPath = ""
	if _, err := a.InboxRoomLine(tab.id, 43); err == nil {
		t.Fatal("没就绪的远程标签被当成能回答的")
	}

	// A host that does not implement the route must surface that, never a silent
	// "found=false" that reads as "this session never took that line in".
	unimplemented := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return remoteRuntimeTestResponse(req, http.StatusNotImplemented, "this host cannot answer room lines"), nil
	})}
	au, tabu := remoteRuntimeTestApp(unimplemented)
	if view, err := au.InboxRoomLine(tabu.id, 43); err == nil {
		t.Fatalf("view = %+v，宿主没这个端点却被当成“没有这条”", view)
	}

	// Garbage is not an answer either.
	garbage := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return remoteRuntimeTestResponse(req, http.StatusOK, "<html>not json</html>"), nil
	})}
	ag, tabg := remoteRuntimeTestApp(garbage)
	if view, err := ag.InboxRoomLine(tabg.id, 43); err == nil {
		t.Fatalf("view = %+v，解不动的回答被当成了答案", view)
	}
}

func TestRemoteRoomLineRejectsAStaleRoute(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/inbox/room-line" || req.URL.Query().Get("session") != runtimeRemoteTestPath {
			t.Errorf("unfenced room-line request: %s", req.URL)
		}
		close(started)
		<-release
		return remoteRuntimeTestResponse(req, http.StatusOK, `{"found":true,"line":{"itemId":"it-1","state":"queued"}}`), nil
	})}
	a, tab := remoteRuntimeTestApp(client)
	done := make(chan error, 1)
	go func() { _, err := a.InboxRoomLine(tab.id, 43); done <- err }()
	<-started
	a.remoteTabMu.Lock()
	tab.selectionRevision++
	a.remoteTabMu.Unlock()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("路线换过之后的回答被接受了")
	}
}

func TestRemoteRoomLineReportsBadSequence(t *testing.T) {
	a, tab := remoteRuntimeTestApp(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.RawQuery, "seq=") {
			t.Fatalf("seq 没被带出去: %s", req.URL)
		}
		return remoteRuntimeTestResponse(req, http.StatusBadRequest, "seq must be a positive integer"), nil
	})})
	if view, err := a.InboxRoomLine(tab.id, 0); err == nil {
		t.Fatalf("view = %+v，宿主说 seq 不合法时不该报成答案", view)
	}
}
