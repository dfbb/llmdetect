package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ironarmor/llmdetect/config"
	"github.com/ironarmor/llmdetect/internal/api"
	"github.com/ironarmor/llmdetect/internal/cache"
	"github.com/ironarmor/llmdetect/internal/detector"
	"github.com/ironarmor/llmdetect/internal/online"
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
			results := online.CheckAll(cfg, model.Model, all)
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

			fmt.Println("Discovering border inputs from official API...")
			officialClient := api.NewClient(model.Official.URL, model.Official.Key,
				cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
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

			// Step 1: online-check
			all := append([]config.Endpoint{model.Official}, model.Channels...)
			onlineResults := online.CheckAll(cfg, model.Model, all)
			var onlineChannels []config.Endpoint
			officialURL := model.Official.URL
			for _, r := range onlineResults {
				if r.Online && r.Endpoint.URL != officialURL {
					onlineChannels = append(onlineChannels, r.Endpoint)
				}
			}

			// Step 2: load or refresh cache
			c := cache.New(cachePath(flagModel))
			var cf *cache.CacheFile
			cacheStale := false
			cacheAgeMinutes := 0

			if c.IsExpired() {
				tokenList := tokens.Load()
				officialClient := api.NewClient(model.Official.URL, model.Official.Key,
					cfg.Concurrency.TimeoutSeconds, cfg.Concurrency.MaxRetries)
				result, err := detector.Discover(ctx, cfg, model, tokenList, officialClient)
				if err != nil {
					// stale cache fallback
					old, loadErr := c.Load()
					if loadErr != nil {
						fmt.Fprintf(os.Stderr, "refresh failed and no stale cache: %v\n", err)
						os.Exit(1)
					}
					cf = old
					cacheStale = true
					cacheAgeMinutes = int(time.Since(old.CreatedAt).Minutes())
					fmt.Fprintf(os.Stderr, "Warning: refresh failed (%v), using stale cache (%d min old)\n",
						err, cacheAgeMinutes)
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
						// continue with the in-memory result
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

			// Step 3: probe channels
			probeResults := detector.ProbeChannels(ctx, cfg, model, onlineChannels, cf.BorderInputs)

			duration := time.Since(startTime).Seconds()

			// Channels only (strip official from onlineResults display)
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
