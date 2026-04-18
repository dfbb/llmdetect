package report

import (
	"fmt"
	"time"

	"github.com/fatih/color"

	"github.com/ironarmor/llmdetect/config"
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
}
