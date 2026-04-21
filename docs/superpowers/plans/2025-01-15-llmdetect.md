# llmdetect Go CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI tool that detects whether LLM API reseller channels are running the original model, using the B3IT border-input statistical method.

**Architecture:** Cobra CLI with three subcommands (`detect`, `refresh-cache`, `online-check`). The official API's border inputs are discovered concurrently and cached as JSON. Suspect channels are probed with the same inputs; TV distance is computed per channel. Channels with different root domains run in parallel goroutines; same root domain run serially. Stale-cache fallback keeps detect working when the official API is temporarily down.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, `github.com/fatih/color`, `golang.org/x/time/rate`, `golang.org/x/net/publicsuffix`, stdlib `net/http`, `embed`, `sync`

---

## File Map

| File | Responsibility |
|---|---|
| `cmd/llmdetect/main.go` | Cobra root + subcommand registration |
| `config/loader.go` | Load and validate `config.yaml` + model YAML |
| `config/types.go` | Config structs |
| `internal/api/client.go` | OpenAI-compatible HTTP client, retry/backoff |
| `internal/cache/cache.go` | Cache JSON read/write/TTL/corruption handling |
| `internal/detector/tvdistance.go` | TV distance pure function |
| `internal/detector/discover.go` | Phase 1: concurrent border input discovery |
| `internal/detector/probe.go` | Phase 2: channel probing + domain grouping |
| `internal/online/checker.go` | Concurrent online-check ping |
| `internal/report/terminal.go` | Colored terminal table output |
| `internal/report/json.go` | JSON report file writer |
| `tokens/fallback.txt` | Embedded token candidate list (~5000 entries) |
| `config.yaml` | Default runtime parameters |
| `models/gpt4o.yaml` | Example model/channel config |
| `go.mod` / `go.sum` | Module definition |

---

### Task 1: Project Scaffold

**Files:**
- Create: `/Users/dfbb/Sites/myidea/llmdetect/llmdetect/go.mod`
- Create: `/Users/dfbb/Sites/myidea/llmdetect/llmdetect/config.yaml`
- Create: `/Users/dfbb/Sites/myidea/llmdetect/llmdetect/models/gpt4o.yaml`

- [ ] **Step 1: Initialize module**

```bash
cd /Users/dfbb/Sites/myidea/llmdetect
mkdir llmdetect && cd llmdetect
go mod init github.com/ironarmor/llmdetect
```

Expected: `go.mod` created with `module github.com/ironarmor/llmdetect`

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/spf13/cobra@latest
go get gopkg.in/yaml.v3@latest
go get github.com/fatih/color@latest
go get golang.org/x/time@latest
go get golang.org/x/net@latest
```

- [ ] **Step 3: Create default config.yaml**

```yaml
cache:
  ttl_hours: 1

detection:
  border_inputs: 20
  discovery_candidates: 5000
  queries_per_input: 30
  tv_threshold: 0.4

concurrency:
  max_workers_per_channel: 10
  rate_limit_rps: 5
  timeout_seconds: 15
  max_retries: 3

output:
  report_dir: "./reports"
```

- [ ] **Step 4: Create models/gpt4o.yaml**

```yaml
model: gpt-4o

official:
  name: "OpenAI Official"
  url: "https://api.openai.com/v1"
  key: "sk-placeholder"

channels:
  - name: "Channel-A"
    url: "https://api.example-a.com/v1"
    key: "sk-placeholder-a"
  - name: "Channel-B"
    url: "https://api.example-b.com/v1"
    key: "sk-placeholder-b"
```

- [ ] **Step 5: Create directory structure**

```bash
mkdir -p cmd/llmdetect config internal/api internal/cache \
         internal/detector internal/online internal/report tokens
touch cmd/llmdetect/main.go config/loader.go config/types.go \
      internal/api/client.go internal/cache/cache.go \
      internal/detector/tvdistance.go internal/detector/discover.go \
      internal/detector/probe.go internal/online/checker.go \
      internal/report/terminal.go internal/report/json.go \
      tokens/fallback.txt
```

- [ ] **Step 6: Commit**

```bash
git init
git add .
git commit -m "feat: project scaffold for llmdetect Go CLI"
```

---

### Task 2: Config Types and Loader

**Files:**
- Create: `config/types.go`
- Create: `config/loader.go`
- Create: `config/loader_test.go`

- [ ] **Step 1: Write failing test**

```go
// config/loader_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./config/... -v
```

Expected: FAIL — `config` package doesn't exist yet

- [ ] **Step 3: Write config/types.go**

```go
package config

type CacheConfig struct {
	TTLHours int `yaml:"ttl_hours"`
}

type DetectionConfig struct {
	BorderInputs        int     `yaml:"border_inputs"`
	DiscoveryCandidates int     `yaml:"discovery_candidates"`
	QueriesPerInput     int     `yaml:"queries_per_input"`
	TVThreshold         float64 `yaml:"tv_threshold"`
}

type ConcurrencyConfig struct {
	MaxWorkersPerChannel int     `yaml:"max_workers_per_channel"`
	RateLimitRPS         float64 `yaml:"rate_limit_rps"`
	TimeoutSeconds       int     `yaml:"timeout_seconds"`
	MaxRetries           int     `yaml:"max_retries"`
}

type OutputConfig struct {
	ReportDir string `yaml:"report_dir"`
}

type Config struct {
	Cache       CacheConfig       `yaml:"cache"`
	Detection   DetectionConfig   `yaml:"detection"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
	Output      OutputConfig      `yaml:"output"`
}

type Endpoint struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Key  string `yaml:"key"`
}

type ModelConfig struct {
	Model    string     `yaml:"model"`
	Official Endpoint   `yaml:"official"`
	Channels []Endpoint `yaml:"channels"`
}
```

- [ ] **Step 4: Write config/loader.go**

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

func LoadModel(path string) (*ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read model %s: %w", path, err)
	}
	var m ModelConfig
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse model %s: %w", path, err)
	}
	if m.Model == "" {
		return nil, fmt.Errorf("model.model is required in %s", path)
	}
	if m.Official.URL == "" {
		return nil, fmt.Errorf("model.official.url is required in %s", path)
	}
	if m.Official.Key == "" {
		return nil, fmt.Errorf("model.official.key is required in %s", path)
	}
	return &m, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./config/... -v
```

Expected: PASS — both `TestLoadConfig_Defaults` and `TestLoadModel_Validation` pass

- [ ] **Step 6: Commit**

```bash
git add config/
git commit -m "feat: config loader with YAML types and validation"
```

---

### Task 3: API Client

**Files:**
- Create: `internal/api/client.go`
- Create: `internal/api/client_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/api/client_test.go
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
				{"message": map[string]any{"content": "", "role": "assistant"},
					"finish_reason": "stop",
					"delta":         map[string]any{},
					"logprobs":      nil,
				},
			},
			"model": "gpt-4o",
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "sk-test", 5, 3)
	tok, err := c.QueryOnce(context.Background(), "gpt-4o", "hello")
	if err != nil {
		t.Fatalf("QueryOnce: %v", err)
	}
	_ = tok
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -v
```

Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write internal/api/client.go**

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	timeout    time.Duration
	maxRetries int
	http       *http.Client
}

func NewClient(baseURL, apiKey string, timeoutSeconds, maxRetries int) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
		maxRetries: maxRetries,
		http:       &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
	Temperature float64     `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// QueryOnce sends a single chat completion request and returns the response token.
func (c *Client) QueryOnce(ctx context.Context, model, prompt string) (string, error) {
	body, _ := json.Marshal(chatRequest{
		Model:       model,
		Messages:    []chatMessage{{Role: "user", Content: prompt}},
		MaxTokens:   1,
		Temperature: 0,
	})

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(math.Pow(2, float64(attempt-1))) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("API returned 401 unauthorized — check your API key for %s", c.baseURL)
		}
		if resp.StatusCode >= 500 {
			b, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(b))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
		}

		var result chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			lastErr = fmt.Errorf("decode response: %w", err)
			continue
		}
		if len(result.Choices) == 0 {
			return "", fmt.Errorf("empty choices in response")
		}
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("all retries failed: %w", lastErr)
}

// Ping sends a minimal request and returns true if the endpoint responds with 200.
func (c *Client) Ping(ctx context.Context, model string) bool {
	_, err := c.QueryOnce(ctx, model, "hi")
	return err == nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/api/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat: OpenAI-compatible HTTP client with retry/backoff"
```

---

### Task 4: Token Fallback List

**Files:**
- Modify: `tokens/fallback.txt`

The Python project already has a fallback token list. Extract it.

- [ ] **Step 1: Extract tokens from Python project**

```bash
cd /Users/dfbb/Sites/myidea/llmdetect/token-efficient-change-detection-llm-apis
python3 -c "
import sys
sys.path.insert(0, 'b3it_monitoring/src')
from b3it_monitoring.bi.download_tokenizers import get_best_single_token_strings
tokens = get_best_single_token_strings()
for t in tokens:
    print(t)
" > /Users/dfbb/Sites/myidea/llmdetect/llmdetect/tokens/fallback.txt
wc -l /Users/dfbb/Sites/myidea/llmdetect/llmdetect/tokens/fallback.txt
```

Expected: several thousand lines in `tokens/fallback.txt`

If the Python import fails, use this alternative to generate a minimal list:

```bash
# Alternative: extract printable ASCII tokens (sufficient for testing)
python3 -c "
import string
# Common single-token English words and symbols
tokens = list(string.ascii_letters) + list(string.digits)
tokens += ['hello','world','the','a','is','it','of','and','to','in',
           'that','was','he','she','they','we','you','I','not','but',
           'yes','no','ok','hi','bye','cat','dog','one','two','three']
for t in tokens:
    print(t)
" > /Users/dfbb/Sites/myidea/llmdetect/llmdetect/tokens/fallback.txt
```

- [ ] **Step 2: Verify the file**

```bash
wc -l /Users/dfbb/Sites/myidea/llmdetect/llmdetect/tokens/fallback.txt
head -5 /Users/dfbb/Sites/myidea/llmdetect/llmdetect/tokens/fallback.txt
```

Expected: at least 100 non-empty lines

- [ ] **Step 3: Commit**

```bash
cd /Users/dfbb/Sites/myidea/llmdetect/llmdetect
git add tokens/fallback.txt
git commit -m "feat: add embedded token fallback list"
```

---

### Task 5: TV Distance Calculator

**Files:**
- Create: `internal/detector/tvdistance.go`
- Create: `internal/detector/tvdistance_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/detector/tvdistance_test.go
package detector_test

import (
	"math"
	"testing"

	"github.com/ironarmor/llmdetect/internal/detector"
)

func TestComputeTV_Identical(t *testing.T) {
	p := map[string]int{"a": 10, "b": 20}
	q := map[string]int{"a": 10, "b": 20}
	got := detector.ComputeTV(p, q)
	if math.Abs(got) > 1e-9 {
		t.Errorf("identical distributions: TV = %f, want 0", got)
	}
}

func TestComputeTV_Disjoint(t *testing.T) {
	p := map[string]int{"a": 10}
	q := map[string]int{"b": 10}
	got := detector.ComputeTV(p, q)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("disjoint distributions: TV = %f, want 1.0", got)
	}
}

func TestComputeTV_Partial(t *testing.T) {
	// P: a=1/2, b=1/2   Q: a=1/4, b=3/4
	// TV = 0.5 * (|1/2-1/4| + |1/2-3/4|) = 0.5 * (1/4 + 1/4) = 0.25
	p := map[string]int{"a": 2, "b": 2}
	q := map[string]int{"a": 1, "b": 3}
	got := detector.ComputeTV(p, q)
	if math.Abs(got-0.25) > 1e-9 {
		t.Errorf("partial: TV = %f, want 0.25", got)
	}
}

func TestAverageTV(t *testing.T) {
	tvs := []float64{0.1, 0.3, 0.5}
	got := detector.AverageTV(tvs)
	if math.Abs(got-0.3) > 1e-9 {
		t.Errorf("AverageTV = %f, want 0.3", got)
	}
}

func TestAverageTV_Empty(t *testing.T) {
	got := detector.AverageTV(nil)
	if got != 0 {
		t.Errorf("AverageTV(nil) = %f, want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/detector/... -run TestComputeTV -v
```

Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write internal/detector/tvdistance.go**

```go
package detector

// ComputeTV computes the Total Variation distance between two token count distributions.
// TV(P,Q) = 0.5 * sum(|P(x) - Q(x)|) for all x, where P and Q are normalized to probabilities.
func ComputeTV(p, q map[string]int) float64 {
	totalP := countSum(p)
	totalQ := countSum(q)
	if totalP == 0 || totalQ == 0 {
		return 0
	}

	keys := make(map[string]struct{})
	for k := range p {
		keys[k] = struct{}{}
	}
	for k := range q {
		keys[k] = struct{}{}
	}

	var sum float64
	for k := range keys {
		pProb := float64(p[k]) / float64(totalP)
		qProb := float64(q[k]) / float64(totalQ)
		diff := pProb - qProb
		if diff < 0 {
			diff = -diff
		}
		sum += diff
	}
	return 0.5 * sum
}

func countSum(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// AverageTV returns the mean of a slice of TV distances.
func AverageTV(tvs []float64) float64 {
	if len(tvs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range tvs {
		sum += v
	}
	return sum / float64(len(tvs))
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/detector/... -run TestComputeTV -run TestAverageTV -v
```

Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/detector/tvdistance.go internal/detector/tvdistance_test.go
git commit -m "feat: TV distance calculator with tests"
```

---

### Task 6: Cache Layer

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/cache_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/cache/cache_test.go
package cache_test

import (
	"path/filepath"
	"testing"
	"time"
	"os"
	"errors"

	"github.com/ironarmor/llmdetect/internal/cache"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")

	c := cache.New(path)
	original := &cache.CacheFile{
		Model:       "gpt-4o",
		OfficialURL: "https://api.openai.com/v1",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		ExpiresAt:   time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		BorderInputs: []cache.BorderInput{
			{Prompt: "hello", OfficialDistribution: map[string]int{"world": 5, "there": 3}},
		},
	}

	if err := c.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := c.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Model != original.Model {
		t.Errorf("Model: got %q want %q", loaded.Model, original.Model)
	}
	if len(loaded.BorderInputs) != 1 {
		t.Errorf("BorderInputs: got %d want 1", len(loaded.BorderInputs))
	}
}

func TestIsExpired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")
	c := cache.New(path)

	future := &cache.CacheFile{
		Model: "m", OfficialURL: "u",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	if err := c.Save(future); err != nil {
		t.Fatal(err)
	}
	if c.IsExpired() {
		t.Error("fresh cache should not be expired")
	}

	past := &cache.CacheFile{
		Model: "m", OfficialURL: "u",
		CreatedAt: time.Now().Add(-2 * time.Hour).UTC(),
		ExpiresAt: time.Now().Add(-time.Hour).UTC(),
	}
	if err := c.Save(past); err != nil {
		t.Fatal(err)
	}
	if !c.IsExpired() {
		t.Error("stale cache should be expired")
	}
}

func TestCorruptedCacheDeletedAndReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")
	if err := os.WriteFile(path, []byte("{corrupted json"), 0644); err != nil {
		t.Fatal(err)
	}

	c := cache.New(path)
	_, err := c.Load()
	if !errors.Is(err, cache.ErrCorrupted) {
		t.Fatalf("expected ErrCorrupted, got: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("corrupted cache file should have been deleted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cache/... -v
```

Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write internal/cache/cache.go**

```go
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

var ErrCorrupted = errors.New("cache file is corrupted")
var ErrNotFound = errors.New("cache file not found")

type BorderInput struct {
	Prompt               string         `json:"prompt"`
	OfficialDistribution map[string]int `json:"official_distribution"`
}

type CacheFile struct {
	Model        string        `json:"model"`
	OfficialURL  string        `json:"official_url"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	BorderInputs []BorderInput `json:"border_inputs"`
}

type Cache struct {
	path string
}

func New(path string) *Cache {
	return &Cache{path: path}
}

func (c *Cache) Save(cf *CacheFile) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0644); err != nil {
		return fmt.Errorf("write cache %s: %w", c.path, err)
	}
	return nil
}

func (c *Cache) Load() (*CacheFile, error) {
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var cf CacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		os.Remove(c.path)
		return nil, ErrCorrupted
	}
	return &cf, nil
}

// IsExpired returns true if the cache file does not exist or its ExpiresAt has passed.
func (c *Cache) IsExpired() bool {
	cf, err := c.Load()
	if err != nil {
		return true
	}
	return time.Now().After(cf.ExpiresAt)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cache/... -v
```

Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cache/
git commit -m "feat: cache layer with TTL, corruption handling, and round-trip test"
```

---

### Task 7: Online Checker

**Files:**
- Create: `internal/online/checker.go`
- Create: `internal/online/checker_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/online/checker_test.go
package online_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"encoding/json"

	"github.com/ironarmor/llmdetect/config"
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

	results := online.CheckAll(cfg, "gpt-4o", endpoints)
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/online/... -v
```

Expected: FAIL

- [ ] **Step 3: Write internal/online/checker.go**

```go
package online

import (
	"context"
	"sync"
	"time"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
)

type Result struct {
	Endpoint config.Endpoint
	Online   bool
	Latency  time.Duration
	Error    string
}

// CheckAll concurrently pings all given endpoints and returns one Result per endpoint.
func CheckAll(cfg *config.Config, model string, endpoints []config.Endpoint) []Result {
	results := make([]Result, len(endpoints))
	var wg sync.WaitGroup
	for i, ep := range endpoints {
		wg.Add(1)
		go func(idx int, endpoint config.Endpoint) {
			defer wg.Done()
			c := api.NewClient(endpoint.URL, endpoint.Key,
				cfg.Concurrency.TimeoutSeconds, 1)
			ctx, cancel := context.WithTimeout(context.Background(),
				time.Duration(cfg.Concurrency.TimeoutSeconds)*time.Second)
			defer cancel()
			start := time.Now()
			ok := c.Ping(ctx, model)
			results[idx] = Result{
				Endpoint: endpoint,
				Online:   ok,
				Latency:  time.Since(start),
			}
		}(i, ep)
	}
	wg.Wait()
	return results
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/online/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/online/
git commit -m "feat: concurrent online-check with per-endpoint results"
```

---

### Task 8: Border Input Discovery (Phase 1)

**Files:**
- Create: `internal/detector/discover.go`
- Create: `internal/detector/discover_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/detector/discover_test.go
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

// alternatingServer returns token "A" or "B" alternating per call, making every token a border input.
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
		// It's okay if we got enough — just verify we got at least what we asked for
		// unless there weren't enough candidates
		if len(tokens)*3 >= cfg.Detection.BorderInputs {
			t.Errorf("expected at least %d BIs, got %d", cfg.Detection.BorderInputs, len(result.BorderInputs))
		}
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
	if result.BorderInputsFound >= cfg.Detection.BorderInputs {
		t.Errorf("expected shortage, got %d BIs (target %d)", result.BorderInputsFound, cfg.Detection.BorderInputs)
	}
	if !result.Shortage {
		t.Error("expected Shortage=true when BIs < target")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/detector/... -run TestDiscover -v
```

Expected: FAIL

- [ ] **Step 3: Write internal/detector/discover.go**

```go
package detector

import (
	"context"
	"sync"

	"golang.org/x/time/rate"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
)

type DiscoverResult struct {
	BorderInputs      []cache.BorderInput
	BorderInputsFound int
	Shortage          bool
}

// Discover runs Phase 1: find border inputs from the given token list using the official API.
func Discover(ctx context.Context, cfg *config.Config, model *config.ModelConfig, tokens []string) (*DiscoverResult, error) {
	client := api.NewClient(model.Official.URL, model.Official.Key,
		cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
	limiter := rate.NewLimiter(rate.Limit(cfg.Concurrency.RateLimitRPS), 1)

	target := cfg.Detection.BorderInputs
	if len(tokens) > cfg.Detection.DiscoveryCandidates {
		tokens = tokens[:cfg.Detection.DiscoveryCandidates]
	}

	type biCandidate struct {
		prompt  string
		outputs map[string]struct{}
	}

	sem := make(chan struct{}, cfg.Concurrency.MaxWorkersPerChannel)
	var mu sync.Mutex
	var found []biCandidate
	done := make(chan struct{})

	var wg sync.WaitGroup
	for _, tok := range tokens {
		select {
		case <-done:
			goto phaseTwo
		default:
		}

		wg.Add(1)
		go func(prompt string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			outputs := make(map[string]struct{})
			for i := 0; i < 3; i++ {
				if err := limiter.Wait(ctx); err != nil {
					return
				}
				select {
				case <-done:
					return
				default:
				}
				resp, err := client.QueryOnce(ctx, model.Model, prompt)
				if err != nil {
					return
				}
				outputs[resp] = struct{}{}
			}

			if len(outputs) >= 2 {
				mu.Lock()
				if len(found) < target {
					found = append(found, biCandidate{prompt: prompt, outputs: outputs})
					if len(found) >= target {
						select {
						case <-done:
						default:
							close(done)
						}
					}
				}
				mu.Unlock()
			}
		}(tok)
	}

phaseTwo:
	wg.Wait()

	shortage := len(found) < target
	borderInputsFound := len(found)

	// Phase 1b: build official distribution for each BI
	bis := make([]cache.BorderInput, 0, len(found))
	for _, cand := range found {
		dist := make(map[string]int)
		for i := 0; i < cfg.Detection.QueriesPerInput; i++ {
			if err := limiter.Wait(ctx); err != nil {
				break
			}
			resp, err := client.QueryOnce(ctx, model.Model, cand.prompt)
			if err != nil {
				continue
			}
			dist[resp]++
		}
		bis = append(bis, cache.BorderInput{
			Prompt:               cand.prompt,
			OfficialDistribution: dist,
		})
	}

	return &DiscoverResult{
		BorderInputs:      bis,
		BorderInputsFound: borderInputsFound,
		Shortage:          shortage,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/detector/... -run TestDiscover -v -timeout 30s
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/detector/discover.go internal/detector/discover_test.go
git commit -m "feat: Phase 1 border input discovery with early stop and shortage handling"
```

---

### Task 9: Channel Prober (Phase 2)

**Files:**
- Create: `internal/detector/probe.go`
- Create: `internal/detector/probe_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/detector/probe_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/detector/... -run TestProbeChannels -v
```

Expected: FAIL

- [ ] **Step 3: Write internal/detector/probe.go**

```go
package detector

import (
	"context"
	"sync"

	"golang.org/x/net/publicsuffix"
	"golang.org/x/time/rate"
	"net/url"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
)

type ChannelResult struct {
	Endpoint    config.Endpoint
	TVDistance  float64
	PerInputTV  []float64
	Verdict     string // "original", "spoofed"
}

// ProbeChannels runs Phase 2: probe each channel and compute TV distance vs official distribution.
// Channels with different root domains run in parallel; same root domain run serially.
func ProbeChannels(ctx context.Context, cfg *config.Config, model *config.ModelConfig,
	channels []config.Endpoint, bis []cache.BorderInput) []ChannelResult {

	groups := groupByDomain(channels)

	resultsMu := sync.Mutex{}
	allResults := make([]ChannelResult, 0, len(channels))

	var wg sync.WaitGroup
	for _, group := range groups {
		wg.Add(1)
		go func(grp []config.Endpoint) {
			defer wg.Done()
			for _, ch := range grp {
				r := probeOne(ctx, cfg, model, ch, bis)
				resultsMu.Lock()
				allResults = append(allResults, r)
				resultsMu.Unlock()
			}
		}(group)
	}
	wg.Wait()
	return allResults
}

func probeOne(ctx context.Context, cfg *config.Config, model *config.ModelConfig,
	ch config.Endpoint, bis []cache.BorderInput) ChannelResult {

	client := api.NewClient(ch.URL, ch.Key, cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
	limiter := rate.NewLimiter(rate.Limit(cfg.Concurrency.RateLimitRPS), 1)
	sem := make(chan struct{}, cfg.Concurrency.MaxWorkersPerChannel)

	type biResult struct {
		idx int
		tv  float64
	}
	results := make([]float64, len(bis))
	var wg sync.WaitGroup

	for i, bi := range bis {
		wg.Add(1)
		go func(idx int, b cache.BorderInput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dist := make(map[string]int)
			for j := 0; j < cfg.Detection.QueriesPerInput; j++ {
				if err := limiter.Wait(ctx); err != nil {
					break
				}
				resp, err := client.QueryOnce(ctx, model.Model, b.Prompt)
				if err != nil {
					continue
				}
				dist[resp]++
			}
			results[idx] = ComputeTV(b.OfficialDistribution, dist)
		}(i, bi)
	}
	wg.Wait()

	avg := AverageTV(results)
	verdict := "original"
	if avg >= cfg.Detection.TVThreshold {
		verdict = "spoofed"
	}
	return ChannelResult{
		Endpoint:   ch,
		TVDistance: avg,
		PerInputTV: results,
		Verdict:    verdict,
	}
}

func rootDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(u.Hostname())
	if err != nil {
		return u.Hostname()
	}
	return domain
}

func groupByDomain(channels []config.Endpoint) [][]config.Endpoint {
	order := make([]string, 0)
	groups := make(map[string][]config.Endpoint)
	for _, ch := range channels {
		d := rootDomain(ch.URL)
		if _, seen := groups[d]; !seen {
			order = append(order, d)
		}
		groups[d] = append(groups[d], ch)
	}
	result := make([][]config.Endpoint, 0, len(order))
	for _, d := range order {
		result = append(result, groups[d])
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/detector/... -run TestProbeChannels -v -timeout 30s
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/detector/probe.go internal/detector/probe_test.go
git commit -m "feat: Phase 2 channel probing with domain-based parallelization"
```

---

### Task 10: Reports

**Files:**
- Create: `internal/report/json.go`
- Create: `internal/report/terminal.go`

- [ ] **Step 1: Write internal/report/json.go**

```go
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
)

type JSONReport struct {
	Model           string            `json:"model"`
	RunAt           time.Time         `json:"run_at"`
	DurationSeconds float64           `json:"duration_seconds"`
	Config          JSONReportConfig   `json:"config"`
	BorderInputsFound int             `json:"border_inputs_found,omitempty"`
	CacheStale      bool              `json:"cache_stale,omitempty"`
	CacheAgeMinutes int               `json:"cache_age_minutes,omitempty"`
	Results         []JSONChannelResult `json:"results"`
}

type JSONReportConfig struct {
	BorderInputs    int     `json:"border_inputs"`
	QueriesPerInput int     `json:"queries_per_input"`
	TVThreshold     float64 `json:"tv_threshold"`
}

type JSONChannelResult struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	Online     bool      `json:"online"`
	TVDistance *float64  `json:"tv_distance"`
	Verdict    string    `json:"verdict"`
	PerInputTV []float64 `json:"per_input_tv,omitempty"`
}

type ReportParams struct {
	Model             string
	RunAt             time.Time
	Duration          float64
	Cfg               *config.Config
	BorderInputsFound int
	Shortage          bool
	CacheStale        bool
	CacheAgeMinutes   int
	OnlineResults     []online.Result
	ProbeResults      []detector.ChannelResult
}

func WriteJSON(params ReportParams, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}

	onlineMap := make(map[string]bool)
	for _, r := range params.OnlineResults {
		onlineMap[r.Endpoint.URL] = r.Online
	}
	probeMap := make(map[string]detector.ChannelResult)
	for _, r := range params.ProbeResults {
		probeMap[r.Endpoint.URL] = r
	}

	var results []JSONChannelResult
	for _, or_ := range params.OnlineResults {
		cr := JSONChannelResult{
			Name:   or_.Endpoint.Name,
			URL:    or_.Endpoint.URL,
			Online: or_.Online,
		}
		if or_.Online {
			if pr, ok := probeMap[or_.Endpoint.URL]; ok {
				tv := pr.TVDistance
				cr.TVDistance = &tv
				cr.Verdict = pr.Verdict
				cr.PerInputTV = pr.PerInputTV
			}
		} else {
			cr.Verdict = "offline"
		}
		results = append(results, cr)
	}

	rep := JSONReport{
		Model:           params.Model,
		RunAt:           params.RunAt,
		DurationSeconds: params.Duration,
		Config: JSONReportConfig{
			BorderInputs:    params.Cfg.Detection.BorderInputs,
			QueriesPerInput: params.Cfg.Detection.QueriesPerInput,
			TVThreshold:     params.Cfg.Detection.TVThreshold,
		},
		Results: results,
	}
	if params.Shortage {
		rep.BorderInputsFound = params.BorderInputsFound
	}
	if params.CacheStale {
		rep.CacheStale = true
		rep.CacheAgeMinutes = params.CacheAgeMinutes
	}

	ts := params.RunAt.Format("2006-01-02T15-04-05")
	filename := filepath.Join(dir, fmt.Sprintf("%s_%s.json", params.Model, ts))
	data, _ := json.MarshalIndent(rep, "", "  ")
	return filename, os.WriteFile(filename, data, 0644)
}
```

- [ ] **Step 2: Write internal/report/terminal.go**

```go
package report

import (
	"fmt"
	"time"

	"github.com/fatih/color"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
)

var (
	checkMark = color.GreenString("✓")
	crossMark = color.RedString("✗")
	bold      = color.New(color.Bold).SprintFunc()
	separator = "────────────────────────────────────────────────────────────────"
)

func PrintSummary(params ReportParams, cfg *config.Config) {
	fmt.Printf("\n%s  model: %s   border inputs: %d   queries/input: %d   threshold: %.2f\n",
		bold("llmdetect"),
		bold(params.Model),
		cfg.Detection.BorderInputs,
		cfg.Detection.QueriesPerInput,
		cfg.Detection.TVThreshold,
	)
	fmt.Printf("run at: %s   duration: %.1fs\n\n",
		params.RunAt.Format(time.RFC3339),
		params.Duration,
	)

	if params.CacheStale {
		color.Yellow("⚠  cache is stale (age: %d minutes) — refresh failed, using old data", params.CacheAgeMinutes)
	}
	if params.Shortage {
		color.Yellow("⚠  border_inputs_found: %d (target: %d)", params.BorderInputsFound, cfg.Detection.BorderInputs)
	}

	fmt.Printf("%s\n%s\n", bold("Online Check"), separator)
	for _, r := range params.OnlineResults {
		mark := checkMark
		suffix := ""
		if !r.Online {
			mark = crossMark
			suffix = color.RedString("  [offline, skipped]")
		}
		fmt.Printf("  %s  %-20s  %s%s\n", mark, r.Endpoint.Name, r.Endpoint.URL, suffix)
	}
	fmt.Println()

	probeMap := make(map[string]detector.ChannelResult)
	for _, r := range params.ProbeResults {
		probeMap[r.Endpoint.URL] = r
	}

	fmt.Printf("%s\n%s\n", bold("Detection Results"), separator)
	fmt.Printf("  %-22s  %-10s  %s\n", "Channel", "TV Dist", "Verdict")
	fmt.Printf("  %s\n", separator[:60])
	for _, or_ := range params.OnlineResults {
		if !or_.Online {
			continue
		}
		pr, ok := probeMap[or_.Endpoint.URL]
		if !ok {
			continue
		}
		mark := checkMark + color.GreenString(" original")
		if pr.Verdict == "spoofed" {
			mark = crossMark + color.RedString(" spoofed")
		}
		fmt.Printf("  %-22s  %-10.3f  %s\n", or_.Endpoint.Name, pr.TVDistance, mark)
	}
	fmt.Printf("%s\n", separator)
}
```

- [ ] **Step 3: Build to verify it compiles**

```bash
go build ./internal/report/...
```

Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/report/
git commit -m "feat: terminal and JSON report output"
```

---

### Task 11: Cobra CLI Commands

**Files:**
- Create: `cmd/llmdetect/main.go`

- [ ] **Step 1: Write cmd/llmdetect/main.go**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/cache"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
	"github.com/ironarmor/llmdetect/internal/report"
	"github.com/ironarmor/llmdetect/tokens"
)

var (
	flagModel  string
	flagConfig string
)

func main() {
	root := &cobra.Command{
		Use:   "llmdetect",
		Short: "Detect whether LLM API channels are running the original model",
	}
	root.PersistentFlags().StringVarP(&flagModel, "file", "f", "", "model YAML file (required)")
	root.PersistentFlags().StringVarP(&flagConfig, "config", "c", "./config.yaml", "config YAML file")

	root.AddCommand(cmdOnlineCheck(), cmdRefreshCache(), cmdDetect())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadBoth() (*config.Config, *config.ModelConfig) {
	if flagModel == "" {
		fmt.Fprintln(os.Stderr, "error: -f/--file is required")
		os.Exit(1)
	}
	cfg, err := config.LoadConfig(flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	model, err := config.LoadModel(flagModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading model: %v\n", err)
		os.Exit(1)
	}
	return cfg, model
}

func cachePath(modelFile string) string {
	ext := filepath.Ext(modelFile)
	return strings.TrimSuffix(modelFile, ext) + ".cache"
}

func cmdOnlineCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "online-check",
		Short: "Check whether the official API and all channels are reachable",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			all := append([]config.Endpoint{model.Official}, model.Channels...)
			results := online.CheckAll(cfg, model.Model, all)
			for _, r := range results {
				mark := "✓"
				if !r.Online {
					mark = "✗"
				}
				fmt.Printf("  %s  %-20s  %s\n", mark, r.Endpoint.Name, r.Endpoint.URL)
			}
		},
	}
}

func cmdRefreshCache() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh-cache",
		Short: "Discover border inputs from the official API and update the cache file",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			tokenList := tokens.Load()
			ctx := context.Background()

			fmt.Println("Discovering border inputs from official API...")
			result, err := detector.Discover(ctx, cfg, model, tokenList)
			if err != nil {
				fmt.Fprintf(os.Stderr, "discovery failed: %v\n", err)
				os.Exit(1)
			}

			now := time.Now().UTC()
			cf := &cache.CacheFile{
				Model:        model.Model,
				OfficialURL:  model.Official.URL,
				CreatedAt:    now,
				ExpiresAt:    now.Add(time.Duration(cfg.Cache.TTLHours) * time.Hour),
				BorderInputs: result.BorderInputs,
			}
			c := cache.New(cachePath(flagModel))
			if err := c.Save(cf); err != nil {
				fmt.Fprintf(os.Stderr, "save cache: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Cache saved: %d border inputs (target: %d)\n",
				result.BorderInputsFound, cfg.Detection.BorderInputs)
			if result.Shortage {
				fmt.Printf("Warning: only %d BIs found (target: %d)\n",
					result.BorderInputsFound, cfg.Detection.BorderInputs)
			}
		},
	}
}

func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Run online-check, load/refresh cache, probe all channels, and output a report",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			ctx := context.Background()
			startTime := time.Now()

			// Step 1: online-check
			all := append([]config.Endpoint{model.Official}, model.Channels...)
			onlineResults := online.CheckAll(cfg, model.Model, all)
			var onlineChannels []config.Endpoint
			for _, r := range onlineResults {
				if r.Online && r.Endpoint.URL != model.Official.URL {
					onlineChannels = append(onlineChannels, r.Endpoint)
				}
			}

			// Step 2: cache
			c := cache.New(cachePath(flagModel))
			var cf *cache.CacheFile
			cacheStale := false
			cacheAgeMinutes := 0

			if c.IsExpired() {
				tokenList := tokens.Load()
				result, err := detector.Discover(ctx, cfg, model, tokenList)
				if err != nil {
					// try stale fallback
					old, loadErr := c.Load()
					if loadErr != nil {
						fmt.Fprintf(os.Stderr, "refresh failed and no stale cache: %v\n", err)
						os.Exit(1)
					}
					cf = old
					cacheStale = true
					cacheAgeMinutes = int(time.Since(old.CreatedAt).Minutes())
					fmt.Fprintf(os.Stderr, "Warning: refresh failed (%v), using stale cache (%d min old)\n",
						err, cacheAgeMinutes)
				} else {
					now := time.Now().UTC()
					cf = &cache.CacheFile{
						Model:        model.Model,
						OfficialURL:  model.Official.URL,
						CreatedAt:    now,
						ExpiresAt:    now.Add(time.Duration(cfg.Cache.TTLHours) * time.Hour),
						BorderInputs: result.BorderInputs,
					}
					c.Save(cf)
				}
			} else {
				var err error
				cf, err = c.Load()
				if err != nil {
					fmt.Fprintf(os.Stderr, "load cache: %v\n", err)
					os.Exit(1)
				}
			}

			// Step 3: probe channels
			probeResults := detector.ProbeChannels(ctx, cfg, model, onlineChannels, cf.BorderInputs)

			duration := time.Since(startTime).Seconds()
			params := report.ReportParams{
				Model:             model.Model,
				RunAt:             startTime,
				Duration:          duration,
				Cfg:               cfg,
				BorderInputsFound: len(cf.BorderInputs),
				CacheStale:        cacheStale,
				CacheAgeMinutes:   cacheAgeMinutes,
				OnlineResults:     onlineResults[1:], // skip official
				ProbeResults:      probeResults,
			}

			report.PrintSummary(params, cfg)
			outPath, err := report.WriteJSON(params, cfg.Output.ReportDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "write report: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Report written to: %s\n", outPath)
		},
	}
}
```

- [ ] **Step 2: Create tokens/tokens.go for the embed**

```go
// tokens/tokens.go
package tokens

import (
	_ "embed"
	"strings"
)

//go:embed fallback.txt
var fallbackData string

// Load returns all non-empty lines from the embedded token list.
func Load() []string {
	lines := strings.Split(fallbackData, "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}
```

- [ ] **Step 3: Build the binary**

```bash
go build -o llmdetect ./cmd/llmdetect/
```

Expected: `./llmdetect` binary created with no errors

- [ ] **Step 4: Commit**

```bash
git add cmd/llmdetect/main.go tokens/tokens.go
git commit -m "feat: Cobra CLI with detect/refresh-cache/online-check commands"
```

---

### Task 12: Integration Test

**Files:**
- Create: `integration_test.go`

- [ ] **Step 1: Write integration test**

```go
// integration_test.go
package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/cache"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
	"github.com/ironarmor/llmdetect/tokens"
)

func respondWith(tok string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": tok}}},
		})
	})
}

func nondeterministic() http.Handler {
	var n atomic.Int64
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := "A"
		if n.Add(1)%2 == 0 {
			tok = "B"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": tok}}},
		})
	})
}

func TestEndToEnd_CacheHit(t *testing.T) {
	officialSrv := httptest.NewServer(respondWith("X"))
	channelSrv := httptest.NewServer(respondWith("X"))
	defer officialSrv.Close()
	defer channelSrv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "model.cache")

	cfg := &config.Config{
		Cache:     config.CacheConfig{TTLHours: 1},
		Detection: config.DetectionConfig{BorderInputs: 2, DiscoveryCandidates: 10, QueriesPerInput: 3, TVThreshold: 0.4},
		Concurrency: config.ConcurrencyConfig{MaxWorkersPerChannel: 2, RateLimitRPS: 100, TimeoutSeconds: 5, MaxRetries: 1},
		Output:    config.OutputConfig{ReportDir: dir},
	}
	model := &config.ModelConfig{
		Model:    "gpt-4o",
		Official: config.Endpoint{Name: "official", URL: officialSrv.URL, Key: "sk"},
		Channels: []config.Endpoint{{Name: "chan", URL: channelSrv.URL, Key: "sk"}},
	}

	// Pre-populate cache so detect doesn't need to call official
	c := cache.New(cachePath)
	import_time := func() {}; _ = import_time
	cf := buildTestCache(t, model)
	if err := c.Save(cf); err != nil {
		t.Fatal(err)
	}

	// Step 1: online-check
	results := online.CheckAll(cfg, model.Model,
		append([]config.Endpoint{model.Official}, model.Channels...))
	for _, r := range results {
		if !r.Online {
			t.Errorf("%s should be online", r.Endpoint.Name)
		}
	}

	// Step 2: probe (cache already loaded)
	loaded, err := c.Load()
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	probeResults := detector.ProbeChannels(context.Background(), cfg, model, model.Channels, loaded.BorderInputs)
	if len(probeResults) != 1 {
		t.Fatalf("expected 1 probe result, got %d", len(probeResults))
	}
}

func buildTestCache(t *testing.T, model *config.ModelConfig) *cache.CacheFile {
	t.Helper()
	return &cache.CacheFile{
		Model:       model.Model,
		OfficialURL: model.Official.URL,
		CreatedAt:   cache.TimeNow(),
		ExpiresAt:   cache.TimeNow().Add(3600 * 1000000000),
		BorderInputs: []cache.BorderInput{
			{Prompt: "hello", OfficialDistribution: map[string]int{"X": 10}},
			{Prompt: "world", OfficialDistribution: map[string]int{"X": 8, "Y": 2}},
		},
	}
}

func TestTokenList_NotEmpty(t *testing.T) {
	toks := tokens.Load()
	if len(toks) < 10 {
		t.Errorf("expected at least 10 tokens, got %d", len(toks))
	}
}
```

Note: `buildTestCache` uses `cache.TimeNow()` — add this helper to `internal/cache/cache.go`:

```go
// TimeNow is a testable clock (can be overridden in tests).
func TimeNow() time.Time { return time.Now().UTC() }
```

- [ ] **Step 2: Run integration test**

```bash
go test -v -run TestEndToEnd -timeout 60s ./...
```

Expected: PASS (or compile error if `cache.TimeNow` missing — add it first)

- [ ] **Step 3: Run all tests**

```bash
go test ./... -timeout 60s
```

Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add integration_test.go
git commit -m "test: end-to-end integration test with httptest servers"
```

---

### Task 13: Smoke Test and Final Build

**Files:** none new

- [ ] **Step 1: Build final binary**

```bash
go build -o llmdetect ./cmd/llmdetect/
```

Expected: binary produced, no errors

- [ ] **Step 2: Smoke test --help**

```bash
./llmdetect --help
./llmdetect detect --help
./llmdetect online-check --help
./llmdetect refresh-cache --help
```

Expected: each command prints usage text without panic

- [ ] **Step 3: Run all tests one more time**

```bash
go test ./... -timeout 60s -count=1
```

Expected: all PASS

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete llmdetect Go CLI — B3IT-based channel authenticity detection"
```

---

## Notes

- The integration test's `buildTestCache` helper uses `cache.TimeNow()` which must be added to `cache.go` as a simple wrapper around `time.Now().UTC()`.
- The `tokens/fallback.txt` extraction depends on the Python environment in the sibling repo. If `uv` is not available, the alternative ASCII token list in Task 4 is sufficient for running tests.
- The `detect` command's stale-cache path is covered by unit test coverage in the cache tests and the Phase 1 shortage test; a separate E2E stale test would require time manipulation and is out of scope here.
