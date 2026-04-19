package provider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	billingSalt    = "59cf53e54c78"
	defaultVersion = "2.1.112"
	versionTTL     = 7 * 24 * time.Hour
)

var versionCache struct {
	mu        sync.Mutex
	version   string
	fetchedAt time.Time
}

func fetchLatestCLIVersion() string {
	type release struct {
		TagName string `json:"tag_name"`
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/anthropics/claude-code/releases?per_page=1")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	var releases []release
	if err := json.Unmarshal(b, &releases); err != nil || len(releases) == 0 {
		return ""
	}
	v := releases[0].TagName
	if len(v) > 0 && v[0] == 'v' {
		v = v[1:]
	}
	return v
}

var fetchCLIVersion = fetchLatestCLIVersion

func cliVersion() string {
	versionCache.mu.Lock()
	defer versionCache.mu.Unlock()
	if versionCache.version != "" && time.Since(versionCache.fetchedAt) < versionTTL {
		return versionCache.version
	}
	if v := fetchCLIVersion(); v != "" {
		versionCache.version = v
		versionCache.fetchedAt = time.Now()
		return v
	}
	if versionCache.version != "" {
		return versionCache.version
	}
	return defaultVersion
}

func computeBillingHeader(messageText, version string) string {
	sample := func(i int) string {
		if i < len(messageText) {
			return string(messageText[i])
		}
		return "0"
	}
	sampled := sample(4) + sample(7) + sample(20)
	vhRaw := sha256.Sum256([]byte(billingSalt + sampled + version))
	versionHash := hex.EncodeToString(vhRaw[:])[:3]
	cchRaw := sha256.Sum256([]byte(messageText))
	cch := hex.EncodeToString(cchRaw[:])[:5]
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=cli; cch=%s;", version, versionHash, cch)
}

func generateUserID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	deviceID := hex.EncodeToString(b)
	sb := make([]byte, 16)
	if _, err := rand.Read(sb); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	sessionID := fmt.Sprintf("%x-%x-%x-%x-%x",
		sb[0:4], sb[4:6], sb[6:8], sb[8:10], sb[10:16])
	uid, _ := json.Marshal(map[string]string{
		"device_id":    deviceID,
		"account_uuid": "",
		"session_id":   sessionID,
	})
	return string(uid)
}

// ClaudeCodeAdapter mimics Claude Code CLI request fingerprint.
// Use this adapter when targeting Anthropic-compatible channels that
// gate access to authenticated Claude Code clients.
type ClaudeCodeAdapter struct{}

func (a *ClaudeCodeAdapter) Type() ProviderType  { return ProviderClaudeCode }
func (a *ClaudeCodeAdapter) RequestPath() string { return "/v1/messages" }

func (a *ClaudeCodeAdapter) Headers(apiKey string) map[string]string {
	ver := cliVersion()
	return map[string]string{
		"Authorization":                            "Bearer " + apiKey,
		"User-Agent":                               fmt.Sprintf("claude-cli/%s (external, cli)", ver),
		"x-app":                                    "cli",
		"anthropic-version":                        "2023-06-01",
		"anthropic-beta":                           "claude-code-20250219,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24",
		"anthropic-dangerous-direct-browser-access": "true",
		"Content-Type":            "application/json",
		"Accept":                  "application/json",
		"X-Stainless-Lang":        "js",
		"X-Stainless-Package-Version": "0.74.0",
		"X-Stainless-OS":          "Linux",
		"X-Stainless-Arch":        "x64",
		"X-Stainless-Runtime":     "node",
		"X-Stainless-Runtime-Version": "v24.3.0",
		"X-Stainless-Retry-Count": "0",
		"X-Stainless-Timeout":     "600",
	}
}

type ccSystemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ccMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ccMetadata struct {
	UserID string `json:"user_id"`
}

type ccThinking struct {
	Type string `json:"type"`
}

type ccOutputConfig struct {
	Effort string `json:"effort"`
}

type ccRequest struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	System       []ccSystemBlock `json:"system"`
	Messages     []ccMessage     `json:"messages"`
	Metadata     ccMetadata      `json:"metadata"`
	Thinking     ccThinking      `json:"thinking"`
	OutputConfig ccOutputConfig  `json:"output_config"`
}

func (a *ClaudeCodeAdapter) BuildRequest(model, prompt string, maxTokens int) ([]byte, error) {
	ver := cliVersion()
	billing := computeBillingHeader(prompt, ver)

	system := []ccSystemBlock{
		{Type: "text", Text: billing},
		{Type: "text", Text: "You are Claude Code, Anthropic's official CLI for Claude."},
	}

	return json.Marshal(ccRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []ccMessage{{Role: "user", Content: prompt}},
		Metadata:  ccMetadata{UserID: generateUserID()},
		Thinking:  ccThinking{Type: "adaptive"},
		OutputConfig: ccOutputConfig{Effort: "medium"},
	})
}

type ccResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (a *ClaudeCodeAdapter) ParseResponse(body []byte) (string, TokenUsage, error) {
	var resp ccResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", TokenUsage{}, fmt.Errorf("decode claude-code response: %w", err)
	}
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text, TokenUsage{
				PromptTokens:     resp.Usage.InputTokens,
				CompletionTokens: resp.Usage.OutputTokens,
				TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
			}, nil
		}
	}
	return "", TokenUsage{}, fmt.Errorf("no text block in claude-code response")
}
