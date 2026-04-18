// internal/provider/openai.go
package provider

import (
	"encoding/json"
	"fmt"
)

// OpenAIAdapter implements Adapter for OpenAI and OpenRouter endpoints.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) Type() ProviderType  { return ProviderOpenAI }
func (a *OpenAIAdapter) RequestPath() string { return "/v1/chat/completions" }

func (a *OpenAIAdapter) Headers(apiKey string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (a *OpenAIAdapter) BuildRequest(model, prompt string, maxTokens int) ([]byte, error) {
	return json.Marshal(openAIRequest{
		Model:       model,
		Messages:    []openAIMessage{{Role: "user", Content: prompt}},
		MaxTokens:   maxTokens,
		Temperature: 0,
	})
}

func (a *OpenAIAdapter) ParseResponse(body []byte) (string, TokenUsage, error) {
	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", TokenUsage{}, fmt.Errorf("decode openai response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", TokenUsage{}, fmt.Errorf("empty choices in openai response")
	}
	return resp.Choices[0].Message.Content, TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}, nil
}
