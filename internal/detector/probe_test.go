package detector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/cache"
	"github.com/ironarmor/llmdetect/internal/detector"
)

func identicalServer(t *testing.T, response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": response}}},
		})
	}))
}

func TestProbeChannels_TVDistance(t *testing.T) {
	// Channel always returns "world" — same as official distribution
	chanSrv := identicalServer(t, "world")
	defer chanSrv.Close()

	cfg := &config.Config{
		Detection: config.DetectionConfig{QueriesPerInput: 5, TVThreshold: 0.4},
		Concurrency: config.ConcurrencyConfig{
			MaxWorkersPerChannel: 2, RateLimitRPS: 100,
			TimeoutSeconds: 5, MaxRetries: 1,
		},
	}
	model := &config.ModelConfig{Model: "gpt-4o"}
	channels := []config.Endpoint{
		{Name: "identical", URL: chanSrv.URL, Key: "sk-test"},
	}
	bis := []cache.BorderInput{
		{Prompt: "hello", OfficialDistribution: map[string]int{"world": 10}},
	}

	results := detector.ProbeChannels(context.Background(), cfg, model, channels, bis)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.TVDistance > cfg.Detection.TVThreshold {
		t.Errorf("identical channel: TV = %f, expected < threshold %f", r.TVDistance, cfg.Detection.TVThreshold)
	}
	if r.Verdict != "original" {
		t.Errorf("expected verdict=original, got %s", r.Verdict)
	}
}
