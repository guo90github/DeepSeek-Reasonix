package secrets

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/provider"
)

func TestRedactMasksCommonSecretShapes(t *testing.T) {
	in := strings.Join([]string{
		"DEEPSEEK_API_KEY=sk-real-secret-value-123456",
		"Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz",
		"token xoxb-123456789012-abcdefabcdef",
		"jwt eyJabc.def.ghi",
	}, "\n")

	got := Redact(in)
	for _, leaked := range []string{
		"sk-real-secret-value-123456",
		"ghp_abcdefghijklmnopqrstuvwxyz",
		"xoxb-123456789012-abcdefabcdef",
		"eyJabc.def.ghi",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("secret leaked %q in:\n%s", leaked, got)
		}
	}
	for _, want := range []string{"DEEPSEEK_API_KEY=sk-rea", "Authorization: Bearer [redacted]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted output missing %q:\n%s", want, got)
		}
	}
}

func TestRedactLongConcurrentTranscriptAvoidsRegexpBacktracking(t *testing.T) {
	const secret = "sk-real-secret-value-1234567890"
	var transcript strings.Builder
	for i := range 2_000 {
		fmt.Fprintf(&transcript, "message %d payload=%s DEEPSEEK_API_KEY=%s Authorization: Bearer %s\n", i, strings.Repeat("x", i%31), secret, secret)
	}
	input := transcript.String()

	const workers = 24
	const iterations = 20
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for range workers {
		wg.Go(func() {
			for range iterations {
				got := Redact(input)
				if strings.Contains(got, secret) {
					errs <- "long concurrent redaction leaked the test secret"
					return
				}
				if again := Redact(got); again != got {
					errs <- "long concurrent redaction was not idempotent"
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestRedactMasksJSONQuotedKeys(t *testing.T) {
	in := `http 401: {"access_token":"sk-live-secret","x-api-key":"header-secret","password":"pw-secret"}`
	got := Redact(in)
	for _, leaked := range []string{"sk-live-secret", "header-secret", "pw-secret"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("JSON credential leaked %q in:\n%s", leaked, got)
		}
	}
	if !strings.Contains(got, `"access_token":"`) || !strings.Contains(got, "http 401") {
		t.Fatalf("non-secret structure mangled:\n%s", got)
	}
	if again := Redact(got); again != got {
		t.Fatalf("JSON redaction not idempotent:\nonce:  %q\ntwice: %q", got, again)
	}
}

func TestRedactMasksCookieHeaderValues(t *testing.T) {
	in := "Cookie: session=cookie-secret\nSet-Cookie: sid=abc123def456; Path=/; HttpOnly"
	got := Redact(in)
	for _, leaked := range []string{"cookie-secret", "abc123def456"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("cookie value leaked %q in:\n%s", leaked, got)
		}
	}
	for _, want := range []string{"Cookie: session=[redacted]", "Set-Cookie: sid=[redacted]", "HttpOnly"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted output missing %q:\n%s", want, got)
		}
	}
	if again := Redact(got); again != got {
		t.Fatalf("cookie redaction not idempotent:\nonce:  %q\ntwice: %q", got, again)
	}
}

func TestRedactMasksNonBearerAuthorizationSchemes(t *testing.T) {
	in := strings.Join([]string{
		"Authorization: Basic dXNlcjpwYXNzd29yZA==",
		"Proxy-Authorization: Digest username-hash-abcdef0123456789",
		"Authorization: dXNlcjpwYXNzd29yZC1yYXc=",
	}, "\n")
	got := Redact(in)
	for _, leaked := range []string{"dXNlcjpwYXNzd29yZA==", "username-hash-abcdef0123456789", "dXNlcjpwYXNzd29yZC1yYXc="} {
		if strings.Contains(got, leaked) {
			t.Fatalf("authorization credential leaked %q:\n%s", leaked, got)
		}
	}
	for _, want := range []string{"Authorization: Basic [redacted]", "Digest [redacted]", "Authorization: [redacted]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted output missing %q:\n%s", want, got)
		}
	}
	if again := Redact(got); again != got {
		t.Fatalf("authorization redaction not idempotent:\nonce:  %q\ntwice: %q", got, again)
	}
}

func TestRedactMasksURLUserInfo(t *testing.T) {
	in := "proxy request failed: https://proxy-user:pa@ss@proxy.example.com:8443/connect"
	got := Redact(in)
	for _, leaked := range []string{"proxy-user", "pa", "ss"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("URL credential leaked %q in:\n%s", leaked, got)
		}
	}
	for _, want := range []string{"https://[redacted]@proxy.example.com:8443/connect", "proxy request failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted output missing %q:\n%s", want, got)
		}
	}
	if again := Redact(got); again != got {
		t.Fatalf("URL redaction not idempotent:\nonce:  %q\ntwice: %q", got, again)
	}
}

func TestRedactCredentialsForExternalErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		leaked []string
		want   string
	}{
		{
			name:   "prose api key",
			err:    errors.New("provider rejected api key: relaykey_abcdefghijklmn"),
			leaked: []string{"relaykey_abcdefghijklmn"},
			want:   "provider rejected api key:",
		},
		{
			name:   "partially masked token",
			err:    errors.New("provider rejected token ****ae54"),
			leaked: []string{"ae54"},
			want:   "provider rejected token",
		},
		{
			name:   "bearer token",
			err:    errors.New("upstream returned Authorization: Bearer abcdef0123456789abcdef"),
			leaked: []string{"abcdef0123456789abcdef"},
			want:   "upstream returned",
		},
		{
			name:   "proxy URL user info",
			err:    errors.New("dial https://proxy-user:pa@ss@proxy.example.com:8443: refused"),
			leaked: []string{"proxy-user", "pa", "ss"},
			want:   "proxy.example.com:8443",
		},
		{
			name:   "key value is idempotent",
			err:    errors.New("provider rejected DEEPSEEK_API_KEY=sk-real-secret-value-123456"),
			leaked: []string{"sk-real-secret-value-123456"},
			want:   "provider rejected DEEPSEEK_API_KEY=",
		},
		{
			name:   "opaque mixed case token",
			err:    errors.New("credential relayKeyAbcdefghijkl rejected"),
			leaked: []string{"relayKeyAbcdefghijkl"},
			want:   "credential",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactError(tt.err)
			for _, leaked := range tt.leaked {
				if strings.Contains(got, leaked) {
					t.Fatalf("credential leaked %q in %q", leaked, got)
				}
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("diagnostic context missing %q in %q", tt.want, got)
			}
			if again := RedactCredentials(got); again != got {
				t.Fatalf("external error redaction not idempotent:\nonce:  %q\ntwice: %q", got, again)
			}
		})
	}
	if got := RedactError(nil); got != "" {
		t.Fatalf("RedactError(nil) = %q, want empty", got)
	}
}

func TestRedactIsIdempotent(t *testing.T) {
	// The session save path re-redacts loaded (already-redacted) transcripts;
	// digest stability across load/save cycles requires a byte-for-byte no-op.
	in := strings.Join([]string{
		"DEEPSEEK_API_KEY=sk-real-secret-value-123456",
		"Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz",
		"DB_PWD='hunter2-swordfish'",
		"plain text with PWD=/home/user/project untouched",
	}, "\n")
	once := Redact(in)
	twice := Redact(once)
	if once != twice {
		t.Fatalf("Redact not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}

func TestRedactLeavesWorkingDirectoryPWDAlone(t *testing.T) {
	in := "PWD=/home/user/project\nOLDPWD=/home/user\nDB_PWD=hunter2-swordfish-123"
	got := Redact(in)
	if !strings.Contains(got, "PWD=/home/user/project") {
		t.Fatalf("POSIX PWD variable was mangled:\n%s", got)
	}
	if !strings.Contains(got, "OLDPWD=/home/user") {
		t.Fatalf("OLDPWD was mangled:\n%s", got)
	}
	if strings.Contains(got, "hunter2-swordfish-123") {
		t.Fatalf("DB_PWD value leaked:\n%s", got)
	}
}

func TestEnvKeySensitive(t *testing.T) {
	sensitive := []string{"DEEPSEEK_API_KEY", "GH_TOKEN", "AWS_SECRET_ACCESS_KEY", "DB_PASSWORD", "MYSQL_PWD", "NPM_TOKEN"}
	for _, key := range sensitive {
		if !EnvKeySensitive(key) {
			t.Errorf("EnvKeySensitive(%q) = false, want true", key)
		}
	}
	benign := []string{"PWD", "OLDPWD", "PATH", "HOME", "LANG", "GOPATH", "TERM"}
	for _, key := range benign {
		if EnvKeySensitive(key) {
			t.Errorf("EnvKeySensitive(%q) = true, want false", key)
		}
	}
}

func TestFilterEnvDropsSensitiveKeys(t *testing.T) {
	got := FilterEnv([]string{
		"PATH=/usr/bin",
		"DEEPSEEK_API_KEY=sk-real-secret-value-123456",
		"GH_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz",
		"PWD=/home/user/project",
		"HOME=/tmp/home",
	})
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "DEEPSEEK_API_KEY") || strings.Contains(joined, "GH_TOKEN") {
		t.Fatalf("sensitive env survived:\n%s", joined)
	}
	for _, want := range []string{"PATH=/usr/bin", "HOME=/tmp/home", "PWD=/home/user/project"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("non-sensitive env %q dropped:\n%s", want, joined)
		}
	}
}

func TestProcessEnvUnfilteredByDefault(t *testing.T) {
	t.Setenv("REASONIX_TEST_SECRET_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz")
	joined := strings.Join(ProcessEnv(), "\n")
	if !strings.Contains(joined, "REASONIX_TEST_SECRET_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("ProcessEnv filtered by default; filter_subprocess_env must be opt-in:\n%s", joined)
	}

	SetFilterSubprocessEnv(true)
	t.Cleanup(func() { SetFilterSubprocessEnv(false) })
	joined = strings.Join(ProcessEnv(), "\n")
	if strings.Contains(joined, "REASONIX_TEST_SECRET_TOKEN") {
		t.Fatalf("ProcessEnv leaked sensitive key with filtering enabled:\n%s", joined)
	}
}

func TestProcessEnvAlwaysFiltersRegisteredCredentialKeys(t *testing.T) {
	const key = "REASONIX_TEST_CUSTOM_PROVIDER_CREDENTIAL"
	t.Setenv(key, "opaque-provider-value")
	t.Setenv("REASONIX_TEST_BENIGN_ENV", "visible")
	RegisterCredentialEnvKeys([]string{key})

	joined := strings.Join(ProcessEnv(), "\n")
	if strings.Contains(joined, key+"=") || strings.Contains(joined, "opaque-provider-value") {
		t.Fatalf("registered provider credential survived in subprocess env:\n%s", joined)
	}
	if !strings.Contains(joined, "REASONIX_TEST_BENIGN_ENV=visible") {
		t.Fatalf("ordinary env was removed with opt-in filtering off:\n%s", joined)
	}
}

func TestRedactMessagesDoesNotMutateInput(t *testing.T) {
	const secret = "sk-real-secret-value-123456"
	msgs := []provider.Message{
		{
			Role:    provider.RoleAssistant,
			Content: "checking",
			ToolCalls: []provider.ToolCall{
				{ID: "call_1", Name: "bash", Arguments: `{"command":"echo DEEPSEEK_API_KEY=` + secret + `"}`},
			},
			MemoryCitations: []provider.MemoryCitation{{Note: "token " + secret}},
		},
		{Role: provider.RoleTool, ToolCallID: "call_1", Content: "DEEPSEEK_API_KEY=" + secret},
	}

	out := RedactMessages(msgs)

	// The redacted copy must not carry the raw secret...
	if strings.Contains(out[0].ToolCalls[0].Arguments, secret) || strings.Contains(out[1].Content, secret) {
		t.Fatalf("redacted copy leaked secret: %+v", out)
	}
	// ...and the input — live session history the model still replays — must
	// be untouched, including through the shared ToolCalls/MemoryCitations
	// backing arrays.
	if !strings.Contains(msgs[0].ToolCalls[0].Arguments, secret) {
		t.Fatalf("RedactMessages mutated the caller's ToolCalls: %q", msgs[0].ToolCalls[0].Arguments)
	}
	if !strings.Contains(msgs[0].MemoryCitations[0].Note, secret) {
		t.Fatalf("RedactMessages mutated the caller's MemoryCitations: %q", msgs[0].MemoryCitations[0].Note)
	}
	if !strings.Contains(msgs[1].Content, secret) {
		t.Fatalf("RedactMessages mutated the caller's Content: %q", msgs[1].Content)
	}
}

func TestRedactMasksInternalAddresses(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "private 10/8", in: "dialing 10.0.0.5 now", want: "dialing [redacted] now"},
		{name: "private 172.16/12", in: "peer 172.16.4.4", want: "peer [redacted]"},
		{name: "private 192.168/16", in: "host 192.168.1.100", want: "host [redacted]"},
		{name: "loopback ip", in: "listening on 127.0.0.1", want: "listening on [redacted]"},
		{name: "loopback name", in: "listening on localhost", want: "listening on [redacted]"},
		{name: "loopback ipv6", in: "listening on ::1 (v6)", want: "listening on [redacted] (v6)"},
		{name: "suffix local", in: "printer.local refused", want: "[redacted] refused"},
		{name: "suffix internal", in: "db.internal timed out", want: "[redacted] timed out"},
		{name: "suffix corp", in: "build.corp timed out", want: "[redacted] timed out"},
		{name: "suffix lan", in: "nas.lan unreachable", want: "[redacted] unreachable"},
		{name: "private ip with port", in: "cache 10.0.0.5:6379 down", want: "cache [redacted] down"},
		{name: "internal url ip", in: "GET http://10.0.0.5:8080/admin failed", want: "GET [redacted] failed"},
		{name: "internal url suffix host", in: "GET https://svc.internal/v1/health?x=1 failed", want: "GET [redacted] failed"},
		{name: "localhost url", in: "GET http://localhost:3000/ failed", want: "GET [redacted] failed"},
		{name: "ipv6 loopback url", in: "GET http://[::1]:8080/x failed", want: "GET [redacted] failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.in)
			if got != tt.want {
				t.Fatalf("Redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if again := Redact(got); again != got {
				t.Fatalf("internal-address redaction not idempotent: once %q, twice %q", got, again)
			}
		})
	}
}

func TestRedactPreservesPublicAddresses(t *testing.T) {
	// The load-bearing half of this change: public IPs, public domains, and
	// public URLs must survive byte-for-byte so summaries stay useful.
	for _, in := range []string{
		"resolve 8.8.8.8 for dns",
		"public dns 1.1.1.1 and 93.184.216.34",
		"just below the 172.16 range: 172.15.0.1",
		"just above the 172.31 range: 172.32.0.1",
		"not private because of a length: 172.160.0.1",
		"public domain api.deepseek.com",
		"public host my-host.example.org",
		"public url https://api.deepseek.com/v1/chat/completions",
		"public ip url https://8.8.8.8/dns-query",
		"public ip url https://172.15.0.1/health",
	} {
		if got := Redact(in); got != in {
			t.Fatalf("public address mangled: Redact(%q) = %q", in, got)
		}
	}
}

func TestInternalAddressRedactionReachesEntryPoints(t *testing.T) {
	if got := RedactError(errors.New("dial tcp 10.0.0.5:5432: connection refused")); strings.Contains(got, "10.0.0.5") {
		t.Fatalf("RedactError leaked internal address: %q", got)
	}
	msg := RedactMessage(provider.Message{
		Role:    provider.RoleTool,
		Content: "curl http://svc.corp/health -> ok host=db.internal",
	})
	if strings.Contains(msg.Content, "svc.corp") || strings.Contains(msg.Content, "db.internal") {
		t.Fatalf("RedactMessage leaked internal address: %q", msg.Content)
	}
}
