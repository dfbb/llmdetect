package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/ironarmor/llmdetect/internal/provider"
)

type Client struct {
	baseURL    string
	apiKey     string
	maxRetries int
	http       *http.Client
	adapter    provider.Adapter
	ledger     *TokenLedger
}

// NewClient returns a Client using the OpenAI adapter with no token tracking.
// Existing callers (online-check, discover, probe) use this for backward compatibility.
func NewClient(baseURL, apiKey string, timeoutSeconds, maxRetries int) *Client {
	return NewClientFull(baseURL, apiKey, timeoutSeconds, maxRetries, &provider.OpenAIAdapter{}, nil)
}

// NewClientFull returns a Client with a specific adapter and optional TokenLedger.
func NewClientFull(baseURL, apiKey string, timeoutSeconds, maxRetries int, a provider.Adapter, ledger *TokenLedger) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		maxRetries: maxRetries,
		http:       &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		adapter:    a,
		ledger:     ledger,
	}
}

// QueryOnce sends a single request using the client's adapter and returns the output token.
// Token usage is accumulated to the ledger if one is configured.
func (c *Client) QueryOnce(ctx context.Context, model, prompt string) (string, error) {
	body, err := c.adapter.BuildRequest(model, prompt, 1)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
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
			c.baseURL+c.adapter.RequestPath(), bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		for k, v := range c.adapter.Headers(c.apiKey) {
			req.Header.Set(k, v)
		}

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

		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		token, usage, err := c.adapter.ParseResponse(b)
		if err != nil {
			lastErr = err
			continue
		}
		if c.ledger != nil {
			c.ledger.Add(c.baseURL, usage)
		}
		return token, nil
	}
	return "", fmt.Errorf("all retries failed: %w", lastErr)
}

// Ping sends a minimal request and returns true if the endpoint responds with HTTP 200.
func (c *Client) Ping(ctx context.Context, model string) bool {
	_, err := c.QueryOnce(ctx, model, "hi")
	return err == nil
}
