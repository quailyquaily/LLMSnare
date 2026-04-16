# LLMSnare

`llmsnare` 是一个 Go CLI，用来对 LLM profile 运行上下文忠实度基准：

它只尝试解决一个更窄、也更实际的问题：LLM 在以 agent 的方式工作时，是否真的读到必须的上下文，是否遵循指令，是否只做了必要的工作。

Languages: [English](../README.md) , [日本語](./README.ja.md)

## 在线 Arena

https://mistermorph.com/llmsnare/arena/

## Getting Started

### 1. 下载最新版 binary

从 [GitHub Releases](https://github.com/quailyquaily/LLMSnare/releases/latest) 下载适合你的操作系统和架构的最新版 binary，并把 `llmsnare` 放进 `PATH`。

### 2. 初始化：

```bash
llmsnare init
```

默认会在 `~/.config/llmsnare/` 下生成 config.yaml 和样例 case.

### 3. 修改配置

打开 `~/.config/llmsnare/config.yaml`，填好 `config.yaml` 里的 profile 和 API key。

每个 profile 表示一个 LLM。至少需要添加一个 profile。

### 4. 看一下 case

```bash
llmsnare cases
```

### 5. 跑一次基准测试

```bash
llmsnare run --case <case_name>
```

## 为什么是 LLMSnare

现在流行的 benchmark，大多看终局：答案对不对。

而 agent 的操作过程没怎么测：到底有没有先读必要信息、读了多少覆盖率再开始行动、有没有复用已有工具或者代码。

- 例 1：要求 LLM 参考一个写作规范来完成写作任务，但是 LLM 没有阅读写作规范文件，就开始输出文章，这种情况就是 LLM 偷懒了，对指令遵循程度有问题。
- 例 2：给予 LLM 一个有少许错误的指令，但可以通过 tool calling 获得正确的上下文。如果 LLM 在已经获得正确上下文的前提下，依然指向了错误的指令，那么说明它缺少从错误中恢复的能力。
- 例 3：给予 LLM 一个模糊的指令，但可以通过 tool calling 获得完整的上下文。如果 LLM 在模糊指令中徘徊，执行了很多很多次 tool calling 才能理解，或者依然不能理解，说明它的基础能力有问题。

LLMSnare 测的就是 agent 行为基准，不是结果质量。行为基准包括：先读后写、复用已有的信息、从错误中恢复、并且在重复运行里保持这种纪律。

## 比较一下

| Benchmark | 运行什么任务 | 怎么评分 | 盲区是什么 |
|---|---|---|---|
| HumanEval / MBPP | 小型独立编程题 | 看最终代码能否通过单元测试 | 几乎没有仓库探索，也几乎不看过程 |
| SWE-bench | 真实仓库里的 GitHub issue 修复 | 看测试是否通过、issue 是否解决 | 很强地看结果，但弱于“写前读了多少” |
| WebArena / OSWorld | 浏览器或操作系统任务 | 看任务是否成功、动作序列是否完成 | 适合 UI agent，不适合测仓库编辑纪律 |
| LLMSnare | 类 agent 任务 | 看工具日志、行为指标、case 规则和最终写入 | 使用 mock `rootfs/` 和自定义 case 集 |

## LLMSnare 测什么

- `llmsnare` 适合在同一组 case 上重复运行，持续结果形成 timeline，再看趋势，而不是把单次结果当成全部结论。
- 目前，它测的是工具调用下的仓库阅读纪律：比如先 `list_dir`、`read_file`，再 `write_file`。
- 从工具调用日志里提取行为指标，比如：`read_file_calls`、`write_file_calls`、`list_dir_calls`、`read_write_ratio`、`pre_write_read_coverage`。
- 支持自定义测试集，定制 case 级规则，去检查必须读取、helper 复用、错误路径恢复、输出是否符合约定。
- 把原因保留到文件。每次运行都会记录扣分、加分、最终写入和工具日志，方便审计和回溯。

## 不能测什么

- 不用于给通用模型智力排榜。
- 不能证明某个模型真的「理解很深」或「聪明」，因为 LLMSnare 关注的指标本质上还是行为代理。
- 不能替代真实仓库里的端到端正确性 benchmark。

## 当前限制

- 内置 case 集非常小，只作为例子；实际测试需要自己编写测试集。官方测试集细节不公开，但是数据可以在 [LLMSnare Arena](https://mistermorph.com/llmsnare/arena/) 查看。
- 测试结论如何取决于运行的测试集，每个人运行的测试集合不同，会得出不同的结论。
- 目前的 mock `rootfs/`，可能容易被针对。
- 目前的行为指标只能有限证明 LLM 的理解力。
- 现在只支持测试单任务快照，不是长链路多轮执行轨迹。

## 后续方向

- 增加更多行为信号，低价值重复探索、以及工具报错后的恢复。
- 增加更长的多轮 case，测行为是否随任务拉长而漂移。
- 扩 case 集，接受社区提交的 case。

## 命令

详细命令见 [cmd.md](./cmd.md)。

## 配置

详细配置见 [config.md](./config.md)。

## Case 文件

每个 benchmark case 都是固定形状的目录：

示例布局：

```text
benchmarks/
  read_write_ratio_sample/
    case.yaml
    rootfs/
      main.go
      docs/format.txt
```

`rootfs/` 会整体加载到内存里，再通过 mock 工具暴露给模型。它不会像真实 working tree 那样被直接编辑。

- 撰写 case 的引导见 [case_guide.md](./case_guide.md)。
- 完整 case schema 见 [case_format.md](./case_format.md)。
- 支持的 `check.type` 见 [check_reference.md](./check_reference.md)。

### 内置 Case

内置 case 提供了最简单的用例。`llmsnare init` 会把这个内置 case 复制到磁盘 `~/.config/llmsnare/benchmarks/`，可以基于这个 sample 自由修改。

## HTTP API

API 说明见 [api.md](./api.md)。

## 开发

运行测试：

```bash
go test ./...
```
