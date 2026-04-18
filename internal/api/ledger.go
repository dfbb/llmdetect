package api

import (
	"sync"

	"github.com/ironarmor/llmdetect/internal/provider"
)

// TokenLedger accumulates token usage per endpoint URL across concurrent requests.
type TokenLedger struct {
	mu    sync.Mutex
	usage map[string]*provider.TokenUsage
}

func NewTokenLedger() *TokenLedger {
	return &TokenLedger{usage: make(map[string]*provider.TokenUsage)}
}

func (l *TokenLedger) Add(url string, u provider.TokenUsage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.usage[url]; ok {
		existing.PromptTokens += u.PromptTokens
		existing.CompletionTokens += u.CompletionTokens
		existing.TotalTokens += u.TotalTokens
	} else {
		l.usage[url] = &provider.TokenUsage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		}
	}
}

// Snapshot returns a copy of all accumulated usage. Safe to call concurrently with Add.
func (l *TokenLedger) Snapshot() map[string]provider.TokenUsage {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]provider.TokenUsage, len(l.usage))
	for k, v := range l.usage {
		out[k] = *v
	}
	return out
}

// Total sums usage across all URLs.
func (l *TokenLedger) Total() provider.TokenUsage {
	l.mu.Lock()
	defer l.mu.Unlock()
	var total provider.TokenUsage
	for _, v := range l.usage {
		total.PromptTokens += v.PromptTokens
		total.CompletionTokens += v.CompletionTokens
		total.TotalTokens += v.TotalTokens
	}
	return total
}
