// internal/provider/anthropic.go
package provider

// AnthropicAdapter implements Adapter for the Anthropic Messages API.
type AnthropicAdapter struct{}

func (a *AnthropicAdapter) Type() ProviderType                                         { return ProviderAnthropic }
func (a *AnthropicAdapter) RequestPath() string                                        { return "" }
func (a *AnthropicAdapter) Headers(apiKey string) map[string]string                    { return nil }
func (a *AnthropicAdapter) BuildRequest(model, prompt string, max int) ([]byte, error) { return nil, nil }
func (a *AnthropicAdapter) ParseResponse(body []byte) (string, TokenUsage, error)       { return "", TokenUsage{}, nil }
