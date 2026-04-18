package api_test

import (
	"sync"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestLedger_AddAndSnapshot(t *testing.T) {
	l := api.NewTokenLedger()
	l.Add("https://api.openai.com/v1", provider.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12})

	snap := l.Snapshot()
	u, ok := snap["https://api.openai.com/v1"]
	if !ok {
		t.Fatal("URL not found in snapshot")
	}
	if u.PromptTokens != 10 || u.CompletionTokens != 2 || u.TotalTokens != 12 {
		t.Errorf("got %+v, want {10 2 12}", u)
	}
}

func TestLedger_Accumulates(t *testing.T) {
	l := api.NewTokenLedger()
	url := "https://api.openai.com/v1"
	l.Add(url, provider.TokenUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6})
	l.Add(url, provider.TokenUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6})

	snap := l.Snapshot()
	u := snap[url]
	if u.PromptTokens != 10 || u.TotalTokens != 12 {
		t.Errorf("accumulation wrong: got %+v, want {10 2 12}", u)
	}
}

func TestLedger_Total(t *testing.T) {
	l := api.NewTokenLedger()
	l.Add("https://a.com", provider.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12})
	l.Add("https://b.com", provider.TokenUsage{PromptTokens: 20, CompletionTokens: 4, TotalTokens: 24})

	total := l.Total()
	if total.PromptTokens != 30 || total.CompletionTokens != 6 || total.TotalTokens != 36 {
		t.Errorf("total wrong: got %+v, want {30 6 36}", total)
	}
}

func TestLedger_SnapshotIsImmutable(t *testing.T) {
	l := api.NewTokenLedger()
	url := "https://a.com"
	l.Add(url, provider.TokenUsage{TotalTokens: 5})
	snap := l.Snapshot()
	// Modify the snapshot copy — ledger should not change
	copied := snap[url]
	copied.TotalTokens = 9999
	snap[url] = copied

	total := l.Total()
	if total.TotalTokens != 5 {
		t.Errorf("snapshot mutation affected ledger: got %d", total.TotalTokens)
	}
}

func TestLedger_Concurrent(t *testing.T) {
	l := api.NewTokenLedger()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Add("https://a.com", provider.TokenUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2})
		}()
	}
	wg.Wait()

	total := l.Total()
	if total.TotalTokens != 200 {
		t.Errorf("concurrent total wrong: got %d, want 200", total.TotalTokens)
	}
}
