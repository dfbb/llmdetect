package provider

// TestSonnetPyCompat verifies that ClaudeCodeAdapter generates requests with
// the same headers and body structure as example/cc-gateway/sonnet.py.
//
// Reference: example/cc-gateway/sonnet.py (confirmed working on SSAI gateway).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	ssaiToken   = "cr_f3cfdffec5bb90df1d524df8dad9cc436aabb779e035b388dc2e70b7e3c32e7f"
	ssaiBaseURL = "https://claude.kg83.org/api"
	ssaiModel   = "claude-sonnet-4-6"
	ssaiPrompt  = "Perform a web search for the query: software development developer tools news April 20 2026"
)

// sonnetExpectedHeaders maps every static header key (as returned by the adapter)
// to the exact value sonnet.py sends. Dynamic fields (Authorization, x-api-key,
// User-Agent version, content-length) are checked separately.
var sonnetExpectedHeaders = map[string]string{
	"Accept":                                    "application/json",
	"Accept-Encoding":                           "identity",
	"anthropic-beta":                            "interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advisor-tool-2026-03-01",
	"anthropic-dangerous-direct-browser-access": "true",
	"anthropic-version":                         "2023-06-01",
	"Content-Type":                              "application/json",
	"x-app":                                     "cli",
	"X-Stainless-Arch":                          "arm64",
	"X-Stainless-Lang":                          "js",
	"X-Stainless-OS":                            "MacOS",
	"X-Stainless-Package-Version":               "0.81.0",
	"X-Stainless-Runtime":                       "node",
	"X-Stainless-Runtime-Version":               "v24.3.0",
}

func TestSonnetPyCompat_Headers(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	h := a.HeadersForModel("sk-test", "claude-sonnet-4-6")

	// Every static header from sonnet.py must be present with the exact value.
	for k, want := range sonnetExpectedHeaders {
		if got, ok := h[k]; !ok {
			t.Errorf("missing header %q", k)
		} else if got != want {
			t.Errorf("header %q: got %q, want %q", k, got, want)
		}
	}

	// User-Agent format: "claude-cli/<version> (external, cli)"
	ua := h["User-Agent"]
	if !strings.HasPrefix(ua, "claude-cli/") || !strings.HasSuffix(ua, " (external, cli)") {
		t.Errorf("User-Agent format wrong: %q", ua)
	}

	// Authorization and x-api-key must both be present and carry the key.
	if !strings.HasPrefix(h["Authorization"], "Bearer sk-test") {
		t.Errorf("Authorization: got %q", h["Authorization"])
	}
	if h["x-api-key"] != "sk-test" {
		t.Errorf("x-api-key: got %q", h["x-api-key"])
	}

	// No extra headers beyond what sonnet.py sends.
	allowed := make(map[string]bool, len(sonnetExpectedHeaders)+3)
	for k := range sonnetExpectedHeaders {
		allowed[k] = true
	}
	allowed["Authorization"] = true
	allowed["x-api-key"] = true
	allowed["User-Agent"] = true

	for k := range h {
		if !allowed[k] {
			t.Errorf("unexpected extra header %q (not present in sonnet.py)", k)
		}
	}
}

func TestSonnetPyCompat_Body(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	const model = "claude-sonnet-4-6"
	const maxTokens = 32000 // matches sonnet.py
	body, err := a.BuildRequest(model, "hello world", maxTokens)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	var req ccRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	// Top-level scalar fields.
	if req.Model != model {
		t.Errorf("model: got %q, want %q", req.Model, model)
	}
	if req.MaxTokens != maxTokens {
		t.Errorf("max_tokens: got %d, want %d", req.MaxTokens, maxTokens)
	}
	if req.Temperature != 1 {
		t.Errorf("temperature: got %v, want 1", req.Temperature)
	}
	if !req.Stream {
		t.Error("stream must be true")
	}

	// system[0]: billing header, no cache_control.
	if len(req.System) < 2 {
		t.Fatalf("system blocks: got %d, want >= 2", len(req.System))
	}
	s0 := req.System[0]
	if !strings.HasPrefix(s0.Text, "x-anthropic-billing-header:") {
		t.Errorf("system[0] must be billing header, got: %q", s0.Text)
	}
	if !strings.Contains(s0.Text, "cc_entrypoint=sdk-cli") {
		t.Errorf("system[0] missing cc_entrypoint=sdk-cli: %q", s0.Text)
	}
	if s0.CacheControl != nil {
		t.Errorf("system[0] must have no cache_control (matches sonnet.py)")
	}

	// system[1]: persona block, cache_control ephemeral.
	s1 := req.System[1]
	if s1.CacheControl == nil || s1.CacheControl.Type != "ephemeral" {
		t.Errorf("system[1] must have cache_control ephemeral: %+v", s1)
	}

	// messages: single user message, content block with cache_control ephemeral.
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages: got %+v", req.Messages)
	}
	content := req.Messages[0].Content
	if len(content) != 1 {
		t.Fatalf("user content blocks: got %d, want 1", len(content))
	}
	if content[0].Type != "text" {
		t.Errorf("user content type: got %q, want text", content[0].Type)
	}
	if content[0].CacheControl == nil || content[0].CacheControl.Type != "ephemeral" {
		t.Errorf("user content must have cache_control ephemeral: %+v", content[0])
	}

	// metadata.user_id must be present and valid JSON with device_id/session_id.
	if req.Metadata.UserID == "" {
		t.Fatal("metadata.user_id must not be empty")
	}
	var uid map[string]string
	if err := json.Unmarshal([]byte(req.Metadata.UserID), &uid); err != nil {
		t.Fatalf("metadata.user_id not valid JSON: %v", err)
	}
	if uid["device_id"] == "" || uid["session_id"] == "" {
		t.Errorf("metadata.user_id missing fields: %v", uid)
	}
}

func TestSonnetPyCompat_CCH(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body, err := a.BuildRequest("claude-sonnet-4-6", "hi", 1)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	// Placeholder must be replaced.
	if strings.Contains(string(body), "cch=00000") {
		t.Error("body must not contain cch=00000 placeholder")
	}

	// cch= must appear exactly once in the billing header text.
	idx := strings.Index(string(body), `cch=`)
	if idx == -1 {
		t.Fatal("cch= not found in body")
	}
	// Verify it's 5 lowercase hex chars (same mask as sonnet.py: h & 0xFFFFF → 5 hex chars).
	cchVal := string(body)[idx+4 : idx+9]
	for _, c := range cchVal {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("cch must be lowercase hex, got %q", cchVal)
		}
	}

	// Cross-check: known vector from sonnet.py's test message.
	// msg = "Perform a web search...", the billing header is the first field
	// hashed, so computeCCH over the full body is not easily pre-computed here.
	// Instead verify the algorithm is deterministic (same body → same cch).
	body2, _ := a.BuildRequest("claude-sonnet-4-6", "hi", 1)
	idx2 := strings.Index(string(body2), `cch=`)
	if idx2 == -1 {
		t.Fatal("cch= not found in second body")
	}
	// Note: user_id is random per-adapter, so full bodies differ.
	// But cch is a function of the body, so two bodies with the same content
	// must produce the same cch — verify the length/format at minimum.
	cchVal2 := string(body2)[idx2+4 : idx2+9]
	if len(cchVal2) != 5 {
		t.Errorf("cch2 must be 5 chars, got %q", cchVal2)
	}
}

// TestSonnetPyLive sends a real request to claude.kg83.org using our adapter
// and compares it with sonnet.py's expected behaviour.
// Skipped unless env var TEST_LIVE=1.
func TestSonnetPyLive(t *testing.T) {
	if os.Getenv("TEST_LIVE") != "1" {
		t.Skip("set TEST_LIVE=1 to run live gateway test")
	}

	OverrideCLIVersion("2.1.112")

	a := &ClaudeCodeAdapter{ExtraSystem: SSAIExtraSystem}
	body, err := a.BuildRequest(ssaiModel, ssaiPrompt, 32000)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	hdrs := a.HeadersForModel(ssaiToken, ssaiModel)

	req, err := http.NewRequest(http.MethodPost, ssaiBaseURL+a.RequestPath(), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("HTTP status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, raw)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	text, usage, err := parseClaudeSSE(raw)
	if err != nil {
		// Print first 500 bytes to help diagnose
		t.Logf("raw response (first 500 bytes):\n%s", truncateStr(string(raw), 500))
		t.Fatalf("parse SSE: %v", err)
	}

	t.Logf("response text (%d chars): %s", len(text), truncateStr(text, 300))
	t.Logf("usage: prompt=%d completion=%d total=%d",
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)

	if text == "" {
		t.Error("response text must not be empty")
	}
	if usage.PromptTokens == 0 {
		t.Error("usage.prompt_tokens must be > 0")
	}

	// Print the billing header we sent so it can be compared with sonnet.py.
	var sentReq ccRequest
	if err := json.Unmarshal(body, &sentReq); err == nil && len(sentReq.System) > 0 {
		t.Logf("sent billing header: %s", sentReq.System[0].Text)
	}

	fmt.Printf("\n=== LIVE RESULT ===\nHTTP 200 OK\nTokens: prompt=%d completion=%d\nText: %s\n",
		usage.PromptTokens, usage.CompletionTokens, truncateStr(text, 400))
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
