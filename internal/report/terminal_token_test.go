package report_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/provider"
	"github.com/ironarmor/llmdetect/internal/report"
)

func TestPrintTokenSummary_ShowsTable(t *testing.T) {
	ledger := api.NewTokenLedger()
	ledger.Add("https://api.openai.com/v1", provider.TokenUsage{PromptTokens: 12450, CompletionTokens: 620, TotalTokens: 13070})
	ledger.Add("https://api.xxx.com/v1", provider.TokenUsage{PromptTokens: 8200, CompletionTokens: 410, TotalTokens: 8610})

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	report.PrintTokenSummary(ledger)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "Token") {
		t.Errorf("expected token table header in output:\n%s", out)
	}
	if !strings.Contains(out, "12,450") {
		t.Errorf("expected formatted number 12,450 in output:\n%s", out)
	}
	// Total is 21,680 — output formats integers with commas
	if !strings.Contains(out, "21,680") {
		t.Errorf("expected total tokens 21,680 in output:\n%s", out)
	}
}

func TestPrintTokenSummary_EmptyLedger(t *testing.T) {
	ledger := api.NewTokenLedger()
	// Should not panic on empty ledger
	report.PrintTokenSummary(ledger)
}
