package serve

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

func multipartBody(t *testing.T, field, name, content string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, w.FormDataContentType()
}

func uploadServe(t *testing.T, root, token string) *Server {
	t.Helper()
	ctrl, _ := rootedServeController(t, root, filepath.Join(t.TempDir(), "session.jsonl"))
	cfg := config.ServeConfig{}
	if token != "" {
		cfg = config.ServeConfig{AuthMode: "token", Token: token}
	}
	srv := New(ctrl, NewBroadcaster(), cfg)
	// The phone's server binds the wildcard address, so hostGuard admits any
	// Host here; these cases are about the content-type gate, not the host.
	srv.setListenAddr("0.0.0.0:8787")
	return srv
}

func TestUploadStoresSharedFileInTheWorkspace(t *testing.T) {
	root := t.TempDir()
	body, contentType := multipartBody(t, "file", "photo.jpg", "hello")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	uploadServe(t, root, "tok").Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /upload = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var got uploadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(got.Files))
	}
	dir := filepath.Join(root, ".reasonix", "uploads")
	if file := got.Files[0]; filepath.Dir(file.Path) != dir || !strings.HasSuffix(file.Name, "photo.jpg") {
		t.Fatalf("stored %+v, want a file below %s", file, dir)
	}
	content, err := os.ReadFile(got.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" || got.Files[0].Bytes != int64(len("hello")) {
		t.Fatalf("stored %q / %d bytes, want hello / 5", content, got.Files[0].Bytes)
	}
	// The workspace belongs to the user's repo: without this marker every shared
	// photo turns up as an untracked change in the change list.
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatalf("upload directory is not self-ignoring: %v", err)
	}
}

func TestUploadRefusesToEscapeTheUploadDirectory(t *testing.T) {
	root := t.TempDir()
	body, contentType := multipartBody(t, "file", `..\..\evil.txt`, "x")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	uploadServe(t, root, "tok").Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /upload = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var got uploadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".reasonix", "uploads")
	if len(got.Files) != 1 || filepath.Dir(got.Files[0].Path) != dir {
		t.Fatalf("stored %+v, want a file inside %s", got.Files, dir)
	}
	for _, escaped := range []string{filepath.Join(root, "evil.txt"), filepath.Join(filepath.Dir(root), "evil.txt")} {
		if _, err := os.Stat(escaped); err == nil {
			t.Fatalf("upload escaped the upload directory: %s", escaped)
		}
	}
}

func TestUploadWithoutAWorkspaceRootIsRejected(t *testing.T) {
	srv := uploadServe(t, "", "tok")
	body, contentType := multipartBody(t, "file", "photo.jpg", "hello")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /upload without a workspace root = %d, want 400", rec.Code)
	}
}

// TestUploadKeepsTheContentTypeGuard pins the two halves of the exemption: the
// multipart path exists only for a Bearer credential, and everything else still
// answers 415 like every other state-changing endpoint.
func TestUploadKeepsTheContentTypeGuard(t *testing.T) {
	root := t.TempDir()
	body, contentType := multipartBody(t, "file", "photo.jpg", "hello")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	uploadServe(t, root, "").Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("multipart POST without any credential = %d, want 415", rec.Code)
	}

	body, contentType = multipartBody(t, "file", "photo.jpg", "hello")
	req = httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	uploadServe(t, root, "tok").Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("a wrong Bearer token was accepted")
	}
}

func TestBearerAuthorizedRequiresTheSharedToken(t *testing.T) {
	srv := New(control.New(control.Options{}), NewBroadcaster(),
		config.ServeConfig{AuthMode: "token", Token: "tok"})
	cases := []struct {
		name   string
		bearer string
		want   bool
	}{
		{"shared token", "Bearer tok", true},
		{"wrong token", "Bearer nope", false},
		{"missing header", "", false},
		{"wrong scheme", "Basic tok", false},
		{"token without a scheme", "tok", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/upload", nil)
		if tc.bearer != "" {
			req.Header.Set("Authorization", tc.bearer)
		}
		if got := srv.bearerAuthorized(req); got != tc.want {
			t.Errorf("%s: bearerAuthorized = %v, want %v", tc.name, got, tc.want)
		}
	}

	// A cookie proves nothing here: a page the user visits can send one.
	cookie := httptest.NewRequest(http.MethodPost, "/upload", nil)
	cookie.AddCookie(&http.Cookie{Name: cookieToken, Value: "tok"})
	if srv.bearerAuthorized(cookie) {
		t.Error("a cookie was accepted as a Bearer credential")
	}

	// No configured token means no exemption exists to grant.
	none := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	if none.bearerAuthorized(httptest.NewRequest(http.MethodPost, "/upload", nil)) {
		t.Error("a server without a token granted the exemption")
	}
}
