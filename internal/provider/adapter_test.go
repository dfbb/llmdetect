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

func TestMaybeUpgradeToClaudeCode(t *testing.T) {
	cases := []struct {
		name      string
		adapter   provider.Adapter
		model     string
		wantType  provider.ProviderType
	}{
		{"anthropic + claude model → upgrade",   &provider.AnthropicAdapter{}, "claude-opus-4-5",     provider.ProviderClaudeCode},
		{"anthropic + Claude uppercase → upgrade", &provider.AnthropicAdapter{}, "Claude-3-5-Sonnet",  provider.ProviderClaudeCode},
		{"anthropic + non-claude model → keep",  &provider.AnthropicAdapter{}, "gpt-4o",              provider.ProviderAnthropic},
		{"openai + claude model → keep openai",  &provider.OpenAIAdapter{},    "claude-opus-4-5",     provider.ProviderOpenAI},
		{"claude-code + claude model → no-op",   &provider.ClaudeCodeAdapter{}, "claude-opus-4-5",    provider.ProviderClaudeCode},
		{"anthropic + empty model → keep",       &provider.AnthropicAdapter{}, "",                    provider.ProviderAnthropic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := provider.MaybeUpgradeToClaudeCode(tc.adapter, tc.model)
			if got.Type() != tc.wantType {
				t.Errorf("got %s, want %s", got.Type(), tc.wantType)
			}
		})
	}
}
