package provider

import (
	"strings"
	"testing"
)

func TestClaudeCodeAdapter_Type(t *testing.T) {
	a := NewClaudeCodeAdapter()
	if a.Type() != ProviderAnthropic {
		t.Fatalf("want ProviderAnthropic, got %q", a.Type())
	}
}

func TestClaudeCodeAdapter_RequestPath(t *testing.T) {
	a := NewClaudeCodeAdapter()
	if a.RequestPath() != "/v1/messages" {
		t.Fatalf("want /v1/messages, got %q", a.RequestPath())
	}
}

func TestClaudeCodeAdapter_Headers(t *testing.T) {
	a := NewClaudeCodeAdapter()
	h := a.Headers("sk-test-key")

	check := func(key, want string) {
		t.Helper()
		got, ok := h[key]
		if !ok {
			t.Errorf("header %q missing", key)
			return
		}
		if want != "" && got != want {
			t.Errorf("header %q: want %q, got %q", key, want, got)
		}
	}

	check("x-api-key", "sk-test-key")
	check("anthropic-version", "2023-06-01")
	check("User-Agent", "claude-cli/"+claudeCodeVersion)
	check("x-app", "cli")

	// anthropic-beta must contain the known beta flags
	beta := h["anthropic-beta"]
	for _, flag := range []string{
		"interleaved-thinking-2025-05-14",
		"token-efficient-tools-2025-02-19",
	} {
		if !strings.Contains(beta, flag) {
			t.Errorf("anthropic-beta missing %q, got %q", flag, beta)
		}
	}

	// cch= header must be present and have the right format
	cch, ok := h["x-anthropic-billing-header"]
	if !ok {
		t.Fatal("x-anthropic-billing-header missing")
	}
	if !strings.HasPrefix(cch, "cch=") {
		t.Fatalf("x-anthropic-billing-header must start with cch=, got %q", cch)
	}
	suffix := strings.TrimPrefix(cch, "cch=")
	if len(suffix) != 5 {
		t.Fatalf("cch suffix must be 5 chars, got %d: %q", len(suffix), suffix)
	}
	for _, c := range suffix {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("cch suffix must be lowercase hex, got %q", suffix)
		}
	}
}

func TestClaudeCodeAdapter_CCH_DeterministicPerInstance(t *testing.T) {
	a := NewClaudeCodeAdapter()
	first := a.computeCCH()
	second := a.computeCCH()
	if first != second {
		t.Errorf("cch must be deterministic per adapter instance: %q vs %q", first, second)
	}
}

func TestClaudeCodeAdapter_CCH_UniqueAcrossInstances(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		a := NewClaudeCodeAdapter()
		seen[a.computeCCH()] = true
	}
	// With 20 random nonces and 2^20 possible values, collisions are very rare.
	if len(seen) < 10 {
		t.Errorf("cch values not sufficiently unique across instances: %v", seen)
	}
}

func TestClaudeCodeAdapter_BuildRequest(t *testing.T) {
	a := NewClaudeCodeAdapter()
	b, err := a.BuildRequest("claude-3-5-sonnet-20241022", "hello", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty request body")
	}
}

func TestClaudeCodeAdapter_BuildRequestWithAntiDistillation(t *testing.T) {
	a := NewClaudeCodeAdapter()
	b, err := a.BuildRequestWithAntiDistillation("claude-3-5-sonnet-20241022", "hello", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "anti_distillation") {
		t.Errorf("expected anti_distillation in body, got: %s", b)
	}
	if !strings.Contains(string(b), "fake_tools") {
		t.Errorf("expected fake_tools in anti_distillation, got: %s", b)
	}
}

func TestAdapterFromType_ClaudeCode(t *testing.T) {
	a, err := AdapterFromType(ProviderClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(*ClaudeCodeAdapter); !ok {
		t.Fatalf("want *ClaudeCodeAdapter, got %T", a)
	}
}
