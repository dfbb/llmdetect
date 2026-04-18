// internal/provider/openai.go
package provider

// OpenAIAdapter implements Adapter for OpenAI and OpenRouter.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) Type() ProviderType                                         { return ProviderOpenAI }
func (a *OpenAIAdapter) RequestPath() string                                        { return "" }
func (a *OpenAIAdapter) Headers(apiKey string) map[string]string                    { return nil }
func (a *OpenAIAdapter) BuildRequest(model, prompt string, max int) ([]byte, error) { return nil, nil }
func (a *OpenAIAdapter) ParseResponse(body []byte) (string, TokenUsage, error)      { return "", TokenUsage{}, nil }
