package online_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/online"
)

func makeServer(statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": ""}},
				},
			})
		} else {
			w.WriteHeader(statusCode)
		}
	}))
}

func TestCheckAll_PartialOnline(t *testing.T) {
	onlineSrv := makeServer(http.StatusOK)
	offlineSrv := makeServer(http.StatusInternalServerError)
	defer onlineSrv.Close()
	defer offlineSrv.Close()

	cfg := &config.Config{
		Concurrency: config.ConcurrencyConfig{TimeoutSeconds: 5, MaxRetries: 1},
	}
	endpoints := []config.Endpoint{
		{Name: "online", URL: onlineSrv.URL, Key: "sk-test"},
		{Name: "offline", URL: offlineSrv.URL, Key: "sk-test"},
	}

	newClient := func(ep config.Endpoint) *api.Client {
		return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1)
	}
	results := online.CheckAll(cfg, "gpt-4o", endpoints, newClient)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	byName := make(map[string]online.Result)
	for _, r := range results {
		byName[r.Endpoint.Name] = r
	}
	if !byName["online"].Online {
		t.Error("online server should be online")
	}
	if byName["offline"].Online {
		t.Error("offline server should be offline")
	}
}
