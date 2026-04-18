package detector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/detector"
)

// alternatingServer returns token "A" or "B" alternating per call — every token is a border input.
func alternatingServer(t *testing.T) *httptest.Server {
	var count atomic.Int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		tok := "A"
		if n%2 == 0 {
			tok = "B"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": tok}}},
		})
	}))
}

// deterministicServer always returns the same token — no border inputs.
func deterministicServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "X"}}},
		})
	}))
}

func discoveryCfg(url, key string) (*config.Config, *config.ModelConfig) {
	cfg := &config.Config{
		Detection: config.DetectionConfig{
			BorderInputs:        3,
			DiscoveryCandidates: 20,
			QueriesPerInput:     6,
		},
		Concurrency: config.ConcurrencyConfig{
			MaxWorkersPerChannel: 4,
			RateLimitRPS:         100,
			TimeoutSeconds:       5,
			MaxRetries:           1,
		},
	}
	model := &config.ModelConfig{
		Model:    "gpt-4o",
		Official: config.Endpoint{Name: "official", URL: url, Key: key},
	}
	return cfg, model
}

func TestDiscover_EarlyStop(t *testing.T) {
	srv := alternatingServer(t)
	defer srv.Close()

	cfg, model := discoveryCfg(srv.URL, "sk-test")
	tokens := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	result, err := detector.Discover(context.Background(), cfg, model, tokens)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.BorderInputs) < cfg.Detection.BorderInputs {
		t.Errorf("expected at least %d BIs, got %d", cfg.Detection.BorderInputs, len(result.BorderInputs))
	}
}

func TestDiscover_Shortage(t *testing.T) {
	srv := deterministicServer(t)
	defer srv.Close()

	cfg, model := discoveryCfg(srv.URL, "sk-test")
	tokens := []string{"a", "b", "c"}

	result, err := detector.Discover(context.Background(), cfg, model, tokens)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if result.BorderInputsFound != 0 {
		t.Errorf("expected 0 BIs found, got %d", result.BorderInputsFound)
	}
	if !result.Shortage {
		t.Error("expected Shortage=true when BIs < target")
	}
}
