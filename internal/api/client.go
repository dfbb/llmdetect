package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
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
// Must be called before the client is shared across goroutines.
func (c *Client) SetDebug(w io.Writer) { c.debugOut = w }

func (c *Client) debugf(format string, args ...any) {
	if c.debugOut != nil {
		fmt.Fprintf(c.debugOut, "[debug] "+format+"\n", args...)
	}
}

// redactHeader masks the sensitive portion of auth header values.
// For "Bearer <token>" the token is redacted; for raw keys the whole value is redacted.
func redactHeader(key, val string) string {
	switch key {
	case "Authorization":
		const prefix = "Bearer "
		if strings.HasPrefix(val, prefix) {
			tok := val[len(prefix):]
			if len(tok) > 8 {
				return prefix + tok[:4] + "..." + tok[len(tok)-4:]
			}
			return val
		}
		if len(val) > 8 {
			return val[:4] + "..." + val[len(val)-4:]
		}
		return val
	case "x-api-key":
		if len(val) > 8 {
			return val[:4] + "..." + val[len(val)-4:]
		}
		return val
	}
	return val
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

		hdrs := c.adapter.Headers(c.apiKey)
		if c.debugOut != nil {
			for k, v := range hdrs {
				c.debugf("  %s: %s", k, redactHeader(k, v))
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		for k, v := range hdrs {
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
			c.debugf("parse error: %v | body: %s", err, truncate(string(b), 300))
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

// pingMaxTokens is large enough for gateways that enforce a minimum token budget
// (e.g. when thinking is present). Response body is discarded, so this is safe.
const pingMaxTokens = 100

// Ping sends a minimal request and returns true if the endpoint responds with HTTP 200.
// Body parsing is skipped — HTTP 200 alone is sufficient for a liveness check.
func (c *Client) Ping(ctx context.Context, model string) bool {
	body, err := c.adapter.BuildRequest(model, "hi", pingMaxTokens)
	if err != nil {
		return false
	}
	url := c.baseURL + c.adapter.RequestPath()
	c.debugf("ping adapter=%s  url=%s", c.adapter.Type(), url)
	c.debugf("ping body: %s", truncate(string(body), 500))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false
	}
	hdrs := c.adapter.Headers(c.apiKey)
	if c.debugOut != nil {
		for k, v := range hdrs {
			c.debugf("  %s: %s", k, redactHeader(k, v))
		}
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.debugf("ping error: %v", err)
		return false
	}
	c.debugf("ping HTTP %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		resp.Body.Close()
		c.debugf("ping non-200 body: %s", string(b))
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
