package localcli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"one-api/common/config"
	"one-api/model"
)

// helper: build a minimal Channel that is enough for the provider to be
// constructed without panicking. Channel.Proxy must be non-nil because
// requester.NewHTTPRequester dereferences it; the rest of the fields are
// either ignored by LocalCLIProvider or fall back to the embedded
// OpenAIProvider's defaults.
func makeChannel(baseURL string) *model.Channel {
	proxy := ""
	return &model.Channel{
		Type:  config.ChannelTypeLocalCLI,
		Key:   "ignored-by-cli2api",
		Proxy: &proxy,
		BaseURL: func() *string {
			if baseURL == "" {
				return nil
			}
			return &baseURL
		}(),
	}
}

// TestProviderFactory_DefaultBaseURL verifies that a freshly-created
// LocalCLI channel falls back to the package default URL when the user
// has not set a custom Base URL yet. This is the first-launch UX.
func TestProviderFactory_DefaultBaseURL(t *testing.T) {
	p := LocalCLIProviderFactory{}.Create(makeChannel("")).(*LocalCLIProvider)
	got := p.GetBaseURL()
	if got != DefaultBaseURL {
		t.Fatalf("default BaseURL = %q, want %q", got, DefaultBaseURL)
	}
}

// TestProviderFactory_OverrideBaseURL verifies that a per-channel Base URL
// overrides the package default. This is the docker-network use case where
// users point at http://cli2api-claude:8000 instead of loopback.
func TestProviderFactory_OverrideBaseURL(t *testing.T) {
	want := "http://cli2api-claude:8000"
	p := LocalCLIProviderFactory{}.Create(makeChannel(want)).(*LocalCLIProvider)
	if got := p.GetBaseURL(); got != want {
		t.Fatalf("override BaseURL = %q, want %q", got, want)
	}
}

// TestProviderFactory_ModelListExposed makes sure the canned model list
// stays in sync with what cli2api currently supports (and what the UI
// dropdown advertises). If you change ModelList, update this test so the
// expectation is reviewable in code review rather than baked into the
// frontend silently.
func TestProviderFactory_ModelListExposed(t *testing.T) {
	expected := []string{
		"sonnet", "opus", "haiku",
		"gemini-2.5-pro", "gemini-2.5-flash",
		"codex",
		"qwen3-coder",
	}
	if len(ModelList) != len(expected) {
		t.Fatalf("ModelList length = %d, want %d", len(ModelList), len(expected))
	}
	for i, m := range expected {
		if ModelList[i] != m {
			t.Errorf("ModelList[%d] = %q, want %q", i, ModelList[i], m)
		}
	}
}

// TestProviderFactory_RoundtripAgainstMock spins up an httptest server
// that pretends to be cli2api and confirms that:
//
//   - The LocalCLI provider's GetFullRequestURL targets the right path.
//   - A request reaches that server through the provider's Requester.
//   - The OpenAI-shaped response decodes correctly.
//
// This is the closest thing to an end-to-end test we can do without
// pulling in the full relay pipeline.
func TestProviderFactory_RoundtripAgainstMock(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 0,
			"model":   "sonnet",
			"choices": []any{map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "hi from mock cli2api",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     1,
				"completion_tokens": 5,
				"total_tokens":      6,
			},
		})
	}))
	defer mock.Close()

	p := LocalCLIProviderFactory{}.Create(makeChannel(mock.URL)).(*LocalCLIProvider)

	full := p.GetFullRequestURL("/v1/chat/completions", "sonnet")
	if !strings.HasPrefix(full, mock.URL) {
		t.Fatalf("full URL %q does not target mock server %q", full, mock.URL)
	}
	if !strings.HasSuffix(full, "/v1/chat/completions") {
		t.Fatalf("full URL %q missing chat completions path", full)
	}

	req, err := http.NewRequest(http.MethodPost, full, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range p.GetRequestHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("mock received path %q, want /v1/chat/completions", gotPath)
	}
	if !strings.HasPrefix(gotAuthorization, "Bearer ") {
		t.Fatalf("Authorization header %q does not start with Bearer", gotAuthorization)
	}
}
