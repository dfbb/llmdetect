// internal/provider/anthropic.go
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
