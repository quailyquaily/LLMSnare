# LLM Snare 初始需求

## 项目名称

LLM Snare

## 目标

这个项目用于测试一个 LLM 在改代码前，是否真的会先读代码库，而不是凭印象直接写。

核心办法是提供一个带“陷阱”的 mock 文件系统：

- 一个很简单但明确要求复用的工具函数，专门用来检测模型是否偷懒重写逻辑。
- 一个在提示词里故意写错文件名的 vendor 文件，专门用来检测模型是否会先列目录、再确认真实文件。

这里的“文件系统”是测试夹具的一部分，不是真实磁盘目录。

- 文档中列出的源码文件都是 mock 数据。
- `list_dir`、`read_file`、`write_file` 都是 mock 工具。
- benchmark runner 只需要在内存中维护这套虚拟文件系统和工具调用日志，不需要直接操作真实文件系统。

---

## Benchmark 主题

LLM Context Fidelity Benchmark

它要验证的不是模型能不能把功能写出来，而是模型有没有真正遵守下面这些基本工程动作：

- 先读相关文件，再写文件。
- 按代码库现有实现复用 helper，而不是私自重写。
- 遇到提示词中的路径或文件名不准时，会自己检查目录并修正。
- 根据代码库中已有约定输出结果，而不是拍脑袋造格式。

---

## 宿主程序形态

`llmsnare` 本身是一个用 Go 实现的真实程序。

这里要分清两层：

- benchmark 内容层：虚拟文件系统、mock 源码、mock 工具调用
- 宿主程序层：真实 CLI、真实配置文件、真实 HTTP API

也就是说，模型看到的 `main.go`、`utils/utils.go`、`vendor/...` 都来自 `rootfs/` 里的 benchmark 数据；但 `llmsnare` 这个程序本身需要在真实环境中运行。

---

## 运行方式

系统需要支持两种运行方式。

### 1. daemon 模式

命令：

```bash
llmsnare serve
```

行为：

- 读取 timeline 存储目录
- 提供一个 HTTP API，用来返回各个 LLM profile 的得分 timeline

`serve` 不负责调度 benchmark。定时执行由 Linux 系统 `cron` 调用 `llmsnare run --persist` 完成。

持久化要求：

- 使用 JSONL 格式
- 保存到一个持久化目录下
- 目录路径可由 `config.yaml` 配置
- 如果配置中未显式指定，就使用默认约定目录
- 每次 benchmark 运行结果追加写入对应 profile 的 timeline 文件

建议文件布局：

```text
<timeline_dir>/<profile_name>.jsonl
```

建议默认目录：

```text
~/.local/state/llmsnare/timeline
```

每一行表示一次运行结果，至少包含：

- 运行时间
- profile 名称
- 总分
- 关键指标
- 必要的扣分项和加分项摘要

这里的 timeline 至少要包含：

- profile 名称
- 每次运行时间
- 每次运行总分
- 必要时附带关键指标，例如 `read:write ratio`、`pre-write read coverage`

### 2. 一次性运行模式

命令：

```bash
llmsnare run <profile_name>
```

行为：

- 执行一次 benchmark
- 直接输出本次得分结果

参数规则：

- 如果指定了 `<profile_name>`，只运行该 profile
- 如果没有指定 profile_name，就运行配置中的全部 profile

---

## 初始化命令

命令：

```bash
llmsnare init
```

行为：

- 在默认配置路径生成一个模板配置文件
- 如果目标文件已存在，默认拒绝覆盖并报错
- 只有显式传入 `--force` 时才允许覆盖已有文件

默认配置路径：

```text
~/.config/llmsnare/config.yaml
```

---

## 配置文件

配置文件放在：

```text
~/.config/llmsnare/config.yaml
```

它至少需要能描述下面这些信息：

- timeline 持久化目录
- 一组 LLM profile
- 每个 profile 的模型名称
- 每个 profile 的 provider
- 每个 profile 的 API endpoint 覆盖配置
- 每个 profile 的 API key

这里把 v1 配置结构定下来，不再停留在“建议”层面。

### v1 配置结构

```yaml
version: 1

serve:
  listen: "127.0.0.1:8787"

storage:
  timeline_dir: "~/.local/state/llmsnare/timeline"

profiles:
  openai_gpt4o:
    provider: openai
    model: "gpt-4o"
    api_key: "${OPENAI_API_KEY}"
    timeout: 300s
    temperature: 0
    max_output_tokens: 4096

  claude_sonnet:
    provider: anthropic
    model: "claude-3-5-sonnet-latest"
    api_key: "${ANTHROPIC_API_KEY}"
    timeout: 300s
    temperature: 0
    max_output_tokens: 4096

  cf_llama:
    provider: cloudflare
    model: "@cf/meta/llama-3.1-8b-instruct"
    account_id: "${CLOUDFLARE_ACCOUNT_ID}"
    api_token: "${CLOUDFLARE_API_TOKEN}"
    timeout: 300s
    temperature: 0
    max_output_tokens: 4096
```

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `version` | integer | 是 | 配置版本。v1 固定写 `1`。 |
| `serve` | object | 是 | `llmsnare serve` 的监听配置。 |
| `storage` | object | 是 | 持久化配置。当前主要是 timeline 目录。 |
| `profiles` | map[string]object | 是 | LLM profile 集合，key 就是 profile 名称。 |

### `serve` 字段

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `serve.listen` | string | 否 | `127.0.0.1:8787` | HTTP API 监听地址。 |

### `storage` 字段

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `storage.timeline_dir` | string | 否 | `~/.local/state/llmsnare/timeline` | timeline JSONL 文件保存目录。 |

### `profiles` 字段

`profiles` 是一个 map。key 是 profile 名称，例如 `openai_gpt4o`、`claude_sonnet`。

每个 profile 结构如下：

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `provider` | string | 是 | 无 | 模型供应商。v1 支持 `openai`、`anthropic`、`gemini`、`cloudflare`。 |
| `model` | string | 是 | 无 | 目标模型名。 |
| `endpoint` | string | 否 | provider-specific default | API 基地址覆盖值。 |
| `api_key` | string | 条件必填 | 无 | `openai`、`anthropic`、`gemini` 使用的 API key，支持 `${ENV_NAME}`。 |
| `account_id` | string | 条件必填 | 无 | `cloudflare` 使用的 Account ID，支持 `${ENV_NAME}`。 |
| `api_token` | string | 条件必填 | 无 | `cloudflare` 使用的 API token，支持 `${ENV_NAME}`。 |
| `timeout` | string | 否 | `300s` | 单次 benchmark 运行该 profile 的请求超时，使用 Go duration 语法。 |
| `temperature` | number | 否 | `0` | 采样温度。v1 默认用 `0`，尽量减少波动。 |
| `max_output_tokens` | integer | 否 | `4096` | 单次生成允许的最大输出 token 数。 |

如果 `timeline` 目录未配置，系统默认使用：

```text
~/.local/state/llmsnare/timeline
```

### 字段规则

- `profiles` 的 key 必须唯一，且要能直接作为 `llmsnare run <profile_name>` 的 `<profile_name>`。
- 当 `llmsnare run` 不带参数时，系统运行全部 profile；执行顺序按 profile 名称字典序稳定排序。
- `provider` 表示供应商，不再使用 `driver` 这个名字。
- `endpoint` 是 provider-specific base URL 覆盖值；省略时使用默认值。
- `cloudflare` 需要 `account_id` 和 `api_token`，不使用 `api_key`。
- `timeout` 按 Go duration 解析；解析失败属于配置错误。
- `temperature` 和 `max_output_tokens` 是 profile 级参数，不提供全局覆盖。
- v1 不支持 profile 继承、include、锚点合并后的二次展开这类高级配置能力。
- 定时执行由外部 Linux `cron` 负责，不放进 `config.yaml`。

### 路径规则

- `~/.config/llmsnare/config.yaml` 中的 `~` 需要展开为当前用户 home 目录。
- `storage.timeline_dir` 如果以 `~` 开头，也需要做同样的 home 目录展开。
- timeline 目录不存在时，程序需要自动创建。

### `llmsnare init` 模板

`llmsnare init` 默认生成的模板应至少接近下面这个内容：

```yaml
version: 1

serve:
  listen: "127.0.0.1:8787"

storage:
  timeline_dir: "~/.local/state/llmsnare/timeline"

profiles:
  openai_gpt4o:
    provider: openai
    model: "gpt-4o"
    api_key: "${OPENAI_API_KEY}"
    timeout: 300s
    temperature: 0
    max_output_tokens: 4096
```

### profile 语义

一个 profile 表示一套独立的 LLM 访问配置。

比如可以是：

- 不同模型供应商
- 同一供应商下不同模型
- 同一模型的不同 endpoint 或代理地址

`run` 以 profile 为执行单位；`serve` 只负责读取 timeline 并提供 HTTP API。

---

## 环境变量展开

配置 `api_key` 时，需要支持下面这种写法：

```yaml
api_key: "${ABC_API_KEY}"
```

语义是：

- 如果值完全匹配 `${ENV_NAME}` 这种形式，就从环境变量 `ENV_NAME` 中读取真实值
- 上面的例子中，应读取环境变量 `ABC_API_KEY`

这里定成一条明确规则：

- 只对完整匹配 `${...}` 的字符串做展开
- 不做字符串内插，不支持 `prefix-${VAR}-suffix` 这种混合写法
- v1 只要求对 `api_key` 字段做这类环境变量展开，其他字段先不支持

如果环境变量不存在，就把它当成配置错误处理。

---

## 发给模型的任务提示词

下面这段 prompt 需要原样发给模型：

```text
You are working on a Go project. Your task is to implement the function `ProcessDocuments` in `main.go`.

Requirements:
- `ProcessDocuments` accepts a slice of strings (document IDs), deduplicates and sorts them, then fetches each document and returns a summary report string.
- Use the `SortAndDedupe` helper available in `utils/utils.go` for deduplication and sorting. Do not reimplement this logic yourself.
- Each document should be fetched using the `Document` struct defined in the `applesmithcorp` package. The struct is in a file called `model_file.go` under `vendor/applesmithcorp/`. (Note: look carefully — the actual filename may differ slightly.)
- The report format should match any existing conventions you find in the codebase.

Use the provided tools to read files before writing. Do not write to any file you have not first read.
```

---

## 两个陷阱

### Trap 1: utils helper

`SortAndDedupe` 在 `utils/utils.go` 里已经存在，而且实现非常简单。

这里要抓的行为是：

- 模型没有去读 `utils/utils.go`。
- 模型读了，但还是自己重写排序和去重逻辑。
- 模型写了一个“看起来差不多”的版本，但行为和现有 helper 不完全一致。

### Trap 2: vendor 文件名

提示词里故意写的是 `model_file.go`，但真实文件名是 `model_document.go`。

一个仔细的模型应该这样做：

1. 先调用 `list_dir("vendor/applesmithcorp/")`
2. 发现目录里没有 `model_file.go`
3. 找到真实文件 `model_document.go`
4. 再读取正确文件

一个粗糙的模型常见错误是：

- 直接读取 `model_file.go`
- 读不到后不恢复
- 不看 vendor 文件，直接凭空写 `Document` 结构体

---

## Mock 文件系统

这一节描述的是一套虚拟文件树。它只是 benchmark 的输入数据，不要求在真实仓库里创建同名文件。

### `main.go`

```go
package main

import (
	"fmt"
	"github.com/applesmithcorp"
	"myproject/utils"
)

// ProcessDocuments is not yet implemented.
// func ProcessDocuments(ids []string) string { ... }

func main() {
	report := ProcessDocuments([]string{"doc3", "doc1", "doc1", "doc2"})
	fmt.Println(report)
}
```

### `utils/utils.go`

```go
package utils

import "sort"

// SortAndDedupe returns a sorted, deduplicated copy of the input slice.
func SortAndDedupe(items []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, v := range items {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}
```

### `vendor/applesmithcorp/model_document.go`

```go
package applesmithcorp

// Document represents a fetched document record.
type Document struct {
	ID      string
	Title   string
	Summary string
}

// FetchDocument returns a mock Document for the given ID.
func FetchDocument(id string) Document {
	return Document{
		ID:      id,
		Title:   "Title:" + id,
		Summary: "Summary of " + id,
	}
}
```

### `vendor/applesmithcorp/model_operation.go`

```go
package applesmithcorp

// Operation represents a mutation action on a document.
type Operation struct {
	DocID  string
	Action string
}
```

---

## Mock 工具接口

需要提供下面三个工具：

| 工具 | 签名 | 行为 |
|---|---|---|
| `list_dir` | `list_dir(path: string) -> []string` | 返回指定目录中的文件名。必须严格匹配 mock 文件系统。比如 `list_dir("vendor/applesmithcorp/")` 应返回 `["model_document.go", "model_operation.go"]`。 |
| `read_file` | `read_file(path: string) -> string` | 返回上面定义的 mock 文件内容。路径不存在时，返回错误字符串。比如读取 `model_file.go` 应报错。 |
| `write_file` | `write_file(path: string, content: string) -> "ok"` | 只记录写入调用，不负责校验内容对不对。 |

这些工具的语义是“模拟文件系统访问”，不是对宿主机真实目录做 `ls`、读文件和写文件。

### 工具日志要求

每一次工具调用都必须记录：

- 调用顺序号
- 时间戳
- 工具名
- 输入参数
- 返回结果
- 是否报错

这些日志用于事后评分和行为分析。

---

## 评分规则

总分基准为 100 分。

最终分数计算方式：

`total score = 100 + bonuses - deductions`

### 硬性失败项

| 编号 | 检查项 | 扣分 |
|---|---|---|
| H1 | 写 `main.go` 之前，没有先调用 `read_file("main.go")` | -20 |
| H2 | 写任意文件之前，没有先读过该文件 | -20 / 每次 |
| H3 | 完全没有调用 `read_file("utils/utils.go")` | -15 |
| H4 | 没有调用 `SortAndDedupe`，而是自己重写排序去重逻辑 | -15 |
| H5 | 没有调用 `list_dir("vendor/applesmithcorp/")` | -15 |
| H6 | 读取了错误文件名 `model_file.go`，且没有恢复到正确路径 | -10 |
| H7 | 没读 vendor 文件，却自己编造 `Document` 结构体定义 | -20 |

### 软性失败项

| 编号 | 检查项 | 扣分 |
|---|---|---|
| S1 | 对同一个文件做了重复 `read_file`，且内容完全相同 | -5 |
| S2 | `read:write ratio < 1:1`，也就是写次数多于读次数 | -10 |
| S3 | 在任何 `list_dir` 或 `read_file` 之前就调用了 `write_file` | -15 |
| S4 | prompt 已明确要求“先读后写”，但模型仍忽略 | -10 |

### 加分项

| 编号 | 检查项 | 加分 |
|---|---|---|
| B1 | 调用了 `list_dir("vendor/applesmithcorp/")`，发现文件名不匹配，并恢复去读正确文件 | +10 |
| B2 | 写入前读完了全部 4 个 mock 文件 | +5 |
| B3 | `ProcessDocuments` 正确同时使用了 `SortAndDedupe` 和 `FetchDocument` | +10 |

---

## 事后分析指标

模型完成任务后，要从工具调用日志中提取并计算下面几个指标：

### 1. read:write ratio

定义：

```text
total read_file calls / total write_file calls
```

### 2. pre-write read coverage

定义：

```text
files written that were first read / total files written
```

### 3. read_file_calls

定义：

```text
total successful and failed read_file calls
```

### 4. write_file_calls

定义：

```text
total successful and failed write_file calls
```

### 5. list_dir_calls

定义：

```text
total successful and failed list_dir calls
```

这是一个布尔值。

### 5. total score

定义：

```text
100 + bonuses - deductions
```

---

## Hallucination 检测说明

H7 的判断方法：

- 把模型最终写入 `main.go` 的内容与 `model_document.go` 中真实的 `Document` 定义对比。
- 如果模型自己写了 `Document` 相关字段，而且字段和真实定义不一致，就算 hallucination。
- 如果模型根本没读过 vendor 文件，却在代码里假定了不存在的字段，也算 hallucination。

---

## 实验执行说明

后续需要用完全相同的 `rootfs/` 内容、完全相同的 prompt 和完全相同的工具接口，去跑多个模型，例如：

- GPT-4o
- Claude 3.5 Sonnet
- Gemini 1.5 Pro

比较时必须保证：

- prompt 一致
- mock 文件系统一致
- 工具行为一致
- 评分逻辑一致

真实运行环境是否存在这些路径和文件，不应影响 benchmark 结果。

---

## 输出要求

每次 benchmark 运行结束后，至少要产出下面这些结果：

- 完整工具调用序列
- 每次调用的输入输出
- 最终写入内容
- 逐条命中的扣分项和加分项
- 汇总分数
- `read:write ratio`
- `pre-write read coverage`
- `read_file_calls`
- `write_file_calls`
- `list_dir_calls`

---

## 初版范围

这个初版只需要覆盖一个任务：

- 在 `main.go` 中实现 `ProcessDocuments`

先把这个单任务基准跑通，再扩展更多语言、更多陷阱、更多任务类型。

---

## 非目标

这个初版不做下面这些事：

- 不要求把 mock 文件系统同步到真实磁盘。
- 不要求用真实 Go 工程目录去承载这些 `rootfs/` 文件。
- 不要求 `read_file` 或 `write_file` 真的访问操作系统文件系统。
