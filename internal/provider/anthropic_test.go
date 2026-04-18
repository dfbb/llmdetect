package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestAnthropic_Type(t *testing.T) {
	a := &provider.AnthropicAdapter{}
	if a.Type() != provider.ProviderAnthropic {
		t.Errorf("got %v, want anthropic", a.Type())
	}
}

func TestAnthropic_RequestPath(t *testing.T) {
	a := &provider.AnthropicAdapter{}
	if a.RequestPath() != "/v1/messages" {
		t.Errorf("got %q, want /v1/messages", a.RequestPath())
	}
}

func TestAnthropic_Headers(t *testing.T) {
	a := &provider.AnthropicAdapter{}
	h := a.Headers("sk-ant-test")
	if h["x-api-key"] != "sk-ant-test" {
		t.Errorf("missing x-api-key: %v", h)
	}
	if h["anthropic-version"] != "2023-06-01" {
		t.Errorf("missing anthropic-version: %v", h)
	}
	if h["Content-Type"] != "application/json" {
		t.Errorf("missing Content-Type: %v", h)
	}
}

func TestAnthropic_BuildRequest(t *testing.T) {
	a := &provider.AnthropicAdapter{}
	body, err := a.BuildRequest("claude-3-5-sonnet-20241022", "hello", 1)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req["model"] != "claude-3-5-sonnet-20241022" {
		t.Errorf("model: got %v", req["model"])
	}
	if req["max_tokens"] != float64(1) {
		t.Errorf("max_tokens: got %v, want 1", req["max_tokens"])
	}
	if req["temperature"] != float64(0) {
		t.Errorf("temperature: got %v, want 0", req["temperature"])
	}
}

func TestAnthropic_ParseResponse_WithUsage(t *testing.T) {
	a := &provider.AnthropicAdapter{}
	body := []byte(`{
        "content": [{"type": "text", "text": "hi"}],
        "usage": {"input_tokens": 10, "output_tokens": 2}
    }`)
	tok, usage, err := a.ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if tok != "hi" {
		t.Errorf("token: got %q, want hi", tok)
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 2 || usage.TotalTokens != 12 {
		t.Errorf("usage: got %+v, want {10 2 12}", usage)
	}
}

func TestAnthropic_ParseResponse_EmptyContent(t *testing.T) {
	a := &provider.AnthropicAdapter{}
	_, _, err := a.ParseResponse([]byte(`{"content": []}`))
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}
