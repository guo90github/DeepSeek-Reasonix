package cli

import "testing"

func TestLoopbackServeURL(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"127.0.0.1:8787":  "http://127.0.0.1:8787",
		"0.0.0.0:8787":    "http://127.0.0.1:8787",
		"[::]:8787":       "http://127.0.0.1:8787",
		"localhost:9000":  "http://localhost:9000",
		"192.168.1.5:443": "http://192.168.1.5:443",
	}
	for in, want := range cases {
		if got := loopbackServeURL(in); got != want {
			t.Fatalf("loopbackServeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestServeMCPEnvNamesTheOwningSession(t *testing.T) {
	st := newServeMCPEnvState("0.0.0.0:8787", "")
	if got := st.envForSession("/sessions/w.jsonl"); got["REASONIX_SERVE_URL"] != "http://127.0.0.1:8787" {
		t.Fatalf("serve URL = %q, want the loopback form", got["REASONIX_SERVE_URL"])
	}
	if got := st.envForSession("/sessions/w.jsonl")["REASONIX_SESSION_PATH"]; got != "/sessions/w.jsonl" {
		t.Fatalf("session path = %q, want the controller's own session", got)
	}
	// No session yet: naming none is honest, and Serve refuses an addressed wake
	// it cannot place instead of dropping it into the foreground.
	if _, ok := st.envForSession("")["REASONIX_SESSION_PATH"]; ok {
		t.Fatal("a host with no session yet must not name one")
	}

	st.setBoundAddr("127.0.0.1:9123")
	if got := st.envForSession("/sessions/w.jsonl")["REASONIX_SERVE_URL"]; got != "http://127.0.0.1:9123" {
		t.Fatalf("serve URL after binding = %q, want the bound address", got)
	}
}

func TestServeMCPEnvNamesOnlyFileBackedTokens(t *testing.T) {
	withFile := newServeMCPEnvState("127.0.0.1:8787", " /home/u/serve.token ")
	if got := withFile.envForSession("/sessions/w.jsonl")["REASONIX_SERVE_TOKEN_FILE"]; got != "/home/u/serve.token" {
		t.Fatalf("token file = %q, want the trimmed --token-file path", got)
	}

	// Token auth without a file: the child cannot read the secret, so the
	// environment must not pretend otherwise.
	noFile := newServeMCPEnvState("127.0.0.1:8787", "")
	noFile.setTokenAuth(true)
	if _, ok := noFile.envForSession("/sessions/w.jsonl")["REASONIX_SERVE_TOKEN_FILE"]; ok {
		t.Fatal("a token held in memory must not be advertised as a file")
	}
}
