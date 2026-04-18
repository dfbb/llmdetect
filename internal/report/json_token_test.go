package report_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/online"
	"github.com/ironarmor/llmdetect/internal/provider"
	"github.com/ironarmor/llmdetect/internal/report"
)

func TestWriteJSON_IncludesTokenFields(t *testing.T) {
	ledger := api.NewTokenLedger()
	ledger.Add("https://api.openai.com/v1", provider.TokenUsage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110})
	ledger.Add("https://api.xxx.com/v1", provider.TokenUsage{PromptTokens: 50, CompletionTokens: 5, TotalTokens: 55})

	cfg := &config.Config{
		Detection: config.DetectionConfig{BorderInputs: 5, QueriesPerInput: 10, TVThreshold: 0.4},
		Output:    config.OutputConfig{ReportDir: t.TempDir()},
	}
	onlineResults := []online.Result{
		{Endpoint: config.Endpoint{Name: "ch1", URL: "https://api.xxx.com/v1"}, Online: true},
	}

	params := report.ReportParams{
		Model:         "gpt-4o",
		RunAt:         time.Now(),
		Duration:      10,
		Cfg:           cfg,
		OnlineResults: onlineResults,
		Ledger:        ledger,
	}

	path, err := report.WriteJSON(params, cfg.Output.ReportDir)
	if err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	data, _ := os.ReadFile(path)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if _, ok := out["token_summary"]; !ok {
		t.Errorf("missing token_summary in report:\n%s", data)
	}
	if out["total_tokens"] == nil {
		t.Errorf("missing total_tokens in report:\n%s", data)
	}

	results, _ := out["results"].([]any)
	if len(results) == 0 {
		t.Fatal("no results")
	}
	ch1, _ := results[0].(map[string]any)
	if ch1["tokens_used"] == nil {
		t.Errorf("missing tokens_used in channel result:\n%s", data)
	}
}

func TestWriteJSON_OfflineChannelHasNoTokensUsed(t *testing.T) {
	ledger := api.NewTokenLedger()

	cfg := &config.Config{
		Detection: config.DetectionConfig{BorderInputs: 5, QueriesPerInput: 10, TVThreshold: 0.4},
		Output:    config.OutputConfig{ReportDir: t.TempDir()},
	}
	onlineResults := []online.Result{
		{Endpoint: config.Endpoint{Name: "ch1", URL: "https://api.xxx.com/v1"}, Online: false},
	}

	params := report.ReportParams{
		Model:         "gpt-4o",
		RunAt:         time.Now(),
		Cfg:           cfg,
		OnlineResults: onlineResults,
		Ledger:        ledger,
	}
	path, err := report.WriteJSON(params, cfg.Output.ReportDir)
	if err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	data, _ := os.ReadFile(path)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	results, _ := out["results"].([]any)
	ch1, _ := results[0].(map[string]any)
	if _, hasTokens := ch1["tokens_used"]; hasTokens {
		t.Errorf("offline channel should not have tokens_used: %v", ch1)
	}
}
