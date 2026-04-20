package provider

import (
	"fmt"
	"strings"
)

type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderClaudeCode ProviderType = "claude-code"
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
	return AdapterFromTypeWithExtrahack(p, false)
}

// SSAIExtraSystem is the persona pair that SSAI-style gateways
// (e.g. claude.kg83.org) require in the system array to bypass their
// prompt-caching validation. See docs/solution1.md.
var SSAIExtraSystem = []string{
	"You are a Claude agent, built on Anthropic's Claude Agent SDK.",
	"You are an assistant for performing a web search tool use",
}

// AdapterFromTypeWithExtrahack constructs an Adapter, and when extrahack is
// true and the provider is claude-code, injects SSAIExtraSystem into the
// ClaudeCodeAdapter's system array.
func AdapterFromTypeWithExtrahack(p ProviderType, extrahack bool) (Adapter, error) {
	switch p {
	case ProviderOpenAI:
		return &OpenAIAdapter{}, nil
	case ProviderAnthropic:
		return &AnthropicAdapter{}, nil
	case ProviderClaudeCode:
		a := &ClaudeCodeAdapter{}
		if extrahack {
			a.ExtraSystem = SSAIExtraSystem
		}
		return a, nil
	default:
		return nil, fmt.Errorf("unknown provider %q: valid values are openai, anthropic, claude-code", p)
	}
}

// MaybeUpgradeToClaudeCode replaces a plain AnthropicAdapter with ClaudeCodeAdapter
// when the model name starts with "claude". All other adapters are returned unchanged.
// This ensures Claude models are always queried with the full Claude Code fingerprint.
//
// Note: Detect() writes "anthropic" to the YAML before this upgrade runs, so the
// persisted provider value stays "anthropic". On the next run AdapterFromType returns
// AnthropicAdapter again, and this function upgrades it once more — correct behavior
// at runtime, but the YAML will never reflect "claude-code" for auto-detected endpoints.
func MaybeUpgradeToClaudeCode(a Adapter, modelName string) Adapter {
	return MaybeUpgradeToClaudeCodeWithExtrahack(a, modelName, false)
}

// MaybeUpgradeToClaudeCodeWithExtrahack is like MaybeUpgradeToClaudeCode but
// also injects SSAIExtraSystem into the upgraded adapter when extrahack is true.
func MaybeUpgradeToClaudeCodeWithExtrahack(a Adapter, modelName string, extrahack bool) Adapter {
	if _, ok := a.(*AnthropicAdapter); ok && strings.HasPrefix(strings.ToLower(modelName), "claude") {
		cc := &ClaudeCodeAdapter{}
		if extrahack {
			cc.ExtraSystem = SSAIExtraSystem
		}
		return cc
	}
	if cc, ok := a.(*ClaudeCodeAdapter); ok && extrahack && len(cc.ExtraSystem) == 0 {
		cc.ExtraSystem = SSAIExtraSystem
	}
	return a
}
