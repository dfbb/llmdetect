package detector

import (
	"context"
	"net/url"
	"sync"

	"golang.org/x/net/publicsuffix"
	"golang.org/x/time/rate"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
)

type ChannelResult struct {
	Endpoint   config.Endpoint
	TVDistance float64
	PerInputTV []float64
	Verdict    string // "original" or "spoofed"
}

// ProbeChannels runs Phase 2: probe each channel and compute TV distance vs official distribution.
// Channels with different root domains run in parallel; same root domain run serially.
func ProbeChannels(ctx context.Context, cfg *config.Config, model *config.ModelConfig,
	channels []config.Endpoint, bis []cache.BorderInput,
	newClient func(ep config.Endpoint) *api.Client) []ChannelResult {

	groups := groupByDomain(channels)

	var mu sync.Mutex
	allResults := make([]ChannelResult, 0, len(channels))

	var wg sync.WaitGroup
	for _, group := range groups {
		wg.Add(1)
		go func(grp []config.Endpoint) {
			defer wg.Done()
			for _, ch := range grp {
				r := probeOne(ctx, cfg, model, ch, bis, newClient(ch))
				mu.Lock()
				allResults = append(allResults, r)
				mu.Unlock()
			}
		}(group)
	}
	wg.Wait()
	return allResults
}

func probeOne(ctx context.Context, cfg *config.Config, model *config.ModelConfig,
	ch config.Endpoint, bis []cache.BorderInput, client *api.Client) ChannelResult {

	limiter := rate.NewLimiter(rate.Limit(cfg.Concurrency.RateLimitRPS), cfg.Concurrency.MaxWorkersPerChannel)
	sem := make(chan struct{}, cfg.Concurrency.MaxWorkersPerChannel)

	tvResults := make([]float64, len(bis))
	var wg sync.WaitGroup

	for i, bi := range bis {
		wg.Add(1)
		go func(idx int, b cache.BorderInput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dist := make(map[string]int)
			for j := 0; j < cfg.Detection.QueriesPerInput; j++ {
				if err := limiter.Wait(ctx); err != nil {
					break
				}
				resp, err := client.QueryOnce(ctx, model.Model, b.Prompt)
				if err != nil {
					continue
				}
				dist[resp]++
			}
			tvResults[idx] = ComputeTV(b.OfficialDistribution, dist)
		}(i, bi)
	}
	wg.Wait()

	avg := AverageTV(tvResults)
	verdict := "original"
	if avg >= cfg.Detection.TVThreshold {
		verdict = "spoofed"
	}
	return ChannelResult{
		Endpoint:   ch,
		TVDistance: avg,
		PerInputTV: tvResults,
		Verdict:    verdict,
	}
}

func rootDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(u.Hostname())
	if err != nil {
		return u.Hostname()
	}
	return domain
}

func groupByDomain(channels []config.Endpoint) [][]config.Endpoint {
	order := make([]string, 0)
	groups := make(map[string][]config.Endpoint)
	for _, ch := range channels {
		d := rootDomain(ch.URL)
		if _, seen := groups[d]; !seen {
			order = append(order, d)
		}
		groups[d] = append(groups[d], ch)
	}
	result := make([][]config.Endpoint, 0, len(order))
	for _, d := range order {
		result = append(result, groups[d])
	}
	return result
}
