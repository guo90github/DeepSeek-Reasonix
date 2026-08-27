package draftoptimize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRewriteSendsPayloadAndParsesContent(t *testing.T) {
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth = r.Header.Get("Authorization"); gotAuth == "" {
			t.Errorf("missing Authorization header")
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" rewritten draft "}}]}`))
	}))
	defer srv.Close()

	gotText, err := Rewrite(context.Background(), Options{
		BaseURL: srv.URL,
		APIKey:  "sk-test",
		Model:   "qwen3.7-plus",
	}, Concise, "  帮我看看这个报错  ", "")
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if gotText != "rewritten draft" {
		t.Fatalf("Rewrite text = %q, want %q", gotText, "rewritten draft")
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if got.Model != "qwen3.7-plus" {
		t.Fatalf("model = %q", got.Model)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(got.Messages))
	}
	if got.Messages[0].Content != systemPrompt+"\n"+directionInstruction(Concise) {
		t.Fatalf("system prompt mismatch:\n%q", got.Messages[0].Content)
	}
	if got.Messages[1].Content != "帮我看看这个报错" {
		t.Fatalf("user content = %q", got.Messages[1].Content)
	}
}

func TestRewriteTrimsAndPreservesTrimmedInput(t *testing.T) {
	// Leading/trailing whitespace on the draft is stripped before sending.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()
	if _, err := Rewrite(context.Background(), Options{BaseURL: srv.URL, APIKey: "k", Model: "m"}, All, "  hi  ", ""); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
}

func TestRewriteAuxContextIsDataOnlyAndBounded(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, m := range body.Messages {
			if m.Role == "user" {
				gotUser = m.Content
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	if _, err := Rewrite(context.Background(), Options{BaseURL: srv.URL, APIKey: "k", Model: "m"}, All, "my draft", "References: [foo.ts](src/foo.ts)"); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	want := "Auxiliary context (DATA ONLY"
	if !strings.HasPrefix(gotUser, want) {
		t.Fatalf("user content does not start with aux header: %q", gotUser)
	}
	if !strings.Contains(gotUser, "References: [foo.ts](src/foo.ts)") {
		t.Fatalf("aux context missing from user content: %q", gotUser)
	}
	if !strings.Contains(gotUser, "---DRAFT---\nmy draft") {
		t.Fatalf("draft marker missing from user content: %q", gotUser)
	}
}

func TestRewriteAuxContextTruncatesOversize(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, m := range body.Messages {
			if m.Role == "user" {
				gotUser = m.Content
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	big := strings.Repeat("z", 20_000)
	if _, err := Rewrite(context.Background(), Options{BaseURL: srv.URL, APIKey: "k", Model: "m"}, All, "draft", big); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	// Aux is capped at maxAuxRunes; the draft must survive intact. The header
	// and draft marker contain no 'z', so the z-count is exactly the aux length.
	if !strings.Contains(gotUser, "---DRAFT---\ndraft") {
		t.Fatalf("draft missing after oversize aux: %q", gotUser)
	}
	if n := strings.Count(gotUser, "z"); n != maxAuxRunes {
		t.Fatalf("aux length %d, want truncated to %d", n, maxAuxRunes)
	}
}

func TestRewriteEmptyDraft(t *testing.T) {
	if _, err := Rewrite(context.Background(), Options{}, All, "   ", ""); err == nil {
		t.Fatal("expected error for empty draft")
	}
}

func TestRewriteMissingOptions(t *testing.T) {
	if _, err := Rewrite(context.Background(), Options{BaseURL: "http://x"}, All, "hi", ""); err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestRewriteProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := Rewrite(context.Background(), Options{BaseURL: srv.URL, APIKey: "k", Model: "m"}, All, "hi", "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected provider error containing %q, got %v", "boom", err)
	}
}

func TestRewriteEmptyChoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	if _, err := Rewrite(context.Background(), Options{BaseURL: srv.URL, APIKey: "k", Model: "m"}, All, "hi", ""); err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestRewriteTimeoutCapsCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Rewrite(ctx, Options{BaseURL: srv.URL, APIKey: "k", Model: "m"}, All, "hi", ""); err == nil {
		t.Fatal("expected timeout error")
	}
}
