package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
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
