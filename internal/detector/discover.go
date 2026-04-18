package detector

import (
	"context"
	"sync"

	"golang.org/x/time/rate"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
)

type DiscoverResult struct {
	BorderInputs      []cache.BorderInput
	BorderInputsFound int
	Shortage          bool
}

type biCandidate struct {
	prompt  string
	outputs map[string]struct{}
}

// Discover runs Phase 1: find border inputs from the given token list using the official API.
//
// Phase 1a: Probe each token sequentially, making probeRounds queries per token to detect
// output variability. Sequential probing ensures consecutive server responses are captured
// per prompt, maximising sensitivity to stochastic models. Stops as soon as target BIs
// are found (early stop).
//
// Phase 1b: For each confirmed BI, build an official distribution by querying
// QueriesPerInput times in parallel (bounded by MaxWorkersPerChannel).
func Discover(ctx context.Context, cfg *config.Config, model *config.ModelConfig, tokens []string) (*DiscoverResult, error) {
	client := api.NewClient(model.Official.URL, model.Official.Key,
		cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)

	burst := cfg.Concurrency.MaxWorkersPerChannel
	if burst < 1 {
		burst = 1
	}
	limiter := rate.NewLimiter(rate.Limit(cfg.Concurrency.RateLimitRPS), burst)

	target := cfg.Detection.BorderInputs
	candidates := tokens
	if len(candidates) > cfg.Detection.DiscoveryCandidates {
		candidates = candidates[:cfg.Detection.DiscoveryCandidates]
	}

	const probeRounds = 3

	// Phase 1a: sequential probe per token so each prompt's queries are consecutive.
	var found []biCandidate
	for _, tok := range candidates {
		if len(found) >= target {
			break
		}

		outputs := make(map[string]struct{})
		for i := 0; i < probeRounds; i++ {
			if err := limiter.Wait(ctx); err != nil {
				return nil, ctx.Err()
			}
			resp, err := client.QueryOnce(ctx, model.Model, tok)
			if err != nil {
				break
			}
			outputs[resp] = struct{}{}
		}

		if len(outputs) >= 2 {
			found = append(found, biCandidate{prompt: tok, outputs: outputs})
		}
	}

	shortage := len(found) < target
	borderInputsFound := len(found)

	// Phase 1b: build official distribution for each BI using parallel workers.
	sem := make(chan struct{}, cfg.Concurrency.MaxWorkersPerChannel)
	var mu sync.Mutex
	bis := make([]cache.BorderInput, len(found))

	var wg sync.WaitGroup
	for i, cand := range found {
		wg.Add(1)
		go func(idx int, prompt string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dist := make(map[string]int)
			for j := 0; j < cfg.Detection.QueriesPerInput; j++ {
				if err := limiter.Wait(ctx); err != nil {
					break
				}
				resp, err := client.QueryOnce(ctx, model.Model, prompt)
				if err != nil {
					continue
				}
				dist[resp]++
			}

			mu.Lock()
			bis[idx] = cache.BorderInput{
				Prompt:               prompt,
				OfficialDistribution: dist,
			}
			mu.Unlock()
		}(i, cand.prompt)
	}
	wg.Wait()

	return &DiscoverResult{
		BorderInputs:      bis,
		BorderInputsFound: borderInputsFound,
		Shortage:          shortage,
	}, nil
}
