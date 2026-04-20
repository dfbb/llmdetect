package online

import (
	"context"
	"net/url"
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

// CheckAll pings all given endpoints and returns one Result per endpoint.
// Endpoints sharing the same host are checked serially to avoid hammering
// the same server concurrently; different hosts run in parallel.
func CheckAll(cfg *config.Config, model string, endpoints []config.Endpoint,
	newClient func(ep config.Endpoint) *api.Client) []Result {

	results := make([]Result, len(endpoints))

	type item struct {
		idx int
		ep  config.Endpoint
	}
	groupMap := make(map[string][]item)
	var order []string
	for i, ep := range endpoints {
		h := hostOf(ep.URL)
		if _, seen := groupMap[h]; !seen {
			order = append(order, h)
		}
		groupMap[h] = append(groupMap[h], item{i, ep})
	}

	var wg sync.WaitGroup
	for _, h := range order {
		wg.Add(1)
		go func(group []item) {
			defer wg.Done()
			for _, it := range group {
				c := newClient(it.ep)
				ctx, cancel := context.WithTimeout(context.Background(),
					time.Duration(cfg.Concurrency.TimeoutSeconds)*time.Second)
				start := time.Now()
				ok := c.Ping(ctx, model)
				cancel()
				results[it.idx] = Result{
					Endpoint: it.ep,
					Online:   ok,
					Latency:  time.Since(start),
				}
			}
		}(groupMap[h])
	}
	wg.Wait()
	return results
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
