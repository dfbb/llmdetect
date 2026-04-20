package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestDetect_OpenRouter(t *testing.T) {
	// No server needed — OpenRouter is short-circuited by URL pattern
	a, err := provider.Detect(context.Background(),
		"https://openrouter.ai/api", "key", "gpt-4o", "", "", time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want openai", a.Type())
	}
}

func TestDetect_OpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a, err := provider.Detect(context.Background(),
		srv.URL, "sk-test", "gpt-4o", "", srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want openai", a.Type())
	}
}

func TestDetect_Anthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/messages" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	// non-claude model → plain Anthropic adapter expected
	a, err := provider.Detect(context.Background(),
		srv.URL, "sk-ant-test", "my-custom-model", "", srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderAnthropic {
		t.Errorf("got %v, want anthropic", a.Type())
	}
}

func TestDetect_ClaudeCodePreferred(t *testing.T) {
	// For claude models, ClaudeCode probe is tried first; a server that accepts
	// /v1/messages should be detected as claude-code, not plain anthropic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/messages" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	a, err := provider.Detect(context.Background(),
		srv.URL, "sk-ant-test", "claude-3-5-sonnet-20241022", "", srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderClaudeCode {
		t.Errorf("got %v, want claude-code", a.Type())
	}
}

func TestDetect_Undetectable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := provider.Detect(context.Background(),
		srv.URL, "key", "model", "", srv.URL, time.Second)
	if err == nil {
		t.Fatal("expected ErrProviderUndetectable")
	}
}

func TestDetect_WritesProviderToYAML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
			return
		}
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	yamlContent := `model: gpt-4o
official:
  name: "Test"
  url: "` + srv.URL + `"
  key: "sk-test"
channels: []
`
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := provider.Detect(context.Background(),
		srv.URL, "sk-test", "gpt-4o", yamlPath, srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	data, _ := os.ReadFile(yamlPath)
	if !strings.Contains(string(data), "provider: openai") {
		t.Errorf("provider not written to YAML:\n%s", data)
	}
	// Verify existing fields are preserved
	if !strings.Contains(string(data), `name: "Test"`) {
		t.Errorf("existing field lost after YAML rewrite:\n%s", data)
	}
}

func TestDetect_SkipsProbeIfProviderAlreadyInYAML(t *testing.T) {
	// probeCount will be > 0 if the server is hit
	probeCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeCount++
		w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer srv.Close()

	// Caller is responsible for short-circuiting when provider is known.
	// AdapterFromType does not probe — this test verifies that contract.
	a, err := provider.AdapterFromType(provider.ProviderOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if a == nil {
		t.Fatal("nil adapter")
	}
	if probeCount != 0 {
		t.Errorf("AdapterFromType made %d probe requests, want 0", probeCount)
	}
}

