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
	debugOut   io.Writer // nil = silent
}

// NewClient returns a Client using the OpenAI adapter with no token tracking.
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

// SetDebug enables debug logging to w for every request this client makes.
func (c *Client) SetDebug(w io.Writer) { c.debugOut = w }

func (c *Client) debugf(format string, args ...any) {
	if c.debugOut != nil {
		fmt.Fprintf(c.debugOut, "[debug] "+format+"\n", args...)
	}
}

// QueryOnce sends a single request using the client's adapter and returns the output token.
// Token usage is accumulated to the ledger if one is configured.
func (c *Client) QueryOnce(ctx context.Context, model, prompt string) (string, error) {
	body, err := c.adapter.BuildRequest(model, prompt, 1)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	url := c.baseURL + c.adapter.RequestPath()
	c.debugf("adapter=%s  url=%s", c.adapter.Type(), url)
	if c.debugOut != nil {
		hdrs := c.adapter.Headers(c.apiKey)
		for k, v := range hdrs {
			// Redact key values
			if k == "x-api-key" || k == "Authorization" {
				if len(v) > 12 {
					v = v[:8] + "..." + v[len(v)-4:]
				}
			}
			fmt.Fprintf(c.debugOut, "[debug]   %s: %s\n", k, v)
		}
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

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		for k, v := range c.adapter.Headers(c.apiKey) {
			req.Header.Set(k, v)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			c.debugf("attempt %d: request error: %v", attempt, err)
			lastErr = err
			continue
		}

		c.debugf("attempt %d: HTTP %d", attempt, resp.StatusCode)

		if resp.StatusCode == http.StatusUnauthorized {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.debugf("401 body: %s", truncate(string(b), 200))
			return "", fmt.Errorf("API returned 401 unauthorized — check your API key for %s", c.baseURL)
		}
		if resp.StatusCode >= 500 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.debugf("5xx body: %s", truncate(string(b), 200))
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(b))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.debugf("non-200 body: %s", truncate(string(b), 200))
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
			c.debugf("parse error: %v", err)
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
