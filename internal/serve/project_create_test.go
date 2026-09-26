package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

func postJSON(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// httptest defaults the Host to example.com; hostGuard only admits loopback.
	req.Host = "127.0.0.1:8787"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// TestNewProjectWithoutHostIsNotImplemented pins the honest answer for a
// standalone serve: the foreground workspace root is fixed for the life of the
// process, so only a host with a window can register a project.
func TestNewProjectWithoutHostIsNotImplemented(t *testing.T) {
	srv := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	rec := postJSON(t, srv, "/new-project", `{"parent":"/tmp","name":"demo"}`)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("POST /new-project without a host = %d, want 501", rec.Code)
	}
}

func TestNewProjectValidatesItsInput(t *testing.T) {
	srv := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	called := false
	srv.SetProjectCreator(func(NewProjectRequest) (NewProjectResult, error) {
		called = true
		return NewProjectResult{Root: "/tmp/demo", SessionPath: "/tmp/demo/session.jsonl"}, nil
	})

	cases := []struct {
		name string
		body string
	}{
		{"malformed body", `{`},
		{"nothing to do", `{}`},
		{"name without a parent", `{"name":"demo"}`},
		{"parent without a name", `{"parent":"/tmp"}`},
		{"both spellings", `{"root":"/tmp/demo","parent":"/tmp","name":"demo"}`},
	}
	for _, tc := range cases {
		rec := postJSON(t, srv, "/new-project", tc.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, rec.Code)
		}
	}
	if called {
		t.Fatal("a rejected request reached the host")
	}
}

func TestNewProjectDelegatesToTheHost(t *testing.T) {
	srv := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	var seen NewProjectRequest
	srv.SetProjectCreator(func(req NewProjectRequest) (NewProjectResult, error) {
		seen = req
		return NewProjectResult{Root: "/tmp/demo", Name: "demo", SessionPath: "/tmp/demo/session.jsonl"}, nil
	})

	rec := postJSON(t, srv, "/new-project", `{"parent":" /tmp ","name":" demo "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /new-project = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	if seen.Parent != "/tmp" || seen.Name != "demo" {
		t.Fatalf("host saw %+v, want the trimmed parent and name", seen)
	}
	if !strings.Contains(rec.Body.String(), `"sessionPath":"/tmp/demo/session.jsonl"`) {
		t.Fatalf("response = %s, want the session the caller must switch to", rec.Body.String())
	}
}

func TestNewProjectRejectsAHostThatRegistersNoSession(t *testing.T) {
	srv := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	srv.SetProjectCreator(func(NewProjectRequest) (NewProjectResult, error) {
		return NewProjectResult{Root: "/tmp/demo"}, nil
	})
	rec := postJSON(t, srv, "/new-project", `{"root":"/tmp/demo"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("host without a session = %d, want 409", rec.Code)
	}
}
