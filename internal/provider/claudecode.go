package provider

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/OneOfOne/xxhash"
)

const (
	billingSalt    = "59cf53e54c78"
	defaultVersion = "2.1.112"
	cchSeed        = uint64(0x6E52736AC806831E)
	cchPlaceholder = "00000"
)

var (
	fetchCLIVersion  = fetchLatestCLIVersion
	versionOnce      sync.Once
	cachedCLIVersion string
)

// OverrideCLIVersion pins the CLI version used in headers and billing fields.
// Intended for testing and comparison tools; must be called before any adapter use.
func OverrideCLIVersion(v string) {
	versionOnce.Do(func() { cachedCLIVersion = v })
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

func cliVersion() string {
	versionOnce.Do(func() {
		if v := fetchCLIVersion(); v != "" {
			cachedCLIVersion = v
		} else {
			cachedCLIVersion = defaultVersion
		}
	})
	return cachedCLIVersion
}

func mustRandBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return b
}

// computeVersionHash returns the 3-char version suffix for the billing header.
// It is computed from chars at positions 4, 7, 20 of firstUserMessage + version.
func computeVersionHash(firstUserMessage, version string) string {
	sample := func(i int) string {
		if i < len(firstUserMessage) {
			return string(firstUserMessage[i])
		}
		return "0"
	}
	sampled := sample(4) + sample(7) + sample(20)
	raw := sha256.Sum256([]byte(billingSalt + sampled + version))
	return hex.EncodeToString(raw[:])[:3]
}

// computeCCH computes the 5-char cch field by xxhash64-ing the full request body
// (with cch=00000 as placeholder) and taking the low 20 bits as hex.
// The placeholder "cch=00000" in body is replaced with the result by BuildRequest.
func computeCCH(body []byte) string {
	h := xxhash.NewS64(cchSeed)
	h.Write(body)
	return fmt.Sprintf("%05x", h.Sum64()&0xFFFFF)
}

// billingHeaderWithPlaceholder returns a billing header string with cch=00000.
// BuildRequest replaces the placeholder after computing the real cch over the full body.
func billingHeaderWithPlaceholder(firstUserMessage, version string) string {
	vh := computeVersionHash(firstUserMessage, version)
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=sdk-cli; cch=%s;",
		version, vh, cchPlaceholder)
}

// generateUserID is kept for testing; production code uses ClaudeCodeAdapter.userID().
func generateUserID() string {
	deviceID := hex.EncodeToString(mustRandBytes(32))
	sb := mustRandBytes(16)
	sessionID := fmt.Sprintf("%x-%x-%x-%x-%x", sb[0:4], sb[4:6], sb[6:8], sb[8:10], sb[10:16])
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
//
// ExtraSystem is appended to the system array as additional ephemeral-cached
// text blocks. Some gateways (e.g. SSAI claude.kg83.org) validate that the
// system array contains specific persona substrings matching their
// prompt-caching prefix; populate ExtraSystem to inject them.
type ClaudeCodeAdapter struct {
	ExtraSystem []string

	initOnce  sync.Once
	deviceID  string
	sessionID string
}

func (a *ClaudeCodeAdapter) init() {
	a.initOnce.Do(func() {
		a.deviceID = hex.EncodeToString(mustRandBytes(32))
		sb := mustRandBytes(16)
		a.sessionID = fmt.Sprintf("%x-%x-%x-%x-%x", sb[0:4], sb[4:6], sb[6:8], sb[8:10], sb[10:16])
	})
}

// ccUserIDFields matches the real CC field order: device_id, account_uuid, session_id.
// Field order is preserved via struct tags (Go's encoding/json uses struct field order,
// not alphabetical like map[string]string).
type ccUserIDFields struct {
	DeviceID    string `json:"device_id"`
	AccountUUID string `json:"account_uuid"`
	SessionID   string `json:"session_id"`
}

func (a *ClaudeCodeAdapter) userID() string {
	a.init()
	uid, _ := json.Marshal(ccUserIDFields{
		DeviceID:    a.deviceID,
		AccountUUID: "",
		SessionID:   a.sessionID,
	})
	return string(uid)
}

func (a *ClaudeCodeAdapter) Type() ProviderType  { return ProviderClaudeCode }
func (a *ClaudeCodeAdapter) RequestPath() string { return "/v1/messages" }

func buildBetas(_ string) string {
	return strings.Join([]string{
		"interleaved-thinking-2025-05-14",
		"context-management-2025-06-27",
		"prompt-caching-scope-2026-01-05",
		"advisor-tool-2026-03-01",
	}, ",")
}

func (a *ClaudeCodeAdapter) Headers(apiKey string) map[string]string {
	return a.HeadersForModel(apiKey, "")
}

func (a *ClaudeCodeAdapter) HeadersForModel(apiKey, model string) map[string]string {
	a.init()
	ver := cliVersion()
	return map[string]string{
		"Authorization":                             "Bearer " + apiKey,
		"x-api-key":                                 apiKey,
		"User-Agent":                                fmt.Sprintf("claude-cli/%s (external, cli)", ver),
		"x-app":                                     "cli",
		"anthropic-version":                         "2023-06-01",
		"anthropic-beta":                            buildBetas(model),
		"anthropic-dangerous-direct-browser-access": "true",
		"Content-Type":                              "application/json",
		"Accept":                                    "application/json",
		"Accept-Encoding":                           "identity",
		"X-Stainless-Lang":                          "js",
		"X-Stainless-Package-Version":               "0.81.0",
		"X-Stainless-OS":                            "MacOS",
		"X-Stainless-Arch":                          "arm64",
		"X-Stainless-Runtime":                       "node",
		"X-Stainless-Runtime-Version":               "v24.3.0",
	}
}

type ccCacheControl struct {
	Type string `json:"type"`
}

type ccSystemBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl *ccCacheControl `json:"cache_control,omitempty"`
}

type ccMetadata struct {
	UserID string `json:"user_id"`
}

type ccContentBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl *ccCacheControl `json:"cache_control,omitempty"`
}

type ccMessage struct {
	Role    string           `json:"role"`
	Content []ccContentBlock `json:"content"`
}

type ccRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	System      []ccSystemBlock `json:"system"`
	Messages    []ccMessage     `json:"messages"`
	Metadata    ccMetadata      `json:"metadata"`
	Stream      bool            `json:"stream"`
}

func (a *ClaudeCodeAdapter) BuildRequest(model, prompt string, maxTokens int) ([]byte, error) {
	ver := cliVersion()

	systemBlocks := []ccSystemBlock{
		{Type: "text", Text: billingHeaderWithPlaceholder(prompt, ver)},
		{Type: "text", Text: "You are Claude Code, Anthropic's official CLI for Claude.", CacheControl: &ccCacheControl{Type: "ephemeral"}},
	}
	for _, extra := range a.ExtraSystem {
		systemBlocks = append(systemBlocks, ccSystemBlock{
			Type:         "text",
			Text:         extra,
			CacheControl: &ccCacheControl{Type: "ephemeral"},
		})
	}

	// Step 1: build body with cch placeholder so we can hash the full body.
	req := ccRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Temperature: 1,
		System:      systemBlocks,
		Messages: []ccMessage{{Role: "user", Content: []ccContentBlock{
			{Type: "text", Text: prompt, CacheControl: &ccCacheControl{Type: "ephemeral"}},
		}}},
		Metadata: ccMetadata{UserID: a.userID()},
		Stream:   true,
	}

	bodyWithPlaceholder, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Step 2: compute cch over the full body (with placeholder), then replace.
	cch := computeCCH(bodyWithPlaceholder)
	body := bytes.ReplaceAll(bodyWithPlaceholder,
		[]byte("cch="+cchPlaceholder),
		[]byte("cch="+cch))
	return body, nil
}

func (a *ClaudeCodeAdapter) ParseResponse(body []byte) (string, TokenUsage, error) {
	// Non-streaming: JSON object starts with '{'
	if len(body) > 0 && body[0] == '{' {
		var resp anthropicResponse
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
	// Streaming SSE response
	return parseClaudeSSE(body)
}

func parseClaudeSSE(body []byte) (string, TokenUsage, error) {
	type sseEvent struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Message struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	var sb strings.Builder
	var usage TokenUsage

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimRight(line, "\r")
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			usage.PromptTokens = ev.Message.Usage.InputTokens
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" {
				sb.WriteString(ev.Delta.Text)
			}
		case "message_delta":
			usage.CompletionTokens = ev.Usage.OutputTokens
		}
	}

	text := sb.String()
	if text == "" {
		return "", TokenUsage{}, fmt.Errorf("no text in claude-code SSE response")
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return text, usage, nil
}
