package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
	"github.com/ironarmor/llmdetect/internal/provider"
	"github.com/ironarmor/llmdetect/internal/report"
	"github.com/ironarmor/llmdetect/tokens"
)

var (
	flagModel  string
	flagConfig string
)

func main() {
	root := &cobra.Command{
		Use:   "llmdetect",
		Short: "Detect whether LLM API channels are running the original model",
	}
	root.PersistentFlags().StringVarP(&flagModel, "file", "f", "", "model YAML file (required)")
	root.PersistentFlags().StringVarP(&flagConfig, "config", "c", "./config.yaml", "config YAML file")

	root.AddCommand(cmdOnlineCheck(), cmdRefreshCache(), cmdDetect())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadBoth() (*config.Config, *config.ModelConfig) {
	if flagModel == "" {
		fmt.Fprintln(os.Stderr, "error: -f/--file is required")
		os.Exit(1)
	}
	cfg, err := config.LoadConfig(flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	model, err := config.LoadModel(flagModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading model: %v\n", err)
		os.Exit(1)
	}
	return cfg, model
}

func cachePath(modelFile string) string {
	ext := filepath.Ext(modelFile)
	return strings.TrimSuffix(modelFile, ext) + ".cache"
}

func cmdOnlineCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "online-check",
		Short: "Check whether the official API and all channels are reachable",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			all := append([]config.Endpoint{model.Official}, model.Channels...)
			newClient := func(ep config.Endpoint) *api.Client {
				return api.NewClient(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1)
			}
			results := online.CheckAll(cfg, model.Model, all, newClient)
			for _, r := range results {
				mark := "✓"
				if !r.Online {
					mark = "✗"
				}
				fmt.Printf("  %s  %-20s  %s\n", mark, r.Endpoint.Name, r.Endpoint.URL)
			}
		},
	}
}

func cmdRefreshCache() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh-cache",
		Short: "Discover border inputs from the official API and update the cache file",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			tokenList := tokens.Load()
			ctx := context.Background()
			timeout := time.Duration(cfg.Concurrency.TimeoutSeconds) * time.Second

			// Detect or load provider for the official API.
			var a provider.Adapter
			var err error
			if model.Official.Provider != "" {
				a, err = provider.AdapterFromType(provider.ProviderType(model.Official.Provider))
			} else {
				a, err = provider.Detect(ctx, model.Official.URL, model.Official.Key,
					model.Model, flagModel, model.Official.URL, timeout)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "detect official provider: %v\n", err)
				os.Exit(1)
			}
			if _, isAnthropic := a.(*provider.AnthropicAdapter); isAnthropic &&
				strings.HasPrefix(strings.ToLower(model.Model), "claude") {
				a = &provider.ClaudeCodeAdapter{}
			}

			officialClient := api.NewClientFull(model.Official.URL, model.Official.Key,
				cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries, a, nil)

			fmt.Println("Discovering border inputs from official API...")
			result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
			if err != nil {
				fmt.Fprintf(os.Stderr, "discovery failed: %v\n", err)
				os.Exit(1)
			}

			now := time.Now().UTC()
			cf := &cache.CacheFile{
				Model:        model.Model,
				OfficialURL:  model.Official.URL,
				CreatedAt:    now,
				ExpiresAt:    now.Add(time.Duration(cfg.Cache.TTLHours) * time.Hour),
				BorderInputs: result.BorderInputs,
			}
			c := cache.New(cachePath(flagModel))
			if err := c.Save(cf); err != nil {
				fmt.Fprintf(os.Stderr, "save cache: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Cache saved: %d border inputs (target: %d)\n",
				result.BorderInputsFound, cfg.Detection.BorderInputs)
			if result.Shortage {
				fmt.Printf("Warning: only %d BIs found (target: %d)\n",
					result.BorderInputsFound, cfg.Detection.BorderInputs)
			}
		},
	}
}

func cmdDetect() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Run online-check, load/refresh cache, probe all channels, and output a report",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, model := loadBoth()
			ctx := context.Background()
			startTime := time.Now()
			timeout := time.Duration(cfg.Concurrency.TimeoutSeconds) * time.Second
			ledger := api.NewTokenLedger()

			// Resolve adapter per endpoint (from YAML or via probe).
			// Endpoints that are undetectable are skipped.
			allEps := append([]config.Endpoint{model.Official}, model.Channels...)
			adapters := make(map[string]provider.Adapter, len(allEps))
			var adaptersMu sync.Mutex
			var adapterWg sync.WaitGroup
			for _, ep := range allEps {
				adapterWg.Add(1)
				go func(ep config.Endpoint) {
					defer adapterWg.Done()
					var a provider.Adapter
					var err error
					if ep.Provider != "" {
						a, err = provider.AdapterFromType(provider.ProviderType(ep.Provider))
					} else {
						a, err = provider.Detect(ctx, ep.URL, ep.Key, model.Model, flagModel, ep.URL, timeout)
					}
					if errors.Is(err, provider.ErrProviderUndetectable) {
						fmt.Fprintf(os.Stderr, "warning: provider undetectable for %s, treating as offline\n", ep.URL)
						return
					}
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: detect provider for %s: %v\n", ep.URL, err)
						return
					}
					adaptersMu.Lock()
					adapters[ep.URL] = a
					adaptersMu.Unlock()
				}(ep)
			}
			adapterWg.Wait()

			adapterFor := func(ep config.Endpoint) provider.Adapter {
				a, ok := adapters[ep.URL]
				if !ok {
					return &provider.OpenAIAdapter{}
				}
				// Auto-upgrade plain Anthropic adapter to ClaudeCode fingerprint
				// when the model name starts with "claude".
				if _, isAnthropic := a.(*provider.AnthropicAdapter); isAnthropic &&
					strings.HasPrefix(strings.ToLower(model.Model), "claude") {
					return &provider.ClaudeCodeAdapter{}
				}
				return a
			}

			// clientFor creates a Client with the detected adapter and the shared ledger.
			clientFor := func(ep config.Endpoint) *api.Client {
				return api.NewClientFull(ep.URL, ep.Key,
					cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries, adapterFor(ep), ledger)
			}

			// Uses detected adapters so Anthropic endpoints are checked correctly.
			onlineCheckFactory := func(ep config.Endpoint) *api.Client {
				return api.NewClientFull(ep.URL, ep.Key, cfg.Concurrency.TimeoutSeconds, 1, adapterFor(ep), nil)
			}
			onlineResults := online.CheckAll(cfg, model.Model, allEps, onlineCheckFactory)

			var onlineChannels []config.Endpoint
			officialURL := model.Official.URL
			officialOnline := false
			for _, r := range onlineResults {
				if r.Endpoint.URL == officialURL {
					officialOnline = r.Online
				} else if r.Online {
					onlineChannels = append(onlineChannels, r.Endpoint)
				}
			}

			c := cache.New(cachePath(flagModel))
			var cf *cache.CacheFile
			cacheStale := false
			cacheAgeMinutes := 0

			if c.IsExpired() {
				if !officialOnline {
					old, loadErr := c.Load()
					if loadErr != nil {
						fmt.Fprintf(os.Stderr, "official API offline and no stale cache available\n")
						os.Exit(1)
					}
					cf = old
					cacheStale = true
					cacheAgeMinutes = int(time.Since(old.CreatedAt).Minutes())
					fmt.Fprintf(os.Stderr, "Warning: official API offline, using stale cache (%d min old)\n", cacheAgeMinutes)
				} else {
					tokenList := tokens.Load()
					officialClient := clientFor(model.Official)
					result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
					if err != nil {
						old, loadErr := c.Load()
						if loadErr != nil {
							fmt.Fprintf(os.Stderr, "refresh failed and no stale cache: %v\n", err)
							os.Exit(1)
						}
						cf = old
						cacheStale = true
						cacheAgeMinutes = int(time.Since(old.CreatedAt).Minutes())
						fmt.Fprintf(os.Stderr, "Warning: refresh failed (%v), using stale cache (%d min old)\n", err, cacheAgeMinutes)
					} else {
						now := time.Now().UTC()
						cf = &cache.CacheFile{
							Model:        model.Model,
							OfficialURL:  model.Official.URL,
							CreatedAt:    now,
							ExpiresAt:    now.Add(time.Duration(cfg.Cache.TTLHours) * time.Hour),
							BorderInputs: result.BorderInputs,
						}
						if err := c.Save(cf); err != nil {
							fmt.Fprintf(os.Stderr, "save cache: %v\n", err)
						}
					}
				}
			} else {
				var err error
				cf, err = c.Load()
				if err != nil {
					fmt.Fprintf(os.Stderr, "load cache: %v\n", err)
					os.Exit(1)
				}
			}

			probeResults := detector.ProbeChannels(ctx, cfg, model, onlineChannels, cf.BorderInputs, clientFor)

			duration := time.Since(startTime).Seconds()

			channelOnlineResults := make([]online.Result, 0, len(onlineResults)-1)
			for _, r := range onlineResults {
				if r.Endpoint.URL != officialURL {
					channelOnlineResults = append(channelOnlineResults, r)
				}
			}

			params := report.ReportParams{
				Model:             model.Model,
				RunAt:             startTime,
				Duration:          duration,
				Cfg:               cfg,
				BorderInputsFound: len(cf.BorderInputs),
				Shortage:          len(cf.BorderInputs) < cfg.Detection.BorderInputs,
				CacheStale:        cacheStale,
				CacheAgeMinutes:   cacheAgeMinutes,
				OnlineResults:     channelOnlineResults,
				ProbeResults:      probeResults,
				Ledger:            ledger,
			}

			report.PrintSummary(params, cfg)
			outPath, err := report.WriteJSON(params, cfg.Output.ReportDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "write report: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Report written to: %s\n", outPath)
		},
	}
}
