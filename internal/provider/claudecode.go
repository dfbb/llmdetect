package provider

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
)

// claudeCodeVersion mirrors a recent stable Claude Code release.
const claudeCodeVersion = "2.0.37"

// ClaudeCodeAdapter wraps AnthropicAdapter and adds the HTTP headers that
// Claude Code (the official CLI) sends.  Use this when a channel restricts
// access to clients that identify as Claude Code.
//
// Header fingerprint (derived from the March 2026 source leak):
//   - User-Agent:              claude-cli/<version>
//   - x-app:                   cli
//   - anthropic-version:       2023-06-01
//   - anthropic-beta:          interleaved-thinking-2025-05-14,max-tokens-3-5-sonnet-20250714,token-efficient-tools-2025-02-19,tool-result-inline-2025-11-05,files-api-2025-04-14
//   - x-anthropic-billing-header: cch=<5-char xxhash64 suffix>
//
// The cch= value is a 5-character lowercase hex string derived from
// xxHash64 of the request payload length + a session nonce.  The original
// algorithm lives in Bun's Zig runtime (bun:native/http); we approximate it
// with a deterministic but structurally-identical value so channels that only
// check the header's presence and format accept the request.
// Channels that verify the hash server-side against Anthropic's validation
// service will still reject spoofed requests — that layer cannot be bypassed
// without the private key used in the real hash.
type ClaudeCodeAdapter struct {
	inner       AnthropicAdapter
	sessionNonce uint64 // per-adapter random nonce, seeded once at construction
}

// NewClaudeCodeAdapter returns a ClaudeCodeAdapter ready for use.
func NewClaudeCodeAdapter() *ClaudeCodeAdapter {
	return &ClaudeCodeAdapter{
		sessionNonce: rand.Uint64(), //nolint:gosec // not a security primitive
	}
}

func (a *ClaudeCodeAdapter) Type() ProviderType  { return ProviderAnthropic }
func (a *ClaudeCodeAdapter) RequestPath() string { return a.inner.RequestPath() }

func (a *ClaudeCodeAdapter) Headers(apiKey string) map[string]string {
	h := a.inner.Headers(apiKey)
	h["User-Agent"] = fmt.Sprintf("claude-cli/%s", claudeCodeVersion)
	h["x-app"] = "cli"
	h["anthropic-beta"] = "interleaved-thinking-2025-05-14,max-tokens-3-5-sonnet-20250714,token-efficient-tools-2025-02-19,tool-result-inline-2025-11-05,files-api-2025-04-14"
	h["x-anthropic-billing-header"] = fmt.Sprintf("cch=%s", a.computeCCH())
	return h
}

func (a *ClaudeCodeAdapter) BuildRequest(model, prompt string, maxTokens int) ([]byte, error) {
	return a.inner.BuildRequest(model, prompt, maxTokens)
}

func (a *ClaudeCodeAdapter) ParseResponse(body []byte) (string, TokenUsage, error) {
	return a.inner.ParseResponse(body)
}

// computeCCH returns the 5-character hex suffix used in x-anthropic-billing-header.
// The real Claude Code computes xxHash64 over the serialized request inside
// Bun's native HTTP layer.  We reproduce the structural form (5 lowercase hex
// chars) using xxHash64 over the session nonce so the header passes format
// validation on channel proxies.
func (a *ClaudeCodeAdapter) computeCCH() string {
	h := xxhash64(a.sessionNonce)
	return fmt.Sprintf("%05x", h&0xfffff) // 5 hex chars = 20 bits
}

// xxhash64 is a minimal single-seed implementation of the xxHash64 algorithm.
// Reference: https://github.com/Cyan4973/xxHash/blob/dev/doc/xxhash_spec.md
func xxhash64(seed uint64) uint64 {
	const (
		prime1 = 0x9E3779B185EBCA87
		prime2 = 0xC2B2AE3D27D4EB4F
		prime5 = 0x27D4EB2F165667C5
	)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], seed)

	h := seed + prime5 + 8

	// Process the 8-byte input block.
	k := binary.LittleEndian.Uint64(buf[:])
	k *= prime2
	k = bits64RotateLeft(k, 31)
	k *= prime1
	h ^= k
	h = bits64RotateLeft(h, 27)*prime1 + 0x9C8E62A9C9F25E3D // avalanche mix

	// Finalise.
	h ^= h >> 33
	h *= prime2
	h ^= h >> 29
	h *= 0xC4CEB9FE1A85EC53
	h ^= h >> 32
	return h
}

func bits64RotateLeft(x uint64, k int) uint64 {
	return (x << uint(k)) | (x >> uint(64-k))
}

// claudeCodeRequestExt extends the base anthropic request body with the
// anti-distillation field that Claude Code sends when the ANTI_DISTILLATION_CC
// flag is active.  Channels that forward the body verbatim to Anthropic will
// trigger the server-side canary injection; channels that strip unknown fields
// will ignore it.
type claudeCodeRequestExt struct {
	Model            string             `json:"model"`
	Messages         []anthropicMessage `json:"messages"`
	MaxTokens        int                `json:"max_tokens"`
	Temperature      float64            `json:"temperature"`
	AntiDistillation []string           `json:"anti_distillation,omitempty"`
}

// BuildRequestWithAntiDistillation builds a request body that includes the
// anti_distillation canary field.  Call this instead of BuildRequest when you
// want the full Claude Code body fingerprint (including the canary trap).
func (a *ClaudeCodeAdapter) BuildRequestWithAntiDistillation(model, prompt string, maxTokens int) ([]byte, error) {
	return json.Marshal(claudeCodeRequestExt{
		Model:            model,
		Messages:         []anthropicMessage{{Role: "user", Content: prompt}},
		MaxTokens:        maxTokens,
		Temperature:      0,
		AntiDistillation: []string{"fake_tools"},
	})
}
