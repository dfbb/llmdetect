package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ironarmor/llmdetect/config"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfgPath := writeTempFile(t, "config.yaml", `
cache:
  ttl_hours: 2
detection:
  border_inputs: 10
  discovery_candidates: 1000
  queries_per_input: 5
  tv_threshold: 0.5
concurrency:
  max_workers_per_channel: 4
  rate_limit_rps: 3
  timeout_seconds: 10
  max_retries: 2
output:
  report_dir: "./out"
`)
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Cache.TTLHours != 2 {
		t.Errorf("TTLHours = %d, want 2", cfg.Cache.TTLHours)
	}
	if cfg.Detection.BorderInputs != 10 {
		t.Errorf("BorderInputs = %d, want 10", cfg.Detection.BorderInputs)
	}
}

func TestLoadModel_Validation(t *testing.T) {
	good := writeTempFile(t, "model.yaml", `
model: gpt-4o
official:
  name: "OpenAI"
  url: "https://api.openai.com/v1"
  key: "sk-test"
channels:
  - name: "Chan"
    url: "https://api.chan.com/v1"
    key: "sk-chan"
`)
	_, err := config.LoadModel(good)
	if err != nil {
		t.Fatalf("LoadModel good: %v", err)
	}

	bad := writeTempFile(t, "bad.yaml", `
model: ""
official:
  name: "X"
  url: ""
  key: "sk"
channels: []
`)
	_, err = config.LoadModel(bad)
	if err == nil {
		t.Fatal("expected error for empty model/url")
	}
}

func TestLoadModel_InvalidProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.yaml")
	content := `
model: gpt-4o
official:
  name: Test
  url: https://api.openai.com/v1
  key: sk-test
  provider: gemini
channels: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadModel(path)
	if err == nil {
		t.Fatal("expected error for invalid provider value")
	}
}

func TestLoadModel_ValidProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.yaml")
	content := `
model: gpt-4o
official:
  name: Test
  url: https://api.openai.com/v1
  key: sk-test
  provider: anthropic
channels:
  - name: ch1
    url: https://api.xxx.com/v1
    key: sk-x
    provider: openai
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.LoadModel(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Official.Provider != "anthropic" {
		t.Errorf("official provider: got %q, want anthropic", m.Official.Provider)
	}
	if m.Channels[0].Provider != "openai" {
		t.Errorf("channel provider: got %q, want openai", m.Channels[0].Provider)
	}
}
