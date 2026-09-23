package serve

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/stats"
)

func newListenerTestServer(t *testing.T) *Server {
	t.Helper()
	return newListenerTestServerWithConfig(t, config.ServeConfig{})
}

func newListenerTestServerWithConfig(t *testing.T, cfg config.ServeConfig) *Server {
	t.Helper()
	// Server construction starts the process-wide usage projection, even with
	// a mock controller. Fence it on both sides of this test's home override.
	closeUsage := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := stats.CloseUsageCatalogs(ctx); err != nil {
			t.Fatalf("close usage catalog: %v", err)
		}
	}
	closeUsage()
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Cleanup(closeUsage)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Sink:       bc,
		Label:      "listener-test",
		SessionDir: t.TempDir(),
	})
	t.Cleanup(func() { ctrl.Close() })
	return New(ctrl, bc, cfg)
}

func waitForHTTP(t *testing.T, addr string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for range 100 {
		resp, err := client.Get("http://" + addr + "/assets/logo-wordmark.svg")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET logo = %d, want 200", resp.StatusCode)
			}
			return
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server never came up on %s: %v", addr, lastErr)
}

// RunGracefulListener must serve on the caller-supplied listener so callers
// that need the real bound address (--addr 127.0.0.1:0 with --port-file) can
// listen first, record ln.Addr(), then hand the listener over.
func TestRunGracefulListenerServesOnProvidedListener(t *testing.T) {
	srv := newListenerTestServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.RunGracefulListener(ctx, ln) }()

	waitForHTTP(t, ln.Addr().String())

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunGracefulListener returned a shutdown error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("RunGracefulListener did not return after ctx cancel")
	}
}

// RunGraceful must keep its historical contract (bind from the addr string
// itself) now that it delegates to RunGracefulListener.
func TestRunGracefulStillListensFromAddr(t *testing.T) {
	srv := newListenerTestServer(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	addr := probe.Addr().String()
	probe.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.RunGraceful(ctx, addr) }()

	waitForHTTP(t, addr)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunGraceful returned a shutdown error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("RunGraceful did not return after ctx cancel")
	}
}

// POST /shutdown is how a supervisor retires a managed instance: the serve loop
// must end on its own — with the credential and content type the manager
// actually sends — instead of the supervisor killing the tree mid-turn.
func TestShutdownRequestEndsTheServeLoop(t *testing.T) {
	srv := newListenerTestServerWithConfig(t, config.ServeConfig{AuthMode: "token", Token: "secret"})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	done := make(chan error, 1)
	go func() { done <- srv.RunGracefulListener(t.Context(), ln) }()

	waitForHTTP(t, addr)

	if code := postShutdown(t, addr, ""); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /shutdown = %d, want 401", code)
	}
	select {
	case err := <-done:
		t.Fatalf("an unauthenticated request ended the loop: %v", err)
	default:
	}

	if code := postShutdown(t, addr, "secret"); code != http.StatusAccepted {
		t.Fatalf("POST /shutdown = %d, want 202", code)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunGracefulListener returned a shutdown error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("POST /shutdown did not end the serve loop")
	}
}

// postShutdown sends what a managed instance's supervisor sends, so the test
// takes the same admission chain: bearer credential plus a JSON body.
func postShutdown(t *testing.T, addr, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/shutdown", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /shutdown: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
