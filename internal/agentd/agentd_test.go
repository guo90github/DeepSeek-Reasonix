package agentd

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRegistryRoundTripUnderLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.json")
	rec := Record{Name: "ws-a", Root: `C:\ws\a`, Addr: "127.0.0.1:8788", PID: 42,
		TokenFile: "tok", State: StateRunning, StartedAt: time.Now().UTC()}
	if err := SaveRegistry(path, Registry{}.Upsert(rec)); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	got, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	found, ok := got.Find("ws-a")
	if !ok || found.PID != 42 || found.URL() != "http://127.0.0.1:8788" {
		t.Fatalf("round-tripped record = %+v, %v", found, ok)
	}
	if _, stillThere := got.Remove("ws-a"); !stillThere {
		t.Fatal("Remove reported a missing record")
	}
}

func TestRegistryMissingFileIsEmpty(t *testing.T) {
	got, err := LoadRegistry(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got.SchemaVersion != RegistrySchemaVersion || len(got.Instances) != 0 {
		t.Fatalf("registry = %+v, want an empty current-schema registry", got)
	}
}

func TestRegistryRefusesUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.json")
	body, _ := json.Marshal(map[string]any{"schemaVersion": RegistrySchemaVersion + 1})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil {
		t.Fatal("a future schemaVersion must be refused, not reinterpreted")
	}
}

func TestAllocateAddrInSkipsBoundPort(t *testing.T) {
	first := firstFreePort(t)
	held := net.JoinHostPort("127.0.0.1", strconv.Itoa(first))
	ln, err := net.Listen("tcp", held)
	if err != nil {
		t.Skipf("cannot hold the first port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	addr, err := AllocateAddrIn(first, first+2)
	if err != nil {
		t.Fatalf("AllocateAddrIn: %v", err)
	}
	if addr == held {
		t.Fatalf("allocated the bound port %s", addr)
	}
}

func TestReadPortFileTrimsTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.addr")
	if err := os.WriteFile(path, []byte("127.0.0.1:8790\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	addr, err := ReadPortFile(path)
	if err != nil {
		t.Fatalf("ReadPortFile: %v", err)
	}
	if addr != "127.0.0.1:8790" {
		t.Fatalf("addr = %q", addr)
	}
}

func TestServeEnvNamesOnlyWhatItKnows(t *testing.T) {
	bare := ServeEnv(Record{Addr: "127.0.0.1:8788"})
	if bare["REASONIX_SERVE_URL"] != "http://127.0.0.1:8788" {
		t.Fatalf("url = %q", bare["REASONIX_SERVE_URL"])
	}
	if _, ok := bare["REASONIX_SESSION_PATH"]; ok {
		t.Fatal("an unbound session must not be announced")
	}
	full := ServeEnv(Record{Addr: "127.0.0.1:8788", TokenFile: "tok", SessionPath: "s.jsonl"})
	for key, want := range map[string]string{
		"REASONIX_SERVE_TOKEN_FILE": "tok",
		"REASONIX_SESSION_PATH":     "s.jsonl",
	} {
		if full[key] != want {
			t.Fatalf("%s = %q, want %q", key, full[key], want)
		}
	}
}

// A probe must present the instance's token: an unauthenticated 200 would mean
// the manager and the coordinator disagree about the credential.
func TestProbeRequiresInstanceToken(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "ws.token")
	if err := os.WriteFile(tokenPath, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" || r.Header.Get("Authorization") != "Bearer s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	probe := &Process{Record: Record{Name: "ws-a", Addr: srv.Listener.Addr().String(), TokenFile: tokenPath}}
	if err := probe.Probe(context.Background(), srv.Client()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	probe.Record.TokenFile = filepath.Join(dir, "other.token")
	if err := os.WriteFile(probe.Record.TokenFile, []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := probe.Probe(context.Background(), srv.Client()); err == nil {
		t.Fatal("a wrong token must fail the probe")
	}
}

func firstFreePort(t *testing.T) int {
	t.Helper()
	for port := 18788; port < 18888; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port
	}
	t.Skip("no free port found for the test")
	return 0
}
