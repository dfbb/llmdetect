# llmdetect

[中文文档](README.zh.md)

**llmdetect** detects whether an LLM API channel is running the genuine model or a substitute, based on the [B3IT](https://arxiv.org/abs/2305.01320) statistical method.

It sends the same border-input prompts to both the official API and each channel under test, compares the output token distributions using Total Variation (TV) distance, and verdicts each channel as **original** or **spoofed**.

---

## How It Works

1. **Discover border inputs** — prompts where the target model produces variable outputs across repeated calls (temperature = 0). These are statistically sensitive probes.
2. **Build official distribution** — query the official API `queries_per_input` times per border input and record the token frequency distribution.
3. **Probe channels** — repeat the same queries against each channel and compute TV distance against the official distribution.
4. **Verdict** — channels with average TV ≥ `tv_threshold` are flagged as spoofed.

Border inputs and the official distribution are cached locally (`.cache` file) so subsequent runs do not re-query the official API unless the cache expires.

**Multi-provider support**: llmdetect automatically detects whether an endpoint speaks the OpenAI (`/v1/chat/completions`) or Anthropic (`/v1/messages`) API format, and persists the result to the model YAML so future runs skip the probe entirely.

---

## Requirements

- Go 1.22 or later
- API keys for the official model endpoint and all channels under test

---

## Installation

### Build from source

```bash
git clone git@github.com:dfbb/llmdetect.git
cd llmdetect
go build -o llmdetect ./cmd/llmdetect
```

Move the binary to a directory on your `PATH`:

```bash
mv llmdetect /usr/local/bin/
```

### Verify

```bash
llmdetect --help
```

---

## Configuration

### `config.yaml` — runtime parameters

```yaml
cache:
  ttl_hours: 1                   # how long the border-input cache is valid

detection:
  border_inputs: 20              # number of border inputs to collect
  discovery_candidates: 5000     # token candidates screened in Phase 1
  queries_per_input: 30          # queries per border input per channel
  tv_threshold: 0.4              # TV distance above this → spoofed

concurrency:
  max_workers_per_channel: 10    # parallel workers per channel
  rate_limit_rps: 5              # requests per second per channel
  timeout_seconds: 15            # per-request timeout
  max_retries: 3                 # retry attempts with exponential back-off

output:
  report_dir: "./reports"        # directory for JSON report files
```

### `models/gpt4o.yaml` — model under test

```yaml
model: gpt-4o

official:
  name: "OpenAI Official"
  url: "https://api.openai.com/v1"
  key: "sk-..."
  provider: openai               # optional; auto-detected on first run

channels:
  - name: "Channel A"
    url: "https://api.example-a.com/v1"
    key: "sk-..."
  - name: "Channel B (Anthropic relay)"
    url: "https://api.example-b.com/v1"
    key: "sk-ant-..."
    provider: anthropic          # set explicitly, or leave blank to auto-detect
```

**`provider` field** — valid values: `openai`, `anthropic`. Leave blank to let llmdetect probe the endpoint automatically on first run; the detected value is written back to the YAML so subsequent runs skip probing.

**OpenRouter** endpoints (`*.openrouter.ai`) are always treated as OpenAI-compatible without probing.

---

## Usage

All commands require `-f` to specify the model YAML file.

### Check reachability

```bash
llmdetect online-check -f models/gpt4o.yaml
```

Concurrently pings every endpoint and prints online / offline status:

```
Online Check
────────────────────────────────────────────────
  ✓  OpenAI Official     https://api.openai.com/v1
  ✓  Channel A           https://api.example-a.com/v1
  ✗  Channel B           https://api.example-b.com/v1
```

### Refresh border-input cache

```bash
llmdetect refresh-cache -f models/gpt4o.yaml
```

Forces re-discovery of border inputs from the official API and writes a fresh `.cache` file next to the model YAML. Run this manually when you want to reset the cache ahead of its TTL.

### Run full detection

```bash
llmdetect detect -f models/gpt4o.yaml
```

Runs the full pipeline: online-check → load/refresh cache → probe all online channels → output report.

```
llmdetect  model: gpt-4o   border inputs: 20   queries/input: 30   threshold: 0.40
run at: 2026-04-19T10:32:00+08:00   duration: 3.2s

Online Check
────────────────────────────────────────────────────────────────
  ✓  OpenAI Official     https://api.openai.com/v1
  ✓  Channel A           https://api.example-a.com/v1
  ✗  Channel B           https://api.example-b.com/v1   [offline, skipped]

Detection Results
────────────────────────────────────────────────────────────────
  Channel              TV Dist    Verdict
  ────────────────────────────────────────────────────────────
  Channel A            0.031      ✓ original
────────────────────────────────────────────────────────────────

Token Usage
────────────────────────────────────────────────────────────────
  URL                                              Prompt   Compl    Total
  ──────────────────────────────────────────────────────────────────────
  https://api.openai.com/v1                        12,450     620   13,070
  https://api.example-a.com/v1                      8,200     410    8,610
────────────────────────────────────────────────────────────────
  Total                                            20,650   1,030   21,680
────────────────────────────────────────────────────────────────

Report written to: ./reports/gpt-4o_2026-04-19T10-32-00.json
```

### Specify a custom config file

```bash
llmdetect detect -f models/gpt4o.yaml -c /etc/llmdetect/config.yaml
```

---

## JSON Report

Each `detect` run writes a timestamped JSON file to `output.report_dir`:

```json
{
  "model": "gpt-4o",
  "run_at": "2026-04-19T10:32:00Z",
  "duration_seconds": 3.2,
  "config": {
    "border_inputs": 20,
    "queries_per_input": 30,
    "tv_threshold": 0.4
  },
  "results": [
    {
      "name": "Channel A",
      "url": "https://api.example-a.com/v1",
      "online": true,
      "tv_distance": 0.031,
      "verdict": "original",
      "per_input_tv": [0.01, 0.02, 0.05],
      "tokens_used": { "prompt": 8200, "completion": 410, "total": 8610 }
    },
    {
      "name": "Channel B",
      "url": "https://api.example-b.com/v1",
      "online": false,
      "verdict": "offline"
    }
  ],
  "token_summary": {
    "https://api.openai.com/v1":       { "prompt": 12450, "completion": 620,  "total": 13070 },
    "https://api.example-a.com/v1":    { "prompt": 8200,  "completion": 410,  "total": 8610  }
  },
  "total_tokens": 21680
}
```

**Key fields:**
- `verdict`: `"original"` | `"spoofed"` | `"offline"`
- `per_input_tv`: TV distance for each individual border input
- `tokens_used`: token consumption for this channel during the detect run (omitted for offline channels)
- `token_summary`: per-URL totals including the official API (used during cache refresh)
- `cache_stale: true` + `cache_age_minutes: N` appear when the run fell back to an expired cache

---

## Cache File

The cache is stored alongside the model YAML as `{stem}.cache` (e.g., `models/gpt4o.cache`). It contains the border inputs and official token distributions. Do not commit this file — add it to `.gitignore`.

The cache expires after `cache.ttl_hours`. When expired, `detect` automatically refreshes it from the official API. If the official API is unreachable, `detect` falls back to the stale cache and notes this in the report.

---

## Project Structure

```
cmd/llmdetect/        CLI entry point (Cobra)
config/               Config and model YAML types + loader
internal/
  api/                HTTP client with adapter dispatch and token ledger
  cache/              Border-input cache (JSON + TTL)
  detector/           Phase 1 discovery and Phase 2 channel probing
  online/             Reachability check
  provider/           OpenAI / Anthropic adapter + auto-detection
  report/             Terminal and JSON report output
tokens/               Embedded token candidate list (~5 000 entries)
```

---

## License

[LICENSE](LICENSE)
