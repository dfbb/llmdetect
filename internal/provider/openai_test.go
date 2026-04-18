package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestOpenAI_Type(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want openai", a.Type())
	}
}

func TestOpenAI_RequestPath(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	if a.RequestPath() != "/v1/chat/completions" {
		t.Errorf("got %q, want /v1/chat/completions", a.RequestPath())
	}
}

func TestOpenAI_Headers(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	h := a.Headers("sk-test")
	if h["Authorization"] != "Bearer sk-test" {
		t.Errorf("missing or wrong Authorization header: %v", h)
	}
	if h["Content-Type"] != "application/json" {
		t.Errorf("missing Content-Type header: %v", h)
	}
}

func TestOpenAI_BuildRequest(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	body, err := a.BuildRequest("gpt-4o", "hello", 1)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req["model"] != "gpt-4o" {
		t.Errorf("model: got %v, want gpt-4o", req["model"])
	}
	if req["temperature"] != float64(0) {
		t.Errorf("temperature: got %v, want 0", req["temperature"])
	}
	if req["max_tokens"] != float64(1) {
		t.Errorf("max_tokens: got %v, want 1", req["max_tokens"])
	}
}

func TestOpenAI_ParseResponse_WithUsage(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	body := []byte(`{
        "choices": [{"message": {"content": "world"}}],
        "usage": {"prompt_tokens": 5, "completion_tokens": 1, "total_tokens": 6}
    }`)
	tok, usage, err := a.ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if tok != "world" {
		t.Errorf("token: got %q, want world", tok)
	}
	if usage.PromptTokens != 5 || usage.CompletionTokens != 1 || usage.TotalTokens != 6 {
		t.Errorf("usage: got %+v, want {5 1 6}", usage)
	}
}

func TestOpenAI_ParseResponse_NoUsage(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	body := []byte(`{"choices": [{"message": {"content": "hi"}}]}`)
	tok, usage, err := a.ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if tok != "hi" {
		t.Errorf("token: got %q", tok)
	}
	if usage.TotalTokens != 0 {
		t.Errorf("expected zero usage, got %+v", usage)
	}
}

func TestOpenAI_ParseResponse_EmptyChoices(t *testing.T) {
	a := &provider.OpenAIAdapter{}
	_, _, err := a.ParseResponse([]byte(`{"choices": []}`))
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}
