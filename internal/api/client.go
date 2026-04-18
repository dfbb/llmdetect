package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	maxRetries int
	http       *http.Client
}

func NewClient(baseURL, apiKey string, timeoutSeconds, maxRetries int) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		maxRetries: maxRetries,
		http:       &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// QueryOnce sends a single chat completion request and returns the response token.
func (c *Client) QueryOnce(ctx context.Context, model, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       model,
		Messages:    []chatMessage{{Role: "user", Content: prompt}},
		MaxTokens:   1,
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(math.Pow(2, float64(attempt-1))) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return "", fmt.Errorf("API returned 401 unauthorized — check your API key for %s", c.baseURL)
		}
		if resp.StatusCode >= 500 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(b))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
		}

		var result chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("decode response: %w", err)
			continue
		}
		resp.Body.Close()
		if len(result.Choices) == 0 {
			return "", fmt.Errorf("empty choices in response")
		}
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("all retries failed: %w", lastErr)
}

// Ping sends a minimal request and returns true if the endpoint responds with 200.
func (c *Client) Ping(ctx context.Context, model string) bool {
	_, err := c.QueryOnce(ctx, model, "hi")
	return err == nil
}
