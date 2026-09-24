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

func TestServeMCPEnvReportsWhatItCanName(t *testing.T) {
	st := newServeMCPEnvState("0.0.0.0:8787", "")
	if got := st.envForRoot("/w"); got["REASONIX_SERVE_URL"] != "http://127.0.0.1:8787" {
		t.Fatalf("serve URL = %q, want the loopback form", got["REASONIX_SERVE_URL"])
	}
	if _, ok := st.envForRoot("/w")["REASONIX_SESSION_PATH"]; ok {
		t.Fatal("a state without a session lookup must not name a session")
	}

	st.setServe(func(root string) string {
		if root == "/w" {
			return "/sessions/w.jsonl"
		}
		return ""
	}, false)
	if got := st.envForRoot("/w")["REASONIX_SESSION_PATH"]; got != "/sessions/w.jsonl" {
		t.Fatalf("session path = %q, want the single session of that root", got)
	}
	if _, ok := st.envForRoot("/other")["REASONIX_SESSION_PATH"]; ok {
		t.Fatal("an ambiguous root must not be reported as a precise address")
	}

	st.setBoundAddr("127.0.0.1:9123")
	if got := st.envForRoot("/w")["REASONIX_SERVE_URL"]; got != "http://127.0.0.1:9123" {
		t.Fatalf("serve URL after binding = %q, want the bound address", got)
	}
}

func TestServeMCPEnvNamesOnlyFileBackedTokens(t *testing.T) {
	withFile := newServeMCPEnvState("127.0.0.1:8787", " /home/u/serve.token ")
	if got := withFile.envForRoot("/w")["REASONIX_SERVE_TOKEN_FILE"]; got != "/home/u/serve.token" {
		t.Fatalf("token file = %q, want the trimmed --token-file path", got)
	}

	// Token auth without a file: the child cannot read the secret, so the
	// environment must not pretend otherwise.
	noFile := newServeMCPEnvState("127.0.0.1:8787", "")
	noFile.setServe(nil, true)
	if _, ok := noFile.envForRoot("/w")["REASONIX_SERVE_TOKEN_FILE"]; ok {
		t.Fatal("a token held in memory must not be advertised as a file")
	}
}
