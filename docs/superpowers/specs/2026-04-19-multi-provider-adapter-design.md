# Multi-Provider Adapter + Token Accounting 设计文档

**日期**: 2026-04-19
**关联主设计**: `docs/superpowers/specs/2025-01-15-llmdetect-design.md`
**语言**: Go
**目标**: 为 llmdetect 增加两项能力：
1. 自动探测并适配多种 LLM API 格式（Anthropic、OpenAI、OpenRouter 等），探测结果持久化到 model YAML，后续运行直接复用。
2. 每次 detect 结束后，统计并展示每个 URL 的 token 消耗（终端表格 + JSON 报告）。

---

## 一、新增 / 修改文件

### 新增

```
internal/provider/
├── adapter.go       # Adapter interface + ProviderType 枚举 + TokenUsage 类型
├── detector.go      # 主动探测逻辑（probe → 写回 YAML）
├── openai.go        # OpenAI / OpenRouter 适配器
└── anthropic.go     # Anthropic Messages API 适配器
```

### 修改

| 文件 | 改动内容 |
|---|---|
| `internal/api/client.go` | 持有 `provider.Adapter`；每次请求后向 `TokenLedger` 累加 usage |
| `config/types.go` | `Endpoint` 结构体新增 `Provider string` 字段（yaml tag: `provider,omitempty`）|
| `internal/report/terminal.go` | detect 结束后追加 token 汇总表格 |
| `internal/report/json.go` | `ChannelResult` 新增 `TokensUsed`；顶层新增 `TokenSummary`、`TotalTokens` |

---

## 二、Adapter 接口

```go
// internal/provider/adapter.go

type ProviderType string

const (
    ProviderOpenAI    ProviderType = "openai"
    ProviderAnthropic ProviderType = "anthropic"
)

type TokenUsage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

type Adapter interface {
    Type() ProviderType
    // BuildRequest 构造请求体（JSON），temperature 固定为 0，max_tokens=1
    BuildRequest(model, prompt string, maxTokens int) ([]byte, error)
    // RequestPath 返回请求路径，如 "/v1/chat/completions" 或 "/v1/messages"
    RequestPath() string
    // AuthHeader 返回鉴权 header 名和值，如 ("Authorization", "Bearer sk-...") 或 ("x-api-key", "sk-...")
    AuthHeader(apiKey string) (name, value string)
    // ParseResponse 从响应体中提取输出 token 和 usage
    ParseResponse(body []byte) (outputToken string, usage TokenUsage, err error)
}
```

**OpenAI 适配器**（`openai.go`）：
- `RequestPath()` → `/v1/chat/completions`
- `AuthHeader()` → `Authorization: Bearer <key>`
- `ParseResponse()` → 读 `choices[0].message.content` + `usage.prompt_tokens` / `completion_tokens`
- OpenRouter URL 含 `openrouter.ai` 时直接使用本适配器（格式完全兼容）

**Anthropic 适配器**（`anthropic.go`）：
- `RequestPath()` → `/v1/messages`
- `AuthHeader()` → `x-api-key: <key>`；额外附加 `anthropic-version: 2023-06-01`
- `ParseResponse()` → 读 `content[0].text` + `usage.input_tokens` / `output_tokens`

---

## 三、探测与持久化逻辑

### 函数签名

```go
// internal/provider/detector.go

// Detect 返回该 endpoint 的适配器。
// 若 model YAML 中已有 provider 字段则直接构造，不发请求。
// 若无则依次尝试 OpenAI → Anthropic 格式，成功后回写 YAML。
func Detect(baseURL, apiKey, model string, yamlPath string, endpointKey string) (Adapter, error)
```

### 探测流程

```
读取 YAML endpoint.provider 字段
        ↓ 已有
直接返回对应 Adapter（不发请求）
        ↓ 为空
尝试 OpenAI 格式（POST /v1/chat/completions，max_tokens=1）
        ↓ HTTP 200
回写 provider: openai 到 YAML → 返回 OpenAIAdapter
        ↓ 非 200
尝试 Anthropic 格式（POST /v1/messages，max_tokens=1）
        ↓ HTTP 200
回写 provider: anthropic 到 YAML → 返回 AnthropicAdapter
        ↓ 非 200
返回 ErrProviderUndetectable（该 endpoint 标记为不可用）
```

### YAML 回写规则

- 使用 `gopkg.in/yaml.v3` 的 node-level API（`yaml.Node`）原地修改目标 endpoint 的 `provider` 字段，保留其余字段、顺序和注释。
- 回写操作加文件锁（`sync.Mutex`），防止并发写冲突。
- 探测结果**仅内存缓存至当次运行**，不额外维护缓存文件；持久化依赖 YAML 回写。

### 特殊规则

- URL 含 `openrouter.ai` → 直接返回 `OpenAIAdapter`，**跳过探测**，不回写（provider 已隐含）。
- 用户可手动在 YAML 中填写 `provider` 字段强制覆盖，工具不会覆盖已有值。

---

## 四、TokenLedger

```go
// internal/api/client.go（扩展部分）

type TokenLedger struct {
    mu    sync.Mutex
    usage map[string]*provider.TokenUsage  // key: endpoint 完整 base URL
}

func NewTokenLedger() *TokenLedger
func (l *TokenLedger) Add(url string, u provider.TokenUsage)
func (l *TokenLedger) Snapshot() map[string]provider.TokenUsage  // 返回只读副本
func (l *TokenLedger) Total() provider.TokenUsage                // 所有 URL 合计
```

- `detect` 命令在入口处创建一个 `TokenLedger`，注入到 `client`，运行结束后传给 report。
- 独立执行 `online-check` 和 `refresh-cache` 命令时**不启用** TokenLedger，不统计 token。
- `detect` 命令内部触发的 `refresh-cache`（缓存过期时）**会启用** TokenLedger，官方 API 的开销也计入 `token_summary`。
- provider 探测请求（`Detect()` 发出的单次最小请求）**不计入** TokenLedger。

---

## 五、输出格式变更

### 终端（detect 结束后追加）

```
Token 消耗统计
────────────────────────────────────────────────────────────────
  URL                              提示词     补全    合计
  ──────────────────────────────────────────────────────────────
  https://api.openai.com/v1        12,450      620   13,070
  https://api.xxx.com/v1            8,200      410    8,610
  https://api.zzz.com/v1            8,200      390    8,590
────────────────────────────────────────────────────────────────
  合计                             28,850    1,420   30,270
```

- 离线渠道（verdict = offline）不出现在此表格中。
- 数字使用千分位格式（`12,450`）。

### JSON 报告新增字段

```json
{
  "results": [
    {
      "name": "渠道A-某云",
      "url": "https://api.xxx.com/v1",
      "online": true,
      "tv_distance": 0.03,
      "verdict": "original",
      "per_input_tv": [0.01, 0.02, 0.05],
      "tokens_used": {
        "prompt": 8200,
        "completion": 410,
        "total": 8610
      }
    },
    {
      "name": "渠道B-某转发",
      "url": "https://api.yyy.com/v1",
      "online": false,
      "verdict": "offline"
    }
  ],
  "token_summary": {
    "https://api.openai.com/v1": { "prompt": 12450, "completion": 620,  "total": 13070 },
    "https://api.xxx.com/v1":    { "prompt": 8200,  "completion": 410,  "total": 8610  },
    "https://api.zzz.com/v1":    { "prompt": 8200,  "completion": 390,  "total": 8590  }
  },
  "total_tokens": 30270
}
```

- 离线渠道的 `tokens_used` 字段省略（`omitempty`）。
- `token_summary` 包含官方 API URL（用于 refresh-cache 阶段的开销）以及所有在线渠道 URL。

---

## 六、错误处理

| 场景 | 处理方式 |
|---|---|
| 探测两种格式均失败 | 返回 `ErrProviderUndetectable`，该 endpoint 视同离线，在报告中标记 `verdict: "undetectable"` |
| YAML 回写失败（权限等） | 打印警告，继续运行；下次运行仍会重新探测 |
| `usage` 字段缺失（部分代理不透传） | `TokenUsage` 保持零值，终端表格显示 `—`，JSON 字段设为 `null` |

---

## 七、model YAML 格式变更

新增可选 `provider` 字段，每个 endpoint 独立：

```yaml
model: gpt-4o

official:
  name: "OpenAI 官方"
  url: "https://api.openai.com/v1"
  key: "sk-..."
  provider: openai        # 首次运行后自动写入；可手动填写强制覆盖

channels:
  - name: "渠道A-某云"
    url: "https://api.xxx.com/v1"
    key: "sk-..."
    provider: openai
  - name: "渠道B-Anthropic转发"
    url: "https://api.yyy.com/v1"
    key: "sk-..."
    provider: anthropic
```

`provider` 合法值：`openai`、`anthropic`。其余值在加载时返回配置错误。

---

## 八、新增依赖

无新增第三方依赖。YAML node-level 操作使用已有的 `gopkg.in/yaml.v3`。
