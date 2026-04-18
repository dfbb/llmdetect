package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestQuery_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "world", "role": "assistant"},
					"finish_reason": "stop",
				},
			},
			"model": "gpt-4o",
		})
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "sk-test", 5, 3)
	tok, err := c.QueryOnce(context.Background(), "gpt-4o", "hello")
	if err != nil {
		t.Fatalf("QueryOnce: %v", err)
	}
	if tok != "world" {
		t.Errorf("got token %q, want %q", tok, "world")
	}
}

func TestQuery_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "bad-key", 5, 1)
	_, err := c.QueryOnce(context.Background(), "gpt-4o", "hello")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestQuery_AnthropicAdapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Errorf("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("missing anthropic-version header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "world"}},
			"usage":   map[string]any{"input_tokens": 5, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	c := api.NewClientFull(srv.URL, "sk-ant-test", 5, 1,
		&provider.AnthropicAdapter{}, nil)
	tok, err := c.QueryOnce(context.Background(), "claude-3-5-sonnet-20241022", "hello")
	if err != nil {
		t.Fatalf("QueryOnce: %v", err)
	}
	if tok != "world" {
		t.Errorf("got %q, want world", tok)
	}
}

func TestQuery_LedgerAccumulates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "x"}},
			},
			"usage": map[string]any{
				"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12,
			},
		})
	}))
	defer srv.Close()

	ledger := api.NewTokenLedger()
	c := api.NewClientFull(srv.URL, "sk-test", 5, 1,
		&provider.OpenAIAdapter{}, ledger)

	for i := 0; i < 3; i++ {
		if _, err := c.QueryOnce(context.Background(), "gpt-4o", "hi"); err != nil {
			t.Fatalf("QueryOnce %d: %v", i, err)
		}
	}

	total := ledger.Total()
	if total.PromptTokens != 30 || total.TotalTokens != 36 {
		t.Errorf("ledger total wrong: got %+v, want {30 6 36}", total)
	}
}
