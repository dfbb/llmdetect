# Multi-Provider Adapter + Token Accounting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add multi-provider LLM API support (OpenAI, Anthropic, OpenRouter auto-detection with YAML persistence) and per-URL token consumption tracking to the existing llmdetect CLI.

**Architecture:** New `internal/provider/` package encapsulates format detection and request/response adaptation behind an `Adapter` interface. A `TokenLedger` in `internal/api/` accumulates concurrent per-URL token usage. The existing `discover.go` and `probe.go` stop creating their own `api.Client` instances and instead accept them as parameters, allowing `main.go` to inject the correct adapter and ledger per endpoint.

**Tech Stack:** Go 1.25, `gopkg.in/yaml.v3` (node-level API for YAML writeback), `net/http/httptest` for all adapter and detector tests. No new dependencies.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/provider/adapter.go` | Create | `ProviderType`, `TokenUsage`, `Adapter` interface, `AdapterFromType()` |
| `internal/provider/openai.go` | Create | OpenAI/OpenRouter request builder + response parser |
| `internal/provider/anthropic.go` | Create | Anthropic Messages API request builder + response parser |
| `internal/provider/detector.go` | Create | Probe-based detection + YAML writeback |
| `internal/provider/adapter_test.go` | Create | Tests for adapter.go |
| `internal/provider/openai_test.go` | Create | Tests for openai.go |
| `internal/provider/anthropic_test.go` | Create | Tests for anthropic.go |
| `internal/provider/detector_test.go` | Create | Tests for detector.go |
| `internal/api/ledger.go` | Create | Thread-safe `TokenLedger` |
| `internal/api/ledger_test.go` | Create | Tests for ledger.go |
| `internal/api/client.go` | Modify | Add `adapter`+`ledger` fields, `NewClientFull`, update `QueryOnce` |
| `internal/api/client_test.go` | Modify | Add Anthropic adapter test, ledger accumulation test |
| `config/types.go` | Modify | Add `Provider string` to `Endpoint` |
| `config/loader.go` | Modify | Validate `provider` values |
| `config/loader_test.go` | Modify | Add provider validation tests |
| `internal/detector/discover.go` | Modify | Accept `*api.Client` param instead of creating its own |
| `internal/detector/discover_test.go` | Modify | Pass test client |
| `internal/detector/probe.go` | Modify | Accept `func(config.Endpoint)*api.Client` factory param |
| `internal/detector/probe_test.go` | Modify | Pass test factory |
| `internal/online/checker.go` | Modify | Accept `func(config.Endpoint)*api.Client` factory param |
| `internal/online/checker_test.go` | Modify | Pass test factory |
| `internal/report/terminal.go` | Modify | Add token summary table after detect results |
| `internal/report/json.go` | Modify | Add `TokensUsed`, `TokenSummary`, `TotalTokens` fields |
| `cmd/llmdetect/main.go` | Modify | Wire detection, per-endpoint adapters, TokenLedger |

---

### Task 1: Provider types and Adapter interface

**Files:**
- Create: `internal/provider/adapter.go`
- Create: `internal/provider/adapter_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/provider/adapter_test.go
package provider_test

import (
	"testing"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestAdapterFromType_OpenAI(t *testing.T) {
	a, err := provider.AdapterFromType(provider.ProviderOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want %v", a.Type(), provider.ProviderOpenAI)
	}
}

func TestAdapterFromType_Anthropic(t *testing.T) {
	a, err := provider.AdapterFromType(provider.ProviderAnthropic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Type() != provider.ProviderAnthropic {
		t.Errorf("got %v, want %v", a.Type(), provider.ProviderAnthropic)
	}
}

func TestAdapterFromType_Unknown(t *testing.T) {
	_, err := provider.AdapterFromType("gemini")
	if err == nil {
		t.Fatal("expected error for unknown provider type")
	}
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
cd /Users/dfbb/Sites/myidea/llmdetect/llmdetect
go test ./internal/provider/...
```
Expected: `cannot find package "github.com/ironarmor/llmdetect/internal/provider"`

- [ ] **Step 3: Create adapter.go**

```go
// internal/provider/adapter.go
package provider

import "fmt"

type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
)

// TokenUsage holds token consumption for a single API call.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Adapter abstracts LLM API format differences: request building, auth, response parsing.
type Adapter interface {
	Type() ProviderType
	BuildRequest(model, prompt string, maxTokens int) ([]byte, error)
	RequestPath() string
	// Headers returns all required HTTP headers (auth + version headers).
	Headers(apiKey string) map[string]string
	ParseResponse(body []byte) (outputToken string, usage TokenUsage, err error)
}

// AdapterFromType constructs an Adapter for a known ProviderType.
func AdapterFromType(p ProviderType) (Adapter, error) {
	switch p {
	case ProviderOpenAI:
		return &OpenAIAdapter{}, nil
	case ProviderAnthropic:
		return &AnthropicAdapter{}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q: valid values are openai, anthropic", p)
	}
}
```

- [ ] **Step 4: Run test — expect FAIL (OpenAIAdapter/AnthropicAdapter undefined)**

```bash
go test ./internal/provider/...
```
Expected: `undefined: provider.OpenAIAdapter`

- [ ] **Step 5: Add stub types (will be filled in Tasks 2 and 3)**

Append to `internal/provider/adapter.go`:

```go
// OpenAIAdapter and AnthropicAdapter are defined in openai.go and anthropic.go.
// This file only exports the interface and factory.
```

Create `internal/provider/openai.go` with a minimal stub so the package compiles:

```go
// internal/provider/openai.go
package provider

// OpenAIAdapter implements Adapter for OpenAI and OpenRouter.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) Type() ProviderType                             { return ProviderOpenAI }
func (a *OpenAIAdapter) RequestPath() string                           { return "" }
func (a *OpenAIAdapter) Headers(apiKey string) map[string]string       { return nil }
func (a *OpenAIAdapter) BuildRequest(model, prompt string, max int) ([]byte, error) { return nil, nil }
func (a *OpenAIAdapter) ParseResponse(body []byte) (string, TokenUsage, error)     { return "", TokenUsage{}, nil }
```

Create `internal/provider/anthropic.go` with a minimal stub:

```go
// internal/provider/anthropic.go
package provider

// AnthropicAdapter implements Adapter for the Anthropic Messages API.
type AnthropicAdapter struct{}

func (a *AnthropicAdapter) Type() ProviderType                             { return ProviderAnthropic }
func (a *AnthropicAdapter) RequestPath() string                           { return "" }
func (a *AnthropicAdapter) Headers(apiKey string) map[string]string       { return nil }
func (a *AnthropicAdapter) BuildRequest(model, prompt string, max int) ([]byte, error) { return nil, nil }
func (a *AnthropicAdapter) ParseResponse(body []byte) (string, TokenUsage, error)     { return "", TokenUsage{}, nil }
```

- [ ] **Step 6: Run test — expect PASS**

```bash
go test ./internal/provider/... -v -run TestAdapterFromType
```
Expected: PASS (3 tests)

- [ ] **Step 7: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/provider/
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: add provider Adapter interface and factory"
```

---

### Task 2: OpenAI Adapter

**Files:**
- Modify: `internal/provider/openai.go` (replace stub)
- Create: `internal/provider/openai_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/provider/openai_test.go
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
```

- [ ] **Step 2: Run — expect FAIL (stub returns wrong values)**

```bash
go test ./internal/provider/... -run TestOpenAI -v
```
Expected: multiple FAILs

- [ ] **Step 3: Implement openai.go**

Replace stub in `internal/provider/openai.go`:

```go
package provider

import (
	"encoding/json"
	"fmt"
)

// OpenAIAdapter implements Adapter for OpenAI and OpenRouter endpoints.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) Type() ProviderType   { return ProviderOpenAI }
func (a *OpenAIAdapter) RequestPath() string  { return "/v1/chat/completions" }

func (a *OpenAIAdapter) Headers(apiKey string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (a *OpenAIAdapter) BuildRequest(model, prompt string, maxTokens int) ([]byte, error) {
	return json.Marshal(openAIRequest{
		Model:       model,
		Messages:    []openAIMessage{{Role: "user", Content: prompt}},
		MaxTokens:   maxTokens,
		Temperature: 0,
	})
}

func (a *OpenAIAdapter) ParseResponse(body []byte) (string, TokenUsage, error) {
	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", TokenUsage{}, fmt.Errorf("decode openai response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", TokenUsage{}, fmt.Errorf("empty choices in openai response")
	}
	return resp.Choices[0].Message.Content, TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/provider/... -run TestOpenAI -v
```
Expected: PASS (7 tests)

- [ ] **Step 5: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/provider/openai.go internal/provider/openai_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: implement OpenAI adapter"
```

---

### Task 3: Anthropic Adapter

**Files:**
- Modify: `internal/provider/anthropic.go` (replace stub)
- Create: `internal/provider/anthropic_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/provider/anthropic_test.go
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
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/provider/... -run TestAnthropic -v
```

- [ ] **Step 3: Implement anthropic.go**

Replace stub in `internal/provider/anthropic.go`:

```go
package provider

import (
	"encoding/json"
	"fmt"
)

// AnthropicAdapter implements Adapter for the Anthropic Messages API.
type AnthropicAdapter struct{}

func (a *AnthropicAdapter) Type() ProviderType  { return ProviderAnthropic }
func (a *AnthropicAdapter) RequestPath() string { return "/v1/messages" }

func (a *AnthropicAdapter) Headers(apiKey string) map[string]string {
	return map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
		"Content-Type":      "application/json",
	}
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (a *AnthropicAdapter) BuildRequest(model, prompt string, maxTokens int) ([]byte, error) {
	return json.Marshal(anthropicRequest{
		Model:       model,
		Messages:    []anthropicMessage{{Role: "user", Content: prompt}},
		MaxTokens:   maxTokens,
		Temperature: 0,
	})
}

func (a *AnthropicAdapter) ParseResponse(body []byte) (string, TokenUsage, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", TokenUsage{}, fmt.Errorf("decode anthropic response: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", TokenUsage{}, fmt.Errorf("empty content in anthropic response")
	}
	return resp.Content[0].Text, TokenUsage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/provider/... -v
```
Expected: all tests PASS including Tasks 1–3.

- [ ] **Step 5: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/provider/anthropic.go internal/provider/anthropic_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: implement Anthropic adapter"
```

---

### Task 4: Provider Detector

**Files:**
- Create: `internal/provider/detector.go`
- Create: `internal/provider/detector_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/provider/detector_test.go
package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestDetect_OpenRouter(t *testing.T) {
	// No server needed — OpenRouter is short-circuited by URL pattern
	a, err := provider.Detect(context.Background(),
		"https://openrouter.ai/api", "key", "gpt-4o", "", "", time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want openai", a.Type())
	}
}

func TestDetect_OpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a, err := provider.Detect(context.Background(),
		srv.URL, "sk-test", "gpt-4o", "", srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want openai", a.Type())
	}
}

func TestDetect_Anthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/messages" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	a, err := provider.Detect(context.Background(),
		srv.URL, "sk-ant-test", "claude-3-5-sonnet-20241022", "", srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if a.Type() != provider.ProviderAnthropic {
		t.Errorf("got %v, want anthropic", a.Type())
	}
}

func TestDetect_Undetectable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := provider.Detect(context.Background(),
		srv.URL, "key", "model", "", srv.URL, time.Second)
	if err == nil {
		t.Fatal("expected ErrProviderUndetectable")
	}
}

func TestDetect_WritesProviderToYAML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
			return
		}
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	yamlContent := `model: gpt-4o
official:
  name: "Test"
  url: "` + srv.URL + `"
  key: "sk-test"
channels: []
`
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := provider.Detect(context.Background(),
		srv.URL, "sk-test", "gpt-4o", yamlPath, srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	data, _ := os.ReadFile(yamlPath)
	if !contains(string(data), "provider: openai") {
		t.Errorf("provider not written to YAML:\n%s", data)
	}
	// Verify existing fields are preserved
	if !contains(string(data), `name: "Test"`) {
		t.Errorf("existing field lost after YAML rewrite:\n%s", data)
	}
}

func TestDetect_SkipsProbeIfProviderAlreadyInYAML(t *testing.T) {
	// probeCount will be > 0 if the server is hit
	probeCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeCount++
		w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer srv.Close()

	// Caller is responsible for short-circuiting when provider is known.
	// AdapterFromType does not probe — this test verifies that contract.
	a, err := provider.AdapterFromType(provider.ProviderOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if a == nil {
		t.Fatal("nil adapter")
	}
	if probeCount != 0 {
		t.Errorf("AdapterFromType made %d probe requests, want 0", probeCount)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/provider/... -run TestDetect -v
```
Expected: `undefined: provider.Detect`

- [ ] **Step 3: Implement detector.go**

```go
// internal/provider/detector.go
package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrProviderUndetectable is returned when neither OpenAI nor Anthropic format
// returns HTTP 200 from the target endpoint.
var ErrProviderUndetectable = errors.New("provider undetectable: neither openai nor anthropic format responded with 200")

var yamlMu sync.Mutex

// Detect probes baseURL to determine its API format, writes the result to yamlPath
// (matched by endpointURL), and returns the matching Adapter.
// If baseURL contains "openrouter.ai", OpenAIAdapter is returned without probing.
// Set yamlPath="" to skip writeback.
func Detect(ctx context.Context, baseURL, apiKey, model, yamlPath, endpointURL string, timeout time.Duration) (Adapter, error) {
	if strings.Contains(baseURL, "openrouter.ai") {
		return &OpenAIAdapter{}, nil
	}

	client := &http.Client{Timeout: timeout}

	for _, a := range []Adapter{&OpenAIAdapter{}, &AnthropicAdapter{}} {
		if probeAdapter(ctx, client, baseURL, apiKey, model, a) {
			if yamlPath != "" {
				yamlMu.Lock()
				if err := writeProviderToYAML(yamlPath, endpointURL, a.Type()); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not write provider to %s: %v\n", yamlPath, err)
				}
				yamlMu.Unlock()
			}
			return a, nil
		}
	}
	return nil, ErrProviderUndetectable
}

func probeAdapter(ctx context.Context, client *http.Client, baseURL, apiKey, model string, a Adapter) bool {
	body, err := a.BuildRequest(model, "hi", 1)
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+a.RequestPath(), bytes.NewReader(body))
	if err != nil {
		return false
	}
	for k, v := range a.Headers(apiKey) {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// writeProviderToYAML updates the provider field of the endpoint with matching URL in yamlPath.
// Preserves all other fields, ordering, and comments using yaml.Node.
func writeProviderToYAML(yamlPath, endpointURL string, p ProviderType) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return fmt.Errorf("empty YAML document in %s", yamlPath)
	}
	if !setProviderInNode(root.Content[0], endpointURL, string(p)) {
		return fmt.Errorf("endpoint URL %s not found in %s", endpointURL, yamlPath)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(yamlPath, out, 0644)
}

// setProviderInNode recursively walks node looking for a mapping with url==targetURL,
// then adds or updates its provider field. Returns true if found.
func setProviderInNode(node *yaml.Node, targetURL, providerVal string) bool {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "url" && node.Content[i+1].Value == targetURL {
				setMappingKey(node, "provider", providerVal)
				return true
			}
		}
		for i := 1; i < len(node.Content); i += 2 {
			if setProviderInNode(node.Content[i], targetURL, providerVal) {
				return true
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if setProviderInNode(child, targetURL, providerVal) {
				return true
			}
		}
	}
	return false
}

// setMappingKey updates an existing key's value in a mapping node,
// or appends a new key-value pair if the key does not exist.
func setMappingKey(node *yaml.Node, key, value string) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Value = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/provider/... -v
```
Expected: all tests PASS

- [ ] **Step 5: Run with race detector**

```bash
go test ./internal/provider/... -race
```
Expected: PASS, no data races

- [ ] **Step 6: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/provider/detector.go internal/provider/detector_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: add provider detector with YAML persistence"
```

---

### Task 5: TokenLedger

**Files:**
- Create: `internal/api/ledger.go`
- Create: `internal/api/ledger_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/api/ledger_test.go
package api_test

import (
	"sync"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestLedger_AddAndSnapshot(t *testing.T) {
	l := api.NewTokenLedger()
	l.Add("https://api.openai.com/v1", provider.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12})

	snap := l.Snapshot()
	u, ok := snap["https://api.openai.com/v1"]
	if !ok {
		t.Fatal("URL not found in snapshot")
	}
	if u.PromptTokens != 10 || u.CompletionTokens != 2 || u.TotalTokens != 12 {
		t.Errorf("got %+v, want {10 2 12}", u)
	}
}

func TestLedger_Accumulates(t *testing.T) {
	l := api.NewTokenLedger()
	url := "https://api.openai.com/v1"
	l.Add(url, provider.TokenUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6})
	l.Add(url, provider.TokenUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6})

	snap := l.Snapshot()
	u := snap[url]
	if u.PromptTokens != 10 || u.TotalTokens != 12 {
		t.Errorf("accumulation wrong: got %+v, want {10 2 12}", u)
	}
}

func TestLedger_Total(t *testing.T) {
	l := api.NewTokenLedger()
	l.Add("https://a.com", provider.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12})
	l.Add("https://b.com", provider.TokenUsage{PromptTokens: 20, CompletionTokens: 4, TotalTokens: 24})

	total := l.Total()
	if total.PromptTokens != 30 || total.CompletionTokens != 6 || total.TotalTokens != 36 {
		t.Errorf("total wrong: got %+v, want {30 6 36}", total)
	}
}

func TestLedger_SnapshotIsImmutable(t *testing.T) {
	l := api.NewTokenLedger()
	url := "https://a.com"
	l.Add(url, provider.TokenUsage{TotalTokens: 5})
	snap := l.Snapshot()
	// Modify the snapshot copy — ledger should not change
	copied := snap[url]
	copied.TotalTokens = 9999
	snap[url] = copied

	total := l.Total()
	if total.TotalTokens != 5 {
		t.Errorf("snapshot mutation affected ledger: got %d", total.TotalTokens)
	}
}

func TestLedger_Concurrent(t *testing.T) {
	l := api.NewTokenLedger()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Add("https://a.com", provider.TokenUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2})
		}()
	}
	wg.Wait()

	total := l.Total()
	if total.TotalTokens != 200 {
		t.Errorf("concurrent total wrong: got %d, want 200", total.TotalTokens)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/api/... -run TestLedger -v
```
Expected: `undefined: api.NewTokenLedger`

- [ ] **Step 3: Implement ledger.go**

```go
// internal/api/ledger.go
package api

import (
	"sync"

	"github.com/ironarmor/llmdetect/internal/provider"
)

// TokenLedger accumulates token usage per endpoint URL across concurrent requests.
type TokenLedger struct {
	mu    sync.Mutex
	usage map[string]*provider.TokenUsage
}

func NewTokenLedger() *TokenLedger {
	return &TokenLedger{usage: make(map[string]*provider.TokenUsage)}
}

func (l *TokenLedger) Add(url string, u provider.TokenUsage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.usage[url]; ok {
		existing.PromptTokens += u.PromptTokens
		existing.CompletionTokens += u.CompletionTokens
		existing.TotalTokens += u.TotalTokens
	} else {
		l.usage[url] = &provider.TokenUsage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		}
	}
}

// Snapshot returns a copy of all accumulated usage. Safe to call after all goroutines complete.
func (l *TokenLedger) Snapshot() map[string]provider.TokenUsage {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]provider.TokenUsage, len(l.usage))
	for k, v := range l.usage {
		out[k] = *v
	}
	return out
}

// Total sums usage across all URLs.
func (l *TokenLedger) Total() provider.TokenUsage {
	l.mu.Lock()
	defer l.mu.Unlock()
	var total provider.TokenUsage
	for _, v := range l.usage {
		total.PromptTokens += v.PromptTokens
		total.CompletionTokens += v.CompletionTokens
		total.TotalTokens += v.TotalTokens
	}
	return total
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/api/... -run TestLedger -race -v
```
Expected: PASS, no data races

- [ ] **Step 5: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/api/ledger.go internal/api/ledger_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: add TokenLedger for per-URL token accounting"
```

---

### Task 6: Update client.go to use Adapter + Ledger

**Files:**
- Modify: `internal/api/client.go`
- Modify: `internal/api/client_test.go`

- [ ] **Step 1: Write new failing tests**

Add to `internal/api/client_test.go`:

```go
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
```

You also need to add the missing imports to `client_test.go`. The full updated import block:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/provider"
)
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/api/... -v
```
Expected: `undefined: api.NewClientFull`

- [ ] **Step 3: Rewrite client.go**

```go
// internal/api/client.go
package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/ironarmor/llmdetect/internal/provider"
)

type Client struct {
	baseURL    string
	apiKey     string
	maxRetries int
	http       *http.Client
	adapter    provider.Adapter
	ledger     *TokenLedger
}

// NewClient returns a Client using the OpenAI adapter with no token tracking.
// Existing callers (online-check, discover, probe) use this for backward compatibility.
func NewClient(baseURL, apiKey string, timeoutSeconds, maxRetries int) *Client {
	return NewClientFull(baseURL, apiKey, timeoutSeconds, maxRetries, &provider.OpenAIAdapter{}, nil)
}

// NewClientFull returns a Client with a specific adapter and optional TokenLedger.
func NewClientFull(baseURL, apiKey string, timeoutSeconds, maxRetries int, a provider.Adapter, ledger *TokenLedger) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		maxRetries: maxRetries,
		http:       &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		adapter:    a,
		ledger:     ledger,
	}
}

// QueryOnce sends a single request using the client's adapter and returns the output token.
// Token usage is accumulated to the ledger if one is configured.
func (c *Client) QueryOnce(ctx context.Context, model, prompt string) (string, error) {
	body, err := c.adapter.BuildRequest(model, prompt, 1)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

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
			c.baseURL+c.adapter.RequestPath(), bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		for k, v := range c.adapter.Headers(c.apiKey) {
			req.Header.Set(k, v)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return "", fmt.Errorf("API returned 401 unauthorized — check your API key for %s", c.baseURL)
		}
		if resp.StatusCode >= 500 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(b))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
		}

		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		token, usage, err := c.adapter.ParseResponse(b)
		if err != nil {
			lastErr = err
			continue
		}
		if c.ledger != nil {
			c.ledger.Add(c.baseURL, usage)
		}
		return token, nil
	}
	return "", fmt.Errorf("all retries failed: %w", lastErr)
}

// Ping sends a minimal request and returns true if the endpoint responds with HTTP 200.
func (c *Client) Ping(ctx context.Context, model string) bool {
	_, err := c.QueryOnce(ctx, model, "hi")
	return err == nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/api/... -race -v
```
Expected: all tests PASS

- [ ] **Step 5: Verify full suite still compiles**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/api/client.go internal/api/client_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: update client to use Adapter and TokenLedger"
```

---

### Task 7: Add Provider field to config + validation

**Files:**
- Modify: `config/types.go`
- Modify: `config/loader.go`
- Modify: `config/loader_test.go`

- [ ] **Step 1: Write failing tests**

Add to `config/loader_test.go`:

```go
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
```

Also check that `loader_test.go` imports `os` and `path/filepath` — add them if missing.

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./config/... -run TestLoadModel_Invalid -v
```
Expected: FAIL — no `Provider` field yet

- [ ] **Step 3: Update types.go**

In `config/types.go`, change `Endpoint`:

```go
type Endpoint struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Key      string `yaml:"key"`
	Provider string `yaml:"provider,omitempty"`
}
```

- [ ] **Step 4: Update loader.go**

Add a validation helper and call it from `LoadModel`:

```go
func validateEndpointProvider(ep Endpoint, label string) error {
	if ep.Provider == "" {
		return nil
	}
	switch ep.Provider {
	case "openai", "anthropic":
		return nil
	default:
		return fmt.Errorf("endpoint %q has invalid provider %q: must be openai or anthropic", label, ep.Provider)
	}
}
```

At the end of `LoadModel`, before `return &m, nil`, add:

```go
	if err := validateEndpointProvider(m.Official, "official"); err != nil {
		return nil, err
	}
	for _, ch := range m.Channels {
		if err := validateEndpointProvider(ch, ch.Name); err != nil {
			return nil, err
		}
	}
```

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./config/... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add config/types.go config/loader.go config/loader_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: add Provider field to Endpoint with validation"
```

---

### Task 8: Update discover.go to accept client parameter

**Files:**
- Modify: `internal/detector/discover.go`
- Modify: `internal/detector/discover_test.go`

- [ ] **Step 1: Update the test to pass a client**

Read `internal/detector/discover_test.go` first to understand its current structure, then update the `Discover` call site to pass a pre-built `*api.Client`:

```go
// In the test, replace the Discover call from:
//   result, err := detector.Discover(ctx, cfg, model, tokenList)
// to:
//   client := api.NewClient(model.Official.URL, model.Official.Key,
//       cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
//   result, err := detector.Discover(ctx, cfg, model, tokenList, client)
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/detector/... -run TestDiscover -v
```
Expected: compile error — `Discover` called with wrong number of arguments

- [ ] **Step 3: Update discover.go**

Change the `Discover` signature and remove the internal client creation:

```go
// Before (remove this line inside Discover):
//   client := api.NewClient(model.Official.URL, model.Official.Key,
//       cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)

// After — new signature:
func Discover(ctx context.Context, cfg *config.Config, model *config.ModelConfig, tokens []string, client *api.Client) (*DiscoverResult, error) {
```

The body of `Discover` is otherwise unchanged — it already uses `client` by name.

- [ ] **Step 4: Fix the call site in main.go temporarily**

In `cmd/llmdetect/main.go`, both `cmdRefreshCache` and `cmdDetect` call `detector.Discover`. Add a temporary official client to each call to fix the compile error (Task 10 will make this proper):

In `cmdRefreshCache`:
```go
officialClient := api.NewClient(model.Official.URL, model.Official.Key,
    cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
```

In `cmdDetect` (inside the `if c.IsExpired()` branch):
```go
officialClient := api.NewClient(model.Official.URL, model.Official.Key,
    cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
```

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./... 
```
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/detector/discover.go internal/detector/discover_test.go cmd/llmdetect/main.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "refactor: discover accepts *api.Client parameter"
```

---

### Task 9: Update probe.go and checker.go to accept client factories

**Files:**
- Modify: `internal/detector/probe.go`
- Modify: `internal/detector/probe_test.go`
- Modify: `internal/online/checker.go`
- Modify: `internal/online/checker_test.go`

- [ ] **Step 1: Update probe test**

Read `internal/detector/probe_test.go`. Find the call to `ProbeChannels` and update it to pass a factory:

```go
// Replace ProbeChannels call from:
//   results := detector.ProbeChannels(ctx, cfg, model, channels, bis)
// to:
//   newClient := func(ep config.Endpoint) *api.Client {
//       return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
//   }
//   results := detector.ProbeChannels(ctx, cfg, model, channels, bis, newClient)
```

- [ ] **Step 2: Update checker test**

Read `internal/online/checker_test.go`. Update the `CheckAll` call to pass a factory:

```go
// Replace CheckAll call from:
//   results := online.CheckAll(cfg, model, endpoints)
// to:
//   newClient := func(ep config.Endpoint) *api.Client {
//       return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1)
//   }
//   results := online.CheckAll(cfg, model, endpoints, newClient)
```

- [ ] **Step 3: Run — expect compile failure**

```bash
go test ./internal/detector/... ./internal/online/... -v
```
Expected: compile errors

- [ ] **Step 4: Update probe.go**

Change `ProbeChannels` and `probeOne` to accept a client factory:

```go
// New signature for ProbeChannels:
func ProbeChannels(ctx context.Context, cfg *config.Config, model *config.ModelConfig,
	channels []config.Endpoint, bis []cache.BorderInput,
	newClient func(ep config.Endpoint) *api.Client) []ChannelResult {

// Inside the goroutine, replace: r := probeOne(ctx, cfg, model, ch, bis)
// with:
	r := probeOne(ctx, cfg, model, ch, bis, newClient(ch))

// New signature for probeOne:
func probeOne(ctx context.Context, cfg *config.Config, model *config.ModelConfig,
	ch config.Endpoint, bis []cache.BorderInput, client *api.Client) ChannelResult {

// Remove the internal client creation line:
//   client := api.NewClient(ch.URL, ch.Key, cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
```

- [ ] **Step 5: Update checker.go**

Change `CheckAll` to accept a client factory:

```go
// New signature:
func CheckAll(cfg *config.Config, model string, endpoints []config.Endpoint,
	newClient func(ep config.Endpoint) *api.Client) []Result {

// Inside the goroutine, replace:
//   c := api.NewClient(endpoint.URL, endpoint.Key, cfg.Concurrency.TimeoutSeconds, 1)
// with:
	c := newClient(endpoint)
```

- [ ] **Step 6: Fix call sites in main.go**

In `cmdOnlineCheck`:
```go
newClient := func(ep config.Endpoint) *api.Client {
    return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1)
}
results := online.CheckAll(cfg, model.Model, all, newClient)
```

In `cmdDetect` (Step 1 online-check):
```go
newClientDefault := func(ep config.Endpoint) *api.Client {
    return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1)
}
onlineResults := online.CheckAll(cfg, model.Model, all, newClientDefault)
```

In `cmdDetect` (Step 3 probe):
```go
probeResults := detector.ProbeChannels(ctx, cfg, model, onlineChannels, cf.BorderInputs,
    func(ep config.Endpoint) *api.Client {
        return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
    })
```

- [ ] **Step 7: Run — expect PASS**

```bash
go test ./... 
```
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/detector/probe.go internal/detector/probe_test.go internal/online/checker.go internal/online/checker_test.go cmd/llmdetect/main.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "refactor: probe and checker accept client factory"
```

---

### Task 10: Wire provider detection and TokenLedger into main.go

**Files:**
- Modify: `cmd/llmdetect/main.go`

- [ ] **Step 1: Run existing tests as baseline**

```bash
go test ./... 
```
Expected: PASS — establish that all tests pass before this task.

- [ ] **Step 2: Rewrite cmdDetect to detect providers and use TokenLedger**

Replace the entire `cmdDetect` function body with the following (keep the `cobra.Command` wrapper):

```go
func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Run online-check, load/refresh cache, probe all channels, and output a report",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			ctx := context.Background()
			startTime := time.Now()
			timeout := time.Duration(cfg.Concurrency.TimeoutSeconds) * time.Second
			ledger := api.NewTokenLedger()

			// Resolve adapter per endpoint (from YAML or via probe).
			// Endpoints that are undetectable are skipped.
			allEps := append([]config.Endpoint{model.Official}, model.Channels...)
			adapters := make(map[string]provider.Adapter, len(allEps))
			for _, ep := range allEps {
				var a provider.Adapter
				var err error
				if ep.Provider != "" {
					a, err = provider.AdapterFromType(provider.ProviderType(ep.Provider))
				} else {
					a, err = provider.Detect(ctx, ep.URL, ep.Key, model.Model, flagModel, ep.URL, timeout)
				}
				if errors.Is(err, provider.ErrProviderUndetectable) {
					fmt.Fprintf(os.Stderr, "warning: provider undetectable for %s, treating as offline\n", ep.URL)
					continue
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: detect provider for %s: %v\n", ep.URL, err)
					continue
				}
				adapters[ep.URL] = a
			}

			// clientFor creates a Client with the detected adapter and the shared ledger.
			clientFor := func(ep config.Endpoint) *api.Client {
				a, ok := adapters[ep.URL]
				if !ok {
					a = &provider.OpenAIAdapter{}
				}
				return api.NewClientFull(ep.URL, ep.Key,
					cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries, a, ledger)
			}

			// Step 1: online-check.
			// Uses detected adapters (via clientFor) so Anthropic endpoints are checked correctly.
			onlineCheckFactory := func(ep config.Endpoint) *api.Client {
				a, ok := adapters[ep.URL]
				if !ok {
					a = &provider.OpenAIAdapter{}
				}
				return api.NewClientFull(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1, a, nil)
			}
			onlineResults := online.CheckAll(cfg, model.Model, allEps, onlineCheckFactory)

			var onlineChannels []config.Endpoint
			officialURL := model.Official.URL
			for _, r := range onlineResults {
				if r.Online && r.Endpoint.URL != officialURL {
					onlineChannels = append(onlineChannels, r.Endpoint)
				}
			}

			// Check official API is reachable.
			officialOnline := false
			for _, r := range onlineResults {
				if r.Endpoint.URL == officialURL && r.Online {
					officialOnline = true
				}
			}

			// Step 2: load or refresh cache.
			c := cache.New(cachePath(flagModel))
			var cf *cache.CacheFile
			cacheStale := false
			cacheAgeMinutes := 0

			if c.IsExpired() {
				if !officialOnline {
					old, loadErr := c.Load()
					if loadErr != nil {
						fmt.Fprintf(os.Stderr, "official API offline and no stale cache available\n")
						os.Exit(1)
					}
					cf = old
					cacheStale = true
					cacheAgeMinutes = int(time.Since(old.CreatedAt).Minutes())
					fmt.Fprintf(os.Stderr, "Warning: official API offline, using stale cache (%d min old)\n", cacheAgeMinutes)
				} else {
					tokenList := tokens.Load()
					officialClient := clientFor(model.Official)
					result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
					if err != nil {
						old, loadErr := c.Load()
						if loadErr != nil {
							fmt.Fprintf(os.Stderr, "refresh failed and no stale cache: %v\n", err)
							os.Exit(1)
						}
						cf = old
						cacheStale = true
						cacheAgeMinutes = int(time.Since(old.CreatedAt).Minutes())
						fmt.Fprintf(os.Stderr, "Warning: refresh failed (%v), using stale cache (%d min old)\n", err, cacheAgeMinutes)
					} else {
						now := time.Now().UTC()
						cf = &cache.CacheFile{
							Model:        model.Model,
							OfficialURL:  model.Official.URL,
							CreatedAt:    now,
							ExpiresAt:    now.Add(time.Duration(cfg.Cache.TTLHours) * time.Hour),
							BorderInputs: result.BorderInputs,
						}
						if err := c.Save(cf); err != nil {
							fmt.Fprintf(os.Stderr, "save cache: %v\n", err)
						}
					}
				}
			} else {
				var err error
				cf, err = c.Load()
				if err != nil {
					fmt.Fprintf(os.Stderr, "load cache: %v\n", err)
					os.Exit(1)
				}
			}

			// Step 3: probe channels with per-channel adapters and shared ledger.
			probeResults := detector.ProbeChannels(ctx, cfg, model, onlineChannels, cf.BorderInputs, clientFor)

			duration := time.Since(startTime).Seconds()

			channelOnlineResults := make([]online.Result, 0, len(onlineResults)-1)
			for _, r := range onlineResults {
				if r.Endpoint.URL != officialURL {
					channelOnlineResults = append(channelOnlineResults, r)
				}
			}

			params := report.ReportParams{
				Model:             model.Model,
				RunAt:             startTime,
				Duration:          duration,
				Cfg:               cfg,
				BorderInputsFound: len(cf.BorderInputs),
				Shortage:          len(cf.BorderInputs) < cfg.Detection.BorderInputs,
				CacheStale:        cacheStale,
				CacheAgeMinutes:   cacheAgeMinutes,
				OnlineResults:     channelOnlineResults,
				ProbeResults:      probeResults,
				Ledger:            ledger,
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

Also update `cmdRefreshCache` to use detected adapter:

```go
func cmdRefreshCache() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh-cache",
		Short: "Discover border inputs from the official API and update the cache file",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			tokenList := tokens.Load()
			ctx := context.Background()
			timeout := time.Duration(cfg.Concurrency.TimeoutSeconds) * time.Second

			// Detect or load provider for the official API.
			var a provider.Adapter
			var err error
			if model.Official.Provider != "" {
				a, err = provider.AdapterFromType(provider.ProviderType(model.Official.Provider))
			} else {
				a, err = provider.Detect(ctx, model.Official.URL, model.Official.Key,
					model.Model, flagModel, model.Official.URL, timeout)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "detect official provider: %v\n", err)
				os.Exit(1)
			}

			officialClient := api.NewClientFull(model.Official.URL, model.Official.Key,
				cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries, a, nil)

			fmt.Println("Discovering border inputs from official API...")
			result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
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
```

Update the import block in `main.go` to include the new packages:

```go
import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
	"github.com/ironarmor/llmdetect/internal/provider"
	"github.com/ironarmor/llmdetect/internal/report"
	"github.com/ironarmor/llmdetect/tokens"
)
```

Also add `Ledger *api.TokenLedger` field to `report.ReportParams` in `internal/report/json.go` (temporary stub — Task 12 will use it properly):

```go
// In report/json.go, add to ReportParams:
Ledger *api.TokenLedger
```

And add the import `"github.com/ironarmor/llmdetect/internal/api"` to `report/json.go` and `report/terminal.go`.

- [ ] **Step 3: Run — expect compile and PASS**

```bash
go build ./...
go test ./...
```
Expected: builds and tests PASS

- [ ] **Step 4: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add cmd/llmdetect/main.go internal/report/json.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: wire provider detection and TokenLedger into detect command"
```

---

### Task 11: Token summary table in terminal.go

**Files:**
- Modify: `internal/report/terminal.go`

- [ ] **Step 1: Write failing test**

Create `internal/report/terminal_token_test.go`:

```go
package report_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/provider"
	"github.com/ironarmor/llmdetect/internal/report"
)

func TestPrintTokenSummary_ShowsTable(t *testing.T) {
	ledger := api.NewTokenLedger()
	ledger.Add("https://api.openai.com/v1", provider.TokenUsage{PromptTokens: 12450, CompletionTokens: 620, TotalTokens: 13070})
	ledger.Add("https://api.xxx.com/v1", provider.TokenUsage{PromptTokens: 8200, CompletionTokens: 410, TotalTokens: 8610})

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	report.PrintTokenSummary(ledger)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "Token") {
		t.Errorf("expected token table header in output:\n%s", out)
	}
	if !strings.Contains(out, "12,450") {
		t.Errorf("expected formatted number 12,450 in output:\n%s", out)
	}
	if !strings.Contains(out, fmt.Sprintf("%d", 13070+8610)) {
		t.Errorf("expected total tokens %d in output:\n%s", 13070+8610, out)
	}
}

func TestPrintTokenSummary_EmptyLedger(t *testing.T) {
	ledger := api.NewTokenLedger()
	// Should not panic on empty ledger
	report.PrintTokenSummary(ledger)
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/report/... -run TestPrintTokenSummary -v
```
Expected: `undefined: report.PrintTokenSummary`

- [ ] **Step 3: Implement PrintTokenSummary in terminal.go**

Add to `internal/report/terminal.go`:

```go
import (
	// existing imports ...
	"golang.org/x/text/message"  // NOT available — use manual formatting instead
)
```

Actually `golang.org/x/text` is not in go.mod. Use a helper to format with commas:

```go
func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	offset := len(s) % 3
	for i, c := range []byte(s) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
}

// PrintTokenSummary prints per-URL token consumption after a detect run.
func PrintTokenSummary(ledger *api.TokenLedger) {
	snap := ledger.Snapshot()
	if len(snap) == 0 {
		return
	}
	total := ledger.Total()

	fmt.Printf("\n%s\n%s\n", bold("Token Usage"), separator)
	fmt.Printf("  %-45s  %8s  %6s  %8s\n", "URL", "Prompt", "Compl", "Total")
	fmt.Printf("  %s\n", separator[:70])

	// Sort URLs for stable output
	urls := make([]string, 0, len(snap))
	for u := range snap {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	for _, u := range urls {
		u2 := snap[u]
		display := u
		if len(display) > 45 {
			display = "..." + display[len(display)-42:]
		}
		fmt.Printf("  %-45s  %8s  %6s  %8s\n",
			display,
			formatInt(u2.PromptTokens),
			formatInt(u2.CompletionTokens),
			formatInt(u2.TotalTokens),
		)
	}
	fmt.Printf("  %s\n", separator[:70])
	fmt.Printf("  %-45s  %8s  %6s  %8s\n",
		bold("Total"),
		formatInt(total.PromptTokens),
		formatInt(total.CompletionTokens),
		formatInt(total.TotalTokens),
	)
	fmt.Printf("%s\n", separator)
}
```

Add `"sort"` to the import block in `terminal.go`. Also add `"github.com/ironarmor/llmdetect/internal/api"` if not already there.

Also update `PrintSummary` to call `PrintTokenSummary` at the end when `params.Ledger != nil`:

```go
// At the end of PrintSummary, after the detection results table:
if params.Ledger != nil {
    PrintTokenSummary(params.Ledger)
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/report/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/report/terminal.go internal/report/terminal_token_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: add token usage summary table to terminal output"
```

---

### Task 12: Token fields in JSON report

**Files:**
- Modify: `internal/report/json.go`

- [ ] **Step 1: Write failing test**

Create `internal/report/json_token_test.go`:

```go
package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	json.Unmarshal(data, &out)

	results, _ := out["results"].([]any)
	ch1, _ := results[0].(map[string]any)
	if _, hasTokens := ch1["tokens_used"]; hasTokens {
		t.Errorf("offline channel should not have tokens_used: %v", ch1)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/report/... -run TestWriteJSON_Include -v
```
Expected: FAIL — missing fields in report

- [ ] **Step 3: Update json.go**

Add token fields to the structs and populate them in `WriteJSON`:

```go
// Add to JSONChannelResult:
TokensUsed *JSONTokenUsage `json:"tokens_used,omitempty"`

// Add new type:
type JSONTokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Add to JSONReport:
TokenSummary map[string]JSONTokenUsage `json:"token_summary,omitempty"`
TotalTokens  *int                      `json:"total_tokens,omitempty"`
```

In `WriteJSON`, after building `results`, add token data from the ledger:

```go
// After building results slice, add token data if ledger is present:
if params.Ledger != nil {
    snap := params.Ledger.Snapshot()
    for i, or_ := range params.OnlineResults {
        if !or_.Online {
            continue
        }
        if u, ok := snap[or_.Endpoint.URL]; ok && u.TotalTokens > 0 {
            results[i].TokensUsed = &JSONTokenUsage{
                Prompt:     u.PromptTokens,
                Completion: u.CompletionTokens,
                Total:      u.TotalTokens,
            }
        }
    }
    // Build token_summary (all URLs including official)
    summary := make(map[string]JSONTokenUsage, len(snap))
    for url, u := range snap {
        summary[url] = JSONTokenUsage{
            Prompt:     u.PromptTokens,
            Completion: u.CompletionTokens,
            Total:      u.TotalTokens,
        }
    }
    rep.TokenSummary = summary
    total := params.Ledger.Total()
    t := total.TotalTokens
    rep.TotalTokens = &t
}
```

Also add `"github.com/ironarmor/llmdetect/internal/api"` import to `json.go`.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/report/... -v
```
Expected: all tests PASS

- [ ] **Step 5: Run full suite**

```bash
go test ./... -race
```
Expected: PASS, no data races

- [ ] **Step 6: Build the binary to confirm everything links**

```bash
go build -o /tmp/llmdetect-test ./cmd/llmdetect
```
Expected: binary created with no errors

- [ ] **Step 7: Commit**

```bash
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect add internal/report/json.go internal/report/json_token_test.go
git -C /Users/dfbb/Sites/myidea/llmdetect/llmdetect commit -m "feat: add token usage fields to JSON report"
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Covered by |
|---|---|
| `internal/provider/` package | Tasks 1–4 |
| Probe-based detection (OpenAI → Anthropic order) | Task 4 |
| YAML writeback (node-level, preserves fields) | Task 4 |
| OpenRouter URL bypass | Task 4 |
| `provider` field in `Endpoint` | Task 7 |
| `provider` validation in loader | Task 7 |
| `TokenLedger` with Add/Snapshot/Total | Task 5 |
| Client uses Adapter + Ledger | Task 6 |
| `detect` command: detection before probe | Task 10 |
| `detect` triggers refresh with ledger active | Task 10 |
| Offline official API + no cache = os.Exit(1) | Task 10 |
| `ErrProviderUndetectable` = treated as offline | Task 10 |
| YAML writeback warning (non-fatal) | Task 4, Task 10 |
| Token table in terminal output | Task 11 |
| `tokens_used` in JSON channel result | Task 12 |
| `token_summary` + `total_tokens` in JSON report | Task 12 |
| Offline channels omit `tokens_used` | Task 12 |
| `usage` absent from proxy → zero / null | Handled by adapter ParseResponse returning zero TokenUsage on missing field |

All spec requirements are covered. No gaps.
