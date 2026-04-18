package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/detector"
)

var (
	checkMark = color.GreenString("✓")
	crossMark = color.RedString("✗")
	bold      = color.New(color.Bold).SprintFunc()
	separator = "────────────────────────────────────────────────────────────────"
)

func PrintSummary(params ReportParams, cfg *config.Config) {
	fmt.Printf("\n%s  model: %s   border inputs: %d   queries/input: %d   threshold: %.2f\n",
		bold("llmdetect"),
		bold(params.Model),
		cfg.Detection.BorderInputs,
		cfg.Detection.QueriesPerInput,
		cfg.Detection.TVThreshold,
	)
	fmt.Printf("run at: %s   duration: %.1fs\n\n",
		params.RunAt.Format(time.RFC3339),
		params.Duration,
	)

	if params.CacheStale {
		color.Yellow("⚠  cache is stale (age: %d minutes) — refresh failed, using old data\n", params.CacheAgeMinutes)
	}
	if params.Shortage {
		color.Yellow("⚠  border_inputs_found: %d (target: %d)\n", params.BorderInputsFound, cfg.Detection.BorderInputs)
	}

	fmt.Printf("%s\n%s\n", bold("Online Check"), separator)
	for _, r := range params.OnlineResults {
		mark := checkMark
		suffix := ""
		if !r.Online {
			mark = crossMark
			suffix = color.RedString("  [offline, skipped]")
		}
		fmt.Printf("  %s  %-20s  %s%s\n", mark, r.Endpoint.Name, r.Endpoint.URL, suffix)
	}
	fmt.Println()

	probeMap := make(map[string]detector.ChannelResult)
	for _, r := range params.ProbeResults {
		probeMap[r.Endpoint.URL] = r
	}

	fmt.Printf("%s\n%s\n", bold("Detection Results"), separator)
	fmt.Printf("  %-22s  %-10s  %s\n", "Channel", "TV Dist", "Verdict")
	fmt.Printf("  %s\n", separator[:60])
	for _, or_ := range params.OnlineResults {
		if !or_.Online {
			continue
		}
		pr, ok := probeMap[or_.Endpoint.URL]
		if !ok {
			continue
		}
		mark := checkMark + color.GreenString(" original")
		if pr.Verdict == "spoofed" {
			mark = crossMark + color.RedString(" spoofed")
		}
		fmt.Printf("  %-22s  %-10.3f  %s\n", or_.Endpoint.Name, pr.TVDistance, mark)
	}
	fmt.Printf("%s\n", separator)

	if params.Ledger != nil {
		PrintTokenSummary(params.Ledger)
	}
}

// formatInt formats an integer with comma thousands separators.
func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	offset := len(s) % 3
	for i, c := range []byte(s) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
}

// PrintTokenSummary prints per-URL token consumption after a detect run.
func PrintTokenSummary(ledger *api.TokenLedger) {
	snap := ledger.Snapshot()
	if len(snap) == 0 {
		return
	}
	total := ledger.Total()

	innerSep := strings.Repeat("─", 70)

	fmt.Printf("\n%s\n%s\n", bold("Token Usage"), separator)
	fmt.Printf("  %-45s  %8s  %6s  %8s\n", "URL", "Prompt", "Compl", "Total")
	fmt.Printf("  %s\n", innerSep)

	// Sort URLs for stable output
	urls := make([]string, 0, len(snap))
	for u := range snap {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	for _, u := range urls {
		u2 := snap[u]
		display := u
		if len(display) > 45 {
			display = "..." + display[len(display)-42:]
		}
		fmt.Printf("  %-45s  %8s  %6s  %8s\n",
			display,
			formatInt(u2.PromptTokens),
			formatInt(u2.CompletionTokens),
			formatInt(u2.TotalTokens),
		)
	}
	fmt.Printf("  %s\n", innerSep)
	fmt.Printf("  %-45s  %8s  %6s  %8s\n",
		bold("Total"),
		formatInt(total.PromptTokens),
		formatInt(total.CompletionTokens),
		formatInt(total.TotalTokens),
	)
	fmt.Printf("%s\n", separator)
}
