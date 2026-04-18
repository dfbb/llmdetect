package online

import (
	"context"
	"sync"
	"time"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
)

type Result struct {
	Endpoint config.Endpoint
	Online   bool
	Latency  time.Duration
}

// CheckAll concurrently pings all given endpoints and returns one Result per endpoint.
func CheckAll(cfg *config.Config, model string, endpoints []config.Endpoint) []Result {
	results := make([]Result, len(endpoints))
	var wg sync.WaitGroup
	for i, ep := range endpoints {
		wg.Add(1)
		go func(idx int, endpoint config.Endpoint) {
			defer wg.Done()
			// Both NewClient's per-request timeout and the context's deadline are set to the same
			// duration: per-request prevents a single attempt from hanging, context prevents
			// accumulated retry time from exceeding the budget.
			c := api.NewClient(endpoint.URL, endpoint.Key,
				cfg.Concurrency.TimeoutSeconds, 1)
			ctx, cancel := context.WithTimeout(context.Background(),
				time.Duration(cfg.Concurrency.TimeoutSeconds)*time.Second)
			defer cancel()
			start := time.Now()
			ok := c.Ping(ctx, model)
			results[idx] = Result{
				Endpoint: endpoint,
				Online:   ok,
				Latency:  time.Since(start),
			}
		}(i, ep)
	}
	wg.Wait()
	return results
}
