# llmdetect 设计文档

**日期**: 2025-01-15  
**项目路径**: `llmdetect/`（新建目录，与本仓库同级）  
**语言**: Go  
**目标**: 基于 B3IT 论文原理，检测 LLM API 渠道是否在运行原厂模型（套壳检测）

---

## 一、项目结构

```
llmdetect/
├── cmd/llmdetect/
│   └── main.go                  # cobra root + 子命令注册
├── internal/
│   ├── api/
│   │   └── client.go            # OpenAI 兼容 HTTP 客户端（chat completions）
│   ├── cache/
│   │   └── cache.go             # 边界输入缓存（JSON + mtime TTL）
│   ├── detector/
│   │   ├── discover.go          # Phase 1：从官方 API 发现边界输入
│   │   ├── probe.go             # Phase 2：并发探测各渠道
│   │   └── tvdistance.go        # TV 距离计算 + 判定逻辑
│   ├── online/
│   │   └── checker.go           # online-check：最小请求验证 API 可达性
│   └── report/
│       ├── terminal.go          # 彩色表格终端输出
│       └── json.go              # JSON 报告写文件
├── config/
│   ├── loader.go                # 加载 config.yaml 和 model YAML
│   └── types.go                 # Config / ModelFile 结构体定义
├── tokens/
│   └── fallback.txt             # 内嵌 token 候选词表（移植自 Python b3it_monitoring，~5000 条，go:embed）
├── config.yaml                  # 运行参数
└── models/
    ├── gpt4o.yaml               # 测试目标示例
    └── gpt4o.cache              # 自动生成的缓存文件
```

---

## 二、CLI 命令

```
llmdetect online-check -f models/gpt4o.yaml
llmdetect refresh-cache -f models/gpt4o.yaml
llmdetect detect -f models/gpt4o.yaml
```

| 命令 | 说明 |
|---|---|
| `online-check` | 并发检查官方 API 与所有渠道是否可达，输出在线状态 |
| `refresh-cache` | 强制从官方 API 重新发现边界输入，更新 `.cache` 文件 |
| `detect` | 自动先执行 online-check → 加载/刷新缓存 → 并发探测 → 输出报告 |

**通用 flag：**
- `-f, --file`：指定 model YAML 文件路径（必填）
- `-c, --config`：指定 config.yaml 路径（默认 `./config.yaml`）

---

## 三、配置文件格式

### `config.yaml`（运行参数）

```yaml
cache:
  ttl_hours: 1                    # 缓存有效期（小时）

detection:
  border_inputs: 20               # 目标边界输入数量
  discovery_candidates: 5000      # Phase 1 最多探测的 token 候选数
  discovery_queries_per_candidate: 3  # Phase 1 每候选 token 查询次数（≥2 种输出即为边界输入）
  queries_per_input: 30           # 每个边界输入查询次数
  tv_threshold: 0.4               # TV 距离判定阈值（≥此值为套壳）

concurrency:
  max_workers_per_channel: 10     # 每渠道独立并发槽（非全局）
  rate_limit_rps: 5               # 每渠道每秒请求数限制（token bucket）
  timeout_seconds: 15             # 单次请求超时
  max_retries: 3                  # 失败重试次数（指数退避）

output:
  report_dir: "./reports"         # JSON 报告输出目录
```

### `models/gpt4o.yaml`（测试目标）

```yaml
model: gpt-4o

official:
  name: "OpenAI 官方"
  url: "https://api.openai.com/v1"
  key: "sk-..."

channels:
  - name: "渠道A-某云"
    url: "https://api.xxx.com/v1"
    key: "sk-..."
  - name: "渠道B-某转发"
    url: "https://api.yyy.com/v1"
    key: "sk-..."
```

---

## 四、缓存文件格式

**路径**：与 model YAML 同目录，文件名为 `{stem}.cache`（如 `models/gpt4o.cache`）

```json
{
  "model": "gpt-4o",
  "official_url": "https://api.openai.com/v1",
  "created_at": "2025-01-15T10:30:00Z",
  "expires_at": "2025-01-15T11:30:00Z",
  "border_inputs": [
    {
      "prompt": "hello",
      "official_distribution": {
        "world": 15,
        "there": 8,
        "Hi": 7
      }
    },
    {
      "prompt": "∂",
      "official_distribution": {
        "x": 20,
        "y": 10
      }
    }
  ]
}
```

**字段说明：**
- `official_distribution`：官方 API 对该 prompt 的响应 token 计数（`queries_per_input` 次）
- `detect` 直接复用官方分布，无需重新查询官方 API
- `expires_at` 到期后，`detect` 自动触发 `refresh-cache`

**缓存异常处理：**
- 若 `.cache` 文件存在但 JSON 格式损坏，则删除该文件并返回 `ErrCorrupted`，触发重新执行 `refresh-cache`，而非 panic

---

## 五、核心算法

### online-check

对每个端点（官方 + 所有渠道）并发发送最小请求：

```
POST /v1/chat/completions
{"model":"...","messages":[{"role":"user","content":"hi"}],"max_tokens":1}
```

- HTTP 200 → 在线 ✓
- 超时 / 非 200 → 离线 ✗
- 超时时间由 `concurrency.timeout_seconds` 控制

### refresh-cache（Phase 1）

1. 从内嵌词表（`tokens/fallback.txt`）随机抽取最多 `discovery_candidates` 个单 token 候选
2. **并发**查询官方 API（`max_workers_per_channel` 槽 + `rate_limit_rps` 限速），每个 token 查询 `discovery_queries_per_candidate` 次，`temperature=0`，`max_tokens=1`
3. 找出有 ≥ 2 种不同输出的 token（即边界输入）
4. 达到 `border_inputs` 目标数量后提前停止
5. 对每个边界输入再并发查询 `queries_per_input` 次，建立官方响应分布
6. 写入 `.cache` 文件

**降级策略**：若 refresh 失败（官方 API 不可用），且旧缓存文件存在，则复用旧缓存继续检测，并在报告末尾标注 `"cache_stale": true, "cache_age_minutes": N`。

**致命错误**：若 refresh 失败且**无任何旧缓存可用**（文件不存在或损坏），则以非零退出码终止（`os.Exit(1)`），并打印错误信息，不生成报告。

### detect（Phase 2）

1. 执行 online-check，过滤离线渠道
2. 检查缓存有效性，过期则执行 refresh-cache
3. 对每个在线渠道按**域名分组**探测：
   - 提取每个渠道 URL 的根域名（eTLD+1，如 `api.xxx.com` → `xxx.com`）
   - **不同域名的渠道并行执行**
   - **相同域名的渠道串行执行**（避免同一提供商多渠道并发）
   - 每渠道内部：`max_workers_per_channel` 槽 + `rate_limit_rps` 限速
   - 每个 border_input 查询 `queries_per_input` 次，`temperature=0`，`max_tokens=1`
4. 计算每个渠道的平均 TV 距离：

```
TV(P, Q) = 0.5 × Σ |P(x) - Q(x)|   （对所有输出 token x 求和）
avg_TV   = mean(TV per border_input)
```

5. 判定：
   - `avg_TV < tv_threshold` → 原厂 ✓
   - `avg_TV ≥ tv_threshold` → 套壳 ✗
6. 输出报告

---

## 六、输出格式

### 终端（online-check）

```
在线检查
────────────────────────────────────────────────
  ✓  OpenAI 官方       https://api.openai.com/v1
  ✓  渠道A-某云        https://api.xxx.com/v1
  ✗  渠道B-某转发      https://api.yyy.com/v1
  ✓  渠道C-某中转      https://api.zzz.com/v1
```

格式：`[标记] [名称] [URL]`，标记 `✓` = 在线，`✗` = 离线。

### 终端（detect）

```
模型: gpt-4o   边界输入: 20个   查询: 30次/输入   阈值: 0.40
运行时间: 2025-01-15 10:32:00   耗时: 3m12s

在线检查
────────────────────────────────────────────────
  ✓  OpenAI 官方       https://api.openai.com/v1
  ✓  渠道A-某云        https://api.xxx.com/v1
  ✗  渠道B-某转发      https://api.yyy.com/v1   [离线，跳过]
  ✓  渠道C-某中转      https://api.zzz.com/v1

检测结果
────────────────────────────────────────────────────────────────
  渠道名称          TV 距离    判定
  ──────────────────────────────────────────────────────────────
  渠道A-某云        0.03      ✓ 原厂
  渠道C-某中转      0.61      ✗ 套壳
────────────────────────────────────────────────────────────────
报告已写入: ./reports/gpt4o_2025-01-15T10-32-00.json
```

### JSON 报告

```json
{
  "model": "gpt-4o",
  "run_at": "2025-01-15T10:32:00Z",
  "duration_seconds": 192,
  "config": {
    "border_inputs": 20,
    "queries_per_input": 30,
    "tv_threshold": 0.40
  },
  "results": [
    {
      "name": "渠道A-某云",
      "url": "https://api.xxx.com/v1",
      "online": true,
      "tv_distance": 0.03,
      "verdict": "original",
      "per_input_tv": [0.01, 0.02, 0.05]
    },
    {
      "name": "渠道B-某转发",
      "url": "https://api.yyy.com/v1",
      "online": false,
      "tv_distance": null,
      "verdict": "offline"
    },
    {
      "name": "渠道C-某中转",
      "url": "https://api.zzz.com/v1",
      "online": true,
      "tv_distance": 0.61,
      "verdict": "spoofed"
    }
  ],
  "cache_stale": false
}
```

**JSON 字段说明：**
- `cache_stale`：正常情况下为 `false`（省略或明确输出均可）；若使用了过期旧缓存则为 `true`，同时附带 `cache_age_minutes` 字段
- `cache_age_minutes`：仅在 `cache_stale: true` 时出现，表示旧缓存已过期多少分钟

---

## 七、依赖

**最低 Go 版本：1.22**（使用 `go:embed`、range-over-integers 等特性）

| 库 | 用途 |
|---|---|
| `github.com/spf13/cobra` | CLI 框架 |
| `gopkg.in/yaml.v3` | YAML 解析 |
| `github.com/fatih/color` | 终端彩色输出 |
| 标准库 `net/http` | HTTP 客户端 |
| 标准库 `embed` | 内嵌 token 词表 |
| 标准库 `sync` | WaitGroup / Semaphore 并发控制 |
| `golang.org/x/time/rate` | 每渠道 token bucket 限速（Rate Limiter）|
| `golang.org/x/net/publicsuffix` | eTLD+1 提取，用于渠道按根域名分组并发控制 |
