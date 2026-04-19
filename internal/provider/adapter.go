package provider

import "fmt"

type ProviderType string

const (
	ProviderOpenAI      ProviderType = "openai"
	ProviderAnthropic   ProviderType = "anthropic"
	ProviderClaudeCode  ProviderType = "claude-code"
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
	case ProviderClaudeCode:
		return &ClaudeCodeAdapter{}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q: valid values are openai, anthropic, claude-code", p)
	}
}
