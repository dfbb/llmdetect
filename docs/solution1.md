# Solution 1 — SSAI Gateway (claude.kg83.org) Persona Requirement

## 问题

我们的 `ClaudeCodeAdapter` 按照 `example/cc-gateway/sonnet.py` 的协议指纹构造请求
（billing header、CCH、Stainless headers、temperature=1、stream=true、cache_control
ephemeral 全部一致），但对 SSAI 网关 `claude.kg83.org/api/v1/messages` 发请求
始终得到 `HTTP 400 Internal server error`，而 `sonnet.py` 原版可以成功得到
`HTTP 200`。

## 根因（实测结论）

SSAI 网关除了验证协议指纹之外，还会对 **system 字段中非 billing 块的人设文本**
做内容校验。校验规则不是完整 exact-match，而是：

1. 合并后的 persona 文本里**必须包含字面子串**
   `"You are an assistant for performing a web search tool use"`（即 sonnet.py
   第 3 个 system 块 S3）。
2. 除 S3 之外还需要有**额外的人设前缀**（~50+ 字符的 "You are …" 风格），
   纯 S3 或仅 `"Anthropic's Claude " + S3` 都会被拒。
3. 人设文本可以**跨多个 system 块**或**拼进单一 block**，顺序无关；
   小改动（例如 `agent` → `agents`）可以通过，但替换掉 S3 核心串就会失败。

设：

```
S2 = "You are a Claude agent, built on Anthropic's Claude Agent SDK."
S3 = "You are an assistant for performing a web search tool use"
```

### 通过（HTTP 200）

| 变体 | 说明 |
|---|---|
| `billing` + `S2 + " " + S3` (单块) | sonnet.py 原版内容拼接 |
| `billing` + `S2` + `S3`（3 块） | sonnet.py 原版结构 |
| `billing` + `S3 + " " + S2`（倒序） | 顺序无关 |
| `billing` + `"prefix. " + S2 + " " + S3` | 前缀任意 |
| `billing` + `S2_typo + " " + S3` | S2 允许小改（`agent`→`agents`） |
| `billing` + `"You are Claude Code, Anthropic's official CLI for Claude. " + S3` | 项目自己的 CC persona + S3 |

### 失败（HTTP 400）

| 变体 | 说明 |
|---|---|
| `billing` + `S2` 单独 | 缺 S3 |
| `billing` + `S3` 单独 | 前缀不够 |
| `billing` + `S3 + 填充文本` | 前缀不够 |
| `billing` + `"Anthropic's Claude " + S3` | 前缀太短 |
| `billing` + 含 `Claude/Anthropic/SDK/web search` 关键词但缺 S3 字面串 | 不认关键词 |
| `billing` + S3 改写（`performinng` 拼错或 `you` 小写） | 字面串校验 |
| `billing` + `"You are X. You are Y."` 两句 You are 但不含 S3 | 不认结构 |

## 判断

这��� **SSAI 网关特定的缓存前缀校验**（它们把某个特定版本的 CC 客户端 prompt
做了 prompt caching，`cache_read_input_tokens: 2335` 与此吻合），**不是**
Anthropic 官方 API 的行为，也不是 Claude Code CLI 真实发出的结构（真实的 CC
请求不会包含 S3 这个 web-search 专属人设）。

所以核心矛盾：

- **对 SSAI 网关**：必须注入 `S3` 字面串，否则 400。
- **对真实 Anthropic API / 其它兼容网关**：不能注入 S3，因为那是 web-search
  专属 persona，会干扰通用对话语义。

## 解法（推荐 A）

### 方案 A — 将 SSAI 网关视为不兼容，不改适配器

理由：
- `ClaudeCodeAdapter` 的目标是伪造真实 Claude Code CLI 指纹以穿透
  anthropic-compatible 通道，而不是适配某一个私有网关的缓存校验。
- SSAI 要求的 S3 字面串是 web-search 专属 persona，硬编码到适配器里会污染
  其它正常请求的语义。
- 其它符合官方 Anthropic 行为的网关/直连 API 都能工作，不应为一个特殊网关牺牲
  通用性。

**实施**：
- 保留现状（billing + `"You are Claude Code, Anthropic's official CLI for Claude."`
  两块结构）。
- 把 `TestSonnetPyLive` 保留但注明只能对 SSAI 网关用 sonnet.py 原文跑，
  我们的适配器因不注入 web-search persona 而不通过此网关 —— 这是已知且有意
  的行为，不是 bug。
- 可以在 test 里加注释说明，或者直接删除 `TestSonnetPyLive`。

### 方案 B — 增加可选的 SSAI 兼容模式（备选）

只在需要真的打通 SSAI 网关时才开启：

```go
// ClaudeCodeAdapter 增加字段
type ClaudeCodeAdapter struct {
    // ... 现有字段
    ExtraSystemBlocks []string  // 附加人设块，用于特殊网关
}
```

调用侧：

```go
a := &ClaudeCodeAdapter{
    ExtraSystemBlocks: []string{
        "You are an assistant for performing a web search tool use",
    },
}
```

`BuildRequest` 把 `ExtraSystemBlocks` 里的每条加到 `System` 数组尾部
（都带 `cache_control: ephemeral`）。默认不传就是空切片，不影响现有行为。

这种方式：
- 不污染默认路径的语义。
- 让用户明确知道自己在适配一个要求 web-search persona 的特殊网关。
- 保留 sonnet.py 兼容性的同时不误导真实 Anthropic API 的调用。

### 方案 C — 始终注入 S3（不推荐）

每次都加 web-search persona。问题：
- 语义污染：模型被告知要"执行 web search tool use"，会把普通 chat 请求
  误解为 web search 触发。
- 对真实 Anthropic API 仍能通（它不做前缀校验），但响应质量会下降。
- 只是为了一个特殊网关的测试通过而牺牲通用行为。

## 推荐

采用 **方案 A**：不改适配器，`TestSonnetPyLive` 标注为 SSAI 专属（或删除），
文档里记录这一差异。如果将来真的需要走 SSAI 网关，再用 **方案 B** 加一个
`ExtraSystemBlocks` 字段作为 opt-in 入口。

## 补充发现（真实 CC 抓包对比）

拿到一份真实 Claude Code CLI 抓包请求（通过 SSAI 网关，HTTP 200 成功）。关键差异：

### 协议层差异

| 项 | 我们的适配器 | 真实 CC 抓包 |
|---|---|---|
| URL | `/v1/messages` | `/v1/messages?beta=true` |
| billing entrypoint | `cc_entrypoint=sdk-cli` | `cc_entrypoint=cli` |
| anthropic-beta | 4 个 flag | 8 个 flag（多 `claude-code-20250219`, `redact-thinking-2026-02-12`, `advanced-tool-use-2025-11-20`, `effort-2025-11-24`） |
| body 字段 | `temperature: 1`, 无 thinking | `thinking: {type: adaptive}`, `context_management`, `output_config: {effort: high}`, 无 `temperature` |
| accept-encoding | `identity` | `gzip, br` |
| 额外 headers | — | `x-claude-code-session-id`, `x-stainless-retry-count`, `x-stainless-timeout` |
| tools | — | 包含完整 tool schemas（Agent/Bash/Edit/…） |
| system blocks | billing + CC persona（2 块） | billing + CC persona + **~25880 字符详细指令**（3 块） |

### 实测结论

即便把以上所有差异全部对齐（用抓包里的 token、`cli` entrypoint、`?beta=true`、
完整 8-flag beta list、thinking adaptive、output_config、匹配长度的 filler 作为
system[2]、正确的 version hash），**网关仍返回 HTTP 400**。

**这决定性地表明**：SSAI 网关在校验 `system[2]` 的**字节级或 hash 级完全匹配**，
不是长度、不是结构、不是关键词。网关只接受**已经预缓存的特定 CC prompt**，
自己构造任何等长填充都无效（`cache_read_input_tokens: 2335` 印证了 prompt
caching 命中是成功的必要条件）。

### 对方案的影响

**强化方案 A 的合理性**：我们不可能在不获得真实 CC 25880 字符系统提示词字面量的
前提下通过 SSAI 网关，而那段提示词：
- 受版本更新影响，会变；
- 属于 Claude Code 的 proprietary prompt，直接内嵌有版权/维护风险；
- 即使嵌��了也只适配这一个网关。

**方案 B 的定位需要修订**：从"加 `ExtraSystemBlocks`"改为"加
`OverrideSystemBlocks []ccSystemBlock` 完全替换 system 数组"，让用户自己注入
从抓包或实机 CC 获取的完整 prompt。这相当于承认：穿透 SSAI 网关本质上需要
"借用"一份真实 CC 请求，适配器层无法生成。

## 方案 D — SSAI 专用离线抓包模式（新增备选）

如果确实要打通 SSAI：
1. 用户自己跑一次真实 Claude Code CLI 抓包（例如 mitmproxy）；
2. 把抓包中的 system 数组保存为 JSON 文件；
3. 适配器新增构造函数 `NewClaudeCodeAdapterWithSystem(path string)`，加载后
   每次 BuildRequest 都用该 system 数组（重算 cch，替换 billing header 的
   version hash）。

优点：不污染默认行为，诚实反映"靠 prompt caching 命中"这一事实。
缺点：用户负担高，提示词过期需要重新抓。仅在明确需要 SSAI 通道时才值得做。

## 参考数据

探测脚本和完整响应记录见会话日志。关键通过/失败对照：

```
# 通过
S2 + " " + S3                       → 200
S2 + S3 (无空格)                    → 200
S3 + " " + S2 (倒序)                → 200
"prefix. " + S2 + " " + S3          → 200
"You are Claude Code, ... " + S3    → 200

# 失败
S2 单独                             → 400
S3 单独                             → 400
S3 + 任意填充                       → 400
"Anthropic's Claude " + S3          → 400
仅含关键词但无 S3 字面串            → 400
"You are X. You are Y." 无 S3       → 400
```
