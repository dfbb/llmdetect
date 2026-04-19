package provider

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Prevent network calls during tests by injecting a fixed version.
	fetchCLIVersion = func() string { return "2.1.112" }
	// Reset version cache so tests start from a clean state.
	versionCache.version = ""
	os.Exit(m.Run())
}

func TestClaudeCodeAdapter_Type(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	if a.Type() != ProviderClaudeCode {
		t.Fatalf("expected claude-code, got %s", a.Type())
	}
}

func TestClaudeCodeAdapter_RequestPath(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	if a.RequestPath() != "/v1/messages" {
		t.Fatalf("unexpected path: %s", a.RequestPath())
	}
}

func TestClaudeCodeAdapter_Headers(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	h := a.Headers("sk-test")

	if !strings.HasPrefix(h["Authorization"], "Bearer ") {
		t.Errorf("Authorization must be Bearer, got: %s", h["Authorization"])
	}
	if !strings.HasPrefix(h["User-Agent"], "claude-cli/") {
		t.Errorf("User-Agent must start with claude-cli/, got: %s", h["User-Agent"])
	}
	if h["x-app"] != "cli" {
		t.Errorf("x-app must be cli, got: %s", h["x-app"])
	}
	if h["anthropic-version"] != "2023-06-01" {
		t.Errorf("anthropic-version wrong: %s", h["anthropic-version"])
	}
	if !strings.Contains(h["anthropic-beta"], "claude-code-20250219") {
		t.Errorf("anthropic-beta missing claude-code-20250219: %s", h["anthropic-beta"])
	}
	if h["X-Stainless-Lang"] != "js" {
		t.Errorf("X-Stainless-Lang wrong: %s", h["X-Stainless-Lang"])
	}
}

func TestClaudeCodeAdapter_BuildRequest(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body, err := a.BuildRequest("claude-opus-4-5", "hello world", 1)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var req ccRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Model != "claude-opus-4-5" {
		t.Errorf("model wrong: %s", req.Model)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Errorf("messages wrong: %+v", req.Messages)
	}
	if len(req.System) < 2 {
		t.Fatalf("system must have at least 2 blocks, got %d", len(req.System))
	}
	// First block: billing header
	if !strings.Contains(req.System[0].Text, "x-anthropic-billing-header") {
		t.Errorf("first system block must be billing header, got: %s", req.System[0].Text)
	}
	if !strings.Contains(req.System[0].Text, "cc_entrypoint=cli") {
		t.Errorf("billing header missing cc_entrypoint=cli: %s", req.System[0].Text)
	}
	// Second block: Claude Code identity
	if !strings.Contains(req.System[1].Text, "Claude Code") {
		t.Errorf("second system block must identify as Claude Code: %s", req.System[1].Text)
	}
	if req.Thinking.Type != "adaptive" {
		t.Errorf("thinking.type wrong: %s", req.Thinking.Type)
	}
	if req.OutputConfig.Effort != "medium" {
		t.Errorf("output_config.effort wrong: %s", req.OutputConfig.Effort)
	}
	if req.Metadata.UserID == "" {
		t.Error("metadata.user_id must not be empty")
	}
}

func TestClaudeCodeAdapter_ParseResponse(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body := []byte(`{
		"content": [{"type": "text", "text": "hello"}],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)
	tok, usage, err := a.ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}
	if tok != "hello" {
		t.Errorf("token wrong: %s", tok)
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 5 || usage.TotalTokens != 15 {
		t.Errorf("usage wrong: %+v", usage)
	}
}

func TestAdapterFromType_ClaudeCode(t *testing.T) {
	a, err := AdapterFromType(ProviderClaudeCode)
	if err != nil {
		t.Fatalf("AdapterFromType error: %v", err)
	}
	if _, ok := a.(*ClaudeCodeAdapter); !ok {
		t.Errorf("expected *ClaudeCodeAdapter, got %T", a)
	}
}

func TestComputeBillingHeader(t *testing.T) {
	h := computeBillingHeader("hello world test prompt input", "2.1.81")
	if !strings.HasPrefix(h, "x-anthropic-billing-header:") {
		t.Errorf("unexpected prefix: %s", h)
	}
	if !strings.Contains(h, "cch=") {
		t.Errorf("missing cch= in header: %s", h)
	}
	if !strings.Contains(h, "cc_version=2.1.81.") {
		t.Errorf("missing cc_version: %s", h)
	}
	// Deterministic for same input
	h2 := computeBillingHeader("hello world test prompt input", "2.1.81")
	if h != h2 {
		t.Error("billing header must be deterministic")
	}
}

func TestGenerateUserID(t *testing.T) {
	uid := generateUserID()
	if uid == "" {
		t.Fatal("user ID must not be empty")
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(uid), &parsed); err != nil {
		t.Fatalf("user ID must be valid JSON: %v", err)
	}
	if parsed["device_id"] == "" {
		t.Error("device_id missing")
	}
	if parsed["session_id"] == "" {
		t.Error("session_id missing")
	}
	uid2 := generateUserID()
	if uid == uid2 {
		t.Error("user IDs should be unique")
	}
}
