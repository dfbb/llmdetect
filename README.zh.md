# llmdetect

[English Documentation](README.md)

**llmdetect** 基于 [B3IT](https://arxiv.org/abs/2305.01320) 统计方法，检测 LLM API 渠道是否在运行原厂模型（套壳检测）。

它向官方 API 和待测渠道发送相同的边界输入提示词，用总变差（TV）距离比较输出 token 分布，对每个渠道给出 **原厂** 或 **套壳** 的判定。

---

## 工作原理

1. **发现边界输入** — 找出在 `temperature=0` 下仍会产生多种不同输出的提示词，这些是统计上最敏感的探针。
2. **构建官方分布** — 对每个边界输入向官方 API 查询 `queries_per_input` 次，记录 token 频率分布。
3. **探测渠道** — 对每个待测渠道重复相同查询，计算与官方分布的 TV 距离。
4. **判定** — 平均 TV 距离 ≥ `tv_threshold` 的渠道标记为套壳。

边界输入和官方分布缓存在本地（`.cache` 文件），后续运行无需重新查询官方 API，直到缓存过期。

**多提供商支持**：llmdetect 自动探测端点使用的是 OpenAI（`/v1/chat/completions`）还是 Anthropic（`/v1/messages`）API 格式，并将结果写回 model YAML，后续运行直接复用，无需再次探测。

---

## 环境要求

- Go 1.22 及以上版本
- 官方模型端点及所有待测渠道的 API Key

---

## 安装

### 从源码构建

```bash
git clone git@github.com:dfbb/llmdetect.git
cd llmdetect
go build -o llmdetect ./cmd/llmdetect
```

将二进制文件移动到 `PATH` 中的目录：

```bash
mv llmdetect /usr/local/bin/
```

### 验证安装

```bash
llmdetect --help
```

---

## 配置

### `config.yaml` — 运行参数

```yaml
cache:
  ttl_hours: 1                   # 缓存有效期（小时）

detection:
  border_inputs: 20              # 目标边界输入数量
  discovery_candidates: 5000     # Phase 1 最多筛查的 token 候选数
  queries_per_input: 30          # 每个边界输入每渠道查询次数
  tv_threshold: 0.4              # TV 距离判定阈值（≥此值判定为套壳）

concurrency:
  max_workers_per_channel: 10    # 每渠道并发工作槽数
  rate_limit_rps: 5              # 每渠道每秒请求数限制
  timeout_seconds: 15            # 单次请求超时时间
  max_retries: 3                 # 失败重试次数（指数退避）

output:
  report_dir: "./reports"        # JSON 报告输出目录
```

### `models/gpt4o.yaml` — 测试目标

```yaml
model: gpt-4o

official:
  name: "OpenAI 官方"
  url: "https://api.openai.com/v1"
  key: "sk-..."
  provider: openai               # 可选；首次运行后自动写入

channels:
  - name: "渠道A-某云"
    url: "https://api.example-a.com/v1"
    key: "sk-..."
  - name: "渠道B-Anthropic转发"
    url: "https://api.example-b.com/v1"
    key: "sk-ant-..."
    provider: anthropic          # 可手动填写，或留空由工具自动探测
```

**`provider` 字段** — 合法值：`openai`、`anthropic`。留空时，llmdetect 在首次运行时自动探测并将结果写回 YAML，后续运行直接复用。

**OpenRouter** 端点（`*.openrouter.ai`）始终视为 OpenAI 兼容格式，不发起探测请求。

---

## 使用

所有命令均需通过 `-f` 指定 model YAML 文件。

### 检查可达性

```bash
llmdetect online-check -f models/gpt4o.yaml
```

并发 ping 所有端点，输出在线/离线状态：

```
Online Check
────────────────────────────────────────────────
  ✓  OpenAI 官方         https://api.openai.com/v1
  ✓  渠道A-某云          https://api.example-a.com/v1
  ✗  渠道B-某转发        https://api.example-b.com/v1
```

### 刷新边界输入缓存

```bash
llmdetect refresh-cache -f models/gpt4o.yaml
```

强制从官方 API 重新发现边界输入，在 model YAML 同目录下写入新的 `.cache` 文件。可在缓存到期前手动执行以提前重置。

### 运行完整检测

```bash
llmdetect detect -f models/gpt4o.yaml
```

执行完整流程：在线检查 → 加载/刷新缓存 → 探测所有在线渠道 → 输出报告。

```
llmdetect  model: gpt-4o   border inputs: 20   queries/input: 30   threshold: 0.40
run at: 2026-04-19T10:32:00+08:00   duration: 3.2s

Online Check
────────────────────────────────────────────────────────────────
  ✓  OpenAI 官方         https://api.openai.com/v1
  ✓  渠道A-某云          https://api.example-a.com/v1
  ✗  渠道B-某转发        https://api.example-b.com/v1   [offline, skipped]

Detection Results
────────────────────────────────────────────────────────────────
  Channel              TV Dist    Verdict
  ────────────────────────────────────────────────────────────
  渠道A-某云           0.031      ✓ original
────────────────────────────────────────────────────────────────

Token Usage
────────────────────────────────────────────────────────────────
  URL                                              Prompt   Compl    Total
  ──────────────────────────────────────────────────────────────────────
  https://api.openai.com/v1                        12,450     620   13,070
  https://api.example-a.com/v1                      8,200     410    8,610
────────────────────────────────────────────────────────────────
  Total                                            20,650   1,030   21,680
─────────────────────────────────────────��──────────────────────

Report written to: ./reports/gpt-4o_2026-04-19T10-32-00.json
```

### 指定自定义配置文件

```bash
llmdetect detect -f models/gpt4o.yaml -c /etc/llmdetect/config.yaml
```

---

## JSON 报告

每次 `detect` 运行都会在 `output.report_dir` 目录下写入一个带时间戳的 JSON 文件：

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
      "name": "渠道A-某云",
      "url": "https://api.example-a.com/v1",
      "online": true,
      "tv_distance": 0.031,
      "verdict": "original",
      "per_input_tv": [0.01, 0.02, 0.05],
      "tokens_used": { "prompt": 8200, "completion": 410, "total": 8610 }
    },
    {
      "name": "渠道B-某转发",
      "url": "https://api.example-b.com/v1",
      "online": false,
      "verdict": "offline"
    }
  ],
  "token_summary": {
    "https://api.openai.com/v1":      { "prompt": 12450, "completion": 620,  "total": 13070 },
    "https://api.example-a.com/v1":   { "prompt": 8200,  "completion": 410,  "total": 8610  }
  },
  "total_tokens": 21680
}
```

**主要字段说明：**
- `verdict`：`"original"`（原厂）| `"spoofed"`（套壳）| `"offline"`（离线）
- `per_input_tv`：每个边界输入的 TV 距离
- `tokens_used`：本次 detect 该渠道消耗的 token 数（离线渠道省略此字段）
- `token_summary`：各 URL 的 token 汇总，包含缓存刷新阶段的官方 API 消耗
- `cache_stale: true` + `cache_age_minutes: N`：回退到过期缓存时出现

---

## 缓存文件

缓存存储在 model YAML 同目录下，文件名为 `{stem}.cache`（如 `models/gpt4o.cache`），包含边界输入和官方 token 分布。建议将其加入 `.gitignore`，不要提交到版本库。

缓存在 `cache.ttl_hours` 小时后过期。过期时，`detect` 自动从官方 API 刷新。若官方 API 不可达，则回退到旧缓存并在报告中注明。

---

## 项目结构

```
cmd/llmdetect/        CLI 入口（Cobra）
config/               配置和 model YAML 类型定义 + 加载器
internal/
  api/                带适配器分发和 token 统计的 HTTP 客户端
  cache/              边界输入缓存（JSON + TTL）
  detector/           Phase 1 发现和 Phase 2 渠道探测
  online/             可达性检查
  provider/           OpenAI / Anthropic 适配器 + 自动探测
  report/             终端和 JSON 报告输出
tokens/               内嵌 token 候选词表（约 5000 条）
```

---

## 许可证

[LICENSE](LICENSE)
