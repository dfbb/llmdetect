package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
)

type JSONTokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type JSONReport struct {
	Model             string                     `json:"model"`
	RunAt             time.Time                  `json:"run_at"`
	DurationSeconds   float64                    `json:"duration_seconds"`
	Config            JSONReportConfig           `json:"config"`
	BorderInputsFound int                        `json:"border_inputs_found,omitempty"`
	CacheStale        bool                       `json:"cache_stale,omitempty"`
	CacheAgeMinutes   int                        `json:"cache_age_minutes,omitempty"`
	Results           []JSONChannelResult        `json:"results"`
	TokenSummary      map[string]JSONTokenUsage  `json:"token_summary,omitempty"`
	TotalTokens       *int                       `json:"total_tokens,omitempty"`
}

type JSONReportConfig struct {
	BorderInputs    int     `json:"border_inputs"`
	QueriesPerInput int     `json:"queries_per_input"`
	TVThreshold     float64 `json:"tv_threshold"`
}

type JSONChannelResult struct {
	Name       string          `json:"name"`
	URL        string          `json:"url"`
	Online     bool            `json:"online"`
	TVDistance *float64        `json:"tv_distance"`
	Verdict    string          `json:"verdict"`
	PerInputTV []float64       `json:"per_input_tv,omitempty"`
	TokensUsed *JSONTokenUsage `json:"tokens_used,omitempty"`
}

type ReportParams struct {
	Model             string
	RunAt             time.Time
	Duration          float64
	Cfg               *config.Config
	BorderInputsFound int
	Shortage          bool
	CacheStale        bool
	CacheAgeMinutes   int
	OnlineResults     []online.Result
	ProbeResults      []detector.ChannelResult
	Ledger            *api.TokenLedger
}

func WriteJSON(params ReportParams, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}

	probeMap := make(map[string]detector.ChannelResult)
	for _, r := range params.ProbeResults {
		probeMap[r.Endpoint.URL+r.Endpoint.Key] = r
	}

	var results []JSONChannelResult
	for _, or_ := range params.OnlineResults {
		cr := JSONChannelResult{
			Name:   or_.Endpoint.Name,
			URL:    or_.Endpoint.URL,
			Online: or_.Online,
		}
		if or_.Online {
			if pr, ok := probeMap[or_.Endpoint.URL+or_.Endpoint.Key]; ok {
				tv := pr.TVDistance
				cr.TVDistance = &tv
				cr.Verdict = pr.Verdict
				cr.PerInputTV = pr.PerInputTV
			}
		} else {
			cr.Verdict = "offline"
		}
		results = append(results, cr)
	}

	rep := JSONReport{
		Model:           params.Model,
		RunAt:           params.RunAt,
		DurationSeconds: params.Duration,
		Config: JSONReportConfig{
			BorderInputs:    params.Cfg.Detection.BorderInputs,
			QueriesPerInput: params.Cfg.Detection.QueriesPerInput,
			TVThreshold:     params.Cfg.Detection.TVThreshold,
		},
		Results: results,
	}
	if params.Shortage {
		rep.BorderInputsFound = params.BorderInputsFound
	}
	if params.CacheStale {
		rep.CacheStale = true
		rep.CacheAgeMinutes = params.CacheAgeMinutes
	}

	if params.Ledger != nil {
		snap := params.Ledger.Snapshot()
		for i, or_ := range params.OnlineResults {
			if !or_.Online {
				continue
			}
			if u, ok := snap[or_.Endpoint.URL]; ok {
				rep.Results[i].TokensUsed = &JSONTokenUsage{
					Prompt:     u.PromptTokens,
					Completion: u.CompletionTokens,
					Total:      u.TotalTokens,
				}
			}
		}
		summary := make(map[string]JSONTokenUsage, len(snap))
		for url, u := range snap {
			summary[url] = JSONTokenUsage{
				Prompt:     u.PromptTokens,
				Completion: u.CompletionTokens,
				Total:      u.TotalTokens,
			}
		}
		rep.TokenSummary = summary
		total := params.Ledger.Total()
		t := total.TotalTokens
		rep.TotalTokens = &t
	}

	ts := params.RunAt.Format("2006-01-02T15-04-05")
	filename := filepath.Join(dir, fmt.Sprintf("%s_%s.json", params.Model, ts))
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}
	return filename, os.WriteFile(filename, data, 0644)
}
