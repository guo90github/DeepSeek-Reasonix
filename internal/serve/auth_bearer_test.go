package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/config"
)

// A non-browser caller authenticates with its token as a bearer credential:
// the supervisor that manages a serve has no cookie jar and cannot be handed a
// URL fragment, so this is the path agentd presents.
func TestTokenModeValidBearerHeader(t *testing.T) {
	ag := newAuthGate(config.ServeConfig{AuthMode: "token", Token: "secret"})
	ts := httptest.NewServer(ag.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestTokenModeRejectsWrongBearerHeader(t *testing.T) {
	ag := newAuthGate(config.ServeConfig{AuthMode: "token", Token: "secret"})
	ts := httptest.NewServer(ag.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer ts.Close()

	for _, header := range []string{"Bearer wrong", "Basic secret", "secret"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/status", nil)
		req.Header.Set("Authorization", header)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status = %d, want 401", header, resp.StatusCode)
		}
	}
}
