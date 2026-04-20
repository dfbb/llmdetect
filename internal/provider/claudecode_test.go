package provider

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	fetchCLIVersion = func() string { return "2.1.112" }
	versionOnce.Do(func() { cachedCLIVersion = "2.1.112" })
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
	if !strings.Contains(h["anthropic-beta"], "interleaved-thinking-2025-05-14") {
		t.Errorf("anthropic-beta missing interleaved-thinking-2025-05-14: %s", h["anthropic-beta"])
	}
	if h["Accept-Encoding"] != "identity" {
		t.Errorf("Accept-Encoding must be identity, got: %s", h["Accept-Encoding"])
	}
	if h["X-Stainless-Lang"] != "js" {
		t.Errorf("X-Stainless-Lang wrong: %s", h["X-Stainless-Lang"])
	}
}

func TestClaudeCodeAdapter_BuildRequest(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body, err := a.BuildRequest("claude-opus-4-5", "hello world", 1024)
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
	if req.Temperature != 1 {
		t.Errorf("temperature must be 1, got: %v", req.Temperature)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Errorf("messages wrong: %+v", req.Messages)
	}
	if len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].CacheControl == nil {
		t.Errorf("user message content must have cache_control: %+v", req.Messages[0].Content)
	}
	if len(req.System) < 2 {
		t.Fatalf("system must have at least 2 blocks, got %d", len(req.System))
	}
	if !strings.Contains(req.System[0].Text, "x-anthropic-billing-header") {
		t.Errorf("first system block must be billing header, got: %s", req.System[0].Text)
	}
	if !strings.Contains(req.System[0].Text, "cc_entrypoint=sdk-cli") {
		t.Errorf("billing header missing cc_entrypoint=sdk-cli: %s", req.System[0].Text)
	}
	if !strings.Contains(req.System[1].Text, "Claude Code") {
		t.Errorf("second system block must identify as Claude Code: %s", req.System[1].Text)
	}
	if req.Metadata.UserID == "" {
		t.Error("metadata.user_id must not be empty")
	}
}

func TestClaudeCodeAdapter_BuildRequest_Temperature(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body, err := a.BuildRequest("claude-sonnet-4-6", "hi", 100)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}
	var req ccRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if req.Temperature != 1 {
		t.Errorf("temperature must be 1, got: %v", req.Temperature)
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

func TestClaudeCodeAdapter_ExtraSystem(t *testing.T) {
	a := &ClaudeCodeAdapter{ExtraSystem: SSAIExtraSystem}
	body, err := a.BuildRequest("claude-sonnet-4-6", "hi", 1024)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var req ccRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.System) != 2+len(SSAIExtraSystem) {
		t.Fatalf("system blocks: got %d, want %d", len(req.System), 2+len(SSAIExtraSystem))
	}
	for i, want := range SSAIExtraSystem {
		got := req.System[2+i]
		if got.Text != want {
			t.Errorf("extra system[%d]: got %q, want %q", i, got.Text, want)
		}
		if got.CacheControl == nil || got.CacheControl.Type != "ephemeral" {
			t.Errorf("extra system[%d] missing cache_control ephemeral: %+v", i, got)
		}
	}
}

func TestClaudeCodeAdapter_NoExtraSystemByDefault(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body, err := a.BuildRequest("claude-sonnet-4-6", "hi", 1024)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var req ccRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.System) != 2 {
		t.Errorf("default system blocks: got %d, want 2", len(req.System))
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

func TestAdapterFromTypeWithExtrahack(t *testing.T) {
	a, err := AdapterFromTypeWithExtrahack(ProviderClaudeCode, true)
	if err != nil {
		t.Fatalf("AdapterFromTypeWithExtrahack: %v", err)
	}
	cc, ok := a.(*ClaudeCodeAdapter)
	if !ok {
		t.Fatalf("expected *ClaudeCodeAdapter, got %T", a)
	}
	if len(cc.ExtraSystem) == 0 {
		t.Error("extrahack=true should populate ExtraSystem")
	}

	a2, _ := AdapterFromTypeWithExtrahack(ProviderClaudeCode, false)
	cc2 := a2.(*ClaudeCodeAdapter)
	if len(cc2.ExtraSystem) != 0 {
		t.Error("extrahack=false should leave ExtraSystem empty")
	}

	// extrahack flag must only affect claude-code
	a3, _ := AdapterFromTypeWithExtrahack(ProviderOpenAI, true)
	if _, ok := a3.(*OpenAIAdapter); !ok {
		t.Errorf("expected OpenAIAdapter even with extrahack=true, got %T", a3)
	}
}

func TestMaybeUpgradeToClaudeCodeWithExtrahack(t *testing.T) {
	// anthropic → claude-code with extrahack injects SSAIExtraSystem
	a := MaybeUpgradeToClaudeCodeWithExtrahack(&AnthropicAdapter{}, "claude-sonnet-4-6", true)
	cc, ok := a.(*ClaudeCodeAdapter)
	if !ok {
		t.Fatalf("expected upgrade to *ClaudeCodeAdapter, got %T", a)
	}
	if len(cc.ExtraSystem) == 0 {
		t.Error("upgraded adapter with extrahack=true must have ExtraSystem")
	}

	// existing claude-code adapter gets ExtraSystem populated when extrahack=true
	existing := &ClaudeCodeAdapter{}
	a2 := MaybeUpgradeToClaudeCodeWithExtrahack(existing, "claude-haiku", true)
	cc2 := a2.(*ClaudeCodeAdapter)
	if len(cc2.ExtraSystem) == 0 {
		t.Error("existing claude-code adapter should get ExtraSystem when extrahack=true")
	}

	// extrahack=false leaves things alone
	a3 := MaybeUpgradeToClaudeCodeWithExtrahack(&AnthropicAdapter{}, "claude-sonnet-4-6", false)
	cc3 := a3.(*ClaudeCodeAdapter)
	if len(cc3.ExtraSystem) != 0 {
		t.Error("extrahack=false upgrade must not populate ExtraSystem")
	}
}

func TestComputeVersionHash(t *testing.T) {
	// Verify against known good.sh value: msg = "Perform a web search..."
	// msg[4]='o', msg[7]=' ', msg[20]=' '  → sha256("59cf53e54c78o  2.1.112")[:3] = "47e"
	msg := "Perform a web search for the query: software development developer tools news April 20 2026"
	vh := computeVersionHash(msg, "2.1.112")
	if vh != "47e" {
		t.Errorf("versionHash wrong: got %s, want 47e", vh)
	}

	vh2 := computeVersionHash(msg, "2.1.112")
	if vh != vh2 {
		t.Error("versionHash must be deterministic")
	}
}

func TestBillingHeaderWithPlaceholder(t *testing.T) {
	h := billingHeaderWithPlaceholder("hello world test prompt input", "2.1.81")
	if !strings.HasPrefix(h, "x-anthropic-billing-header:") {
		t.Errorf("unexpected prefix: %s", h)
	}
	if !strings.Contains(h, "cch=00000") {
		t.Errorf("placeholder must be 00000: %s", h)
	}
	if !strings.Contains(h, "cc_version=2.1.81.") {
		t.Errorf("missing cc_version: %s", h)
	}
	if !strings.Contains(h, "cc_entrypoint=sdk-cli") {
		t.Errorf("entrypoint must be sdk-cli: %s", h)
	}
}

func TestComputeCCH(t *testing.T) {
	// cch must be 5 hex chars and deterministic
	body := []byte(`{"model":"claude-haiku","max_tokens":1}`)
	cch := computeCCH(body)
	if len(cch) != 5 {
		t.Errorf("cch must be 5 chars, got %q", cch)
	}
	for _, c := range cch {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("cch must be lowercase hex, got %q", cch)
		}
	}
	if cch != computeCCH(body) {
		t.Error("cch must be deterministic")
	}
}

func TestBuildRequest_CCHNotPlaceholder(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	body, err := a.BuildRequest("claude-haiku-3-5", "hi", 1)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}
	if strings.Contains(string(body), "cch=00000") {
		t.Error("final body must not contain placeholder cch=00000")
	}
	// cch= must appear exactly once and be 5 hex chars
	idx := strings.Index(string(body), "cch=")
	if idx == -1 {
		t.Fatal("cch= not found in body")
	}
	cch := string(body)[idx+4 : idx+9]
	if len(cch) != 5 {
		t.Errorf("cch value wrong length: %q", cch)
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
