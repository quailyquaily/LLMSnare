# LLMSnare

`llmsnare` is a Go CLI for running a context-fidelity benchmark against LLM profiles:

It tries to answer one narrower, more practical question: when an LLM works as an agent, did it actually read the required context, follow instructions, and do only the necessary work?

Languages: [中文](./docs/README.zh.md) , [日本語](./docs/README.ja.md)

## Online Arena

https://mistermorph.com/llmsnare/arena/

## Getting Started

### 1. Download the latest binary

Download the latest binary for your OS and architecture from [GitHub Releases](https://github.com/quailyquaily/LLMSnare/releases/latest), and put `llmsnare` in your `PATH`.

### 2. Initialize

```bash
llmsnare init
```

By default, this generates `config.yaml` and the sample case under `~/.config/llmsnare/`.

### 3. Edit the config

Open `~/.config/llmsnare/config.yaml` and fill in the profiles and API keys in `config.yaml`.

Each profile represents one LLM. You need to add at least one profile.

### 4. List the cases

```bash
llmsnare cases
```

### 5. Run one benchmark

```bash
llmsnare run --case <case_name>
```

## Why LLMSnare

Most popular benchmarks score the end state: is the answer correct.

But the agent's operating process is barely measured: did it read the required information first, how much coverage did it read before acting, did it reuse existing tools or code.

- Example 1: an LLM is asked to complete a writing task by following a style guide, but it never reads the style guide file and starts writing immediately. That is laziness. Its instruction following is weak.
- Example 2: an LLM is given a slightly wrong instruction, but tool calling can recover the correct context. If it still sticks to the wrong instruction after seeing the correct context, then it lacks recovery ability.
- Example 3: an LLM is given an ambiguous instruction, but tool calling can recover the full context. If it wanders around inside that ambiguity and needs many tool calls to understand, or still fails to understand, then its base capability is weak.

LLMSnare measures an agent behavior benchmark, not result quality. The behavior benchmark includes read before write, reuse existing information, recover from errors, and keep that discipline across repeated runs.

## Quick Comparison

| Benchmark | What it runs | How it scores | What it misses |
|---|---|---|---|
| HumanEval / MBPP | Small standalone coding tasks | Unit tests on final code | Little or no repository exploration, almost no process signal |
| SWE-bench | Real GitHub issue fixes in real repos | Test pass / issue resolution | Strong on outcome, weak on pre-write reading behavior |
| WebArena / OSWorld | Browser or OS tasks | Task success, action sequence | Good for UI agents, not for repository editing discipline |
| LLMSnare | Agent-like tasks | Tool logs, behavior metrics, case rules, and final writes | Uses mock `rootfs/` and custom case sets |

## What LLMSnare Measures

- `llmsnare` is meant for repeated runs against the same case set. The normal workflow is to persist results and inspect timelines, not to treat one run as the whole story.
- For now, it measures repository-reading discipline under tool use: `list_dir`, `read_file`, then `write_file`.
- It extracts behavior metrics from tool logs, such as `read_file_calls`, `write_file_calls`, `list_dir_calls`, `read_write_ratio`, and `pre_write_read_coverage`.
- It supports custom benchmark sets, with case-level rules for required reads, helper reuse, recovery from wrong paths, and output conventions.
- It keeps the reasons in the result files. Each run records deductions, bonuses, final writes, and tool logs for audit and replay.

## What It Cannot Measure

- It does not rank general model intelligence.
- It cannot prove that a model truly has deep understanding or is generally "smart", because the metrics are still behavior proxies.
- It does not replace end-to-end correctness benchmarks such as test-based evaluation in real repos.

## Current Limitations

- The built-in case set is very small and only serves as an example. For real evaluation, you need to write your own benchmark set. The official set details are not public, but the data can be seen in [LLMSnare Arena](https://mistermorph.com/llmsnare/arena/).
- Conclusions depend on the case set you run. Different people will run different case sets and get different conclusions.
- The current mock `rootfs/` may be easier to game.
- Current behavior metrics can only provide limited evidence of understanding.
- The current runner only supports single-task snapshots, not long multi-turn execution traces.

## Future Direction

- Add more behavior signals, low-value repeated exploration, and recovery after tool errors.
- Add longer multi-turn cases and measure whether behavior drifts as tasks get longer.
- Expand the case set and accept community-submitted cases.

## Commands

See [docs/cmd.md](./docs/cmd.md).

## Config

See [docs/config.md](./docs/config.md).

## Case Files

Each benchmark case is a directory with a fixed shape:

Example layout:

```text
benchmarks/
  read_write_ratio_sample/
    case.yaml
    rootfs/
      main.go
      docs/format.txt
```

`rootfs/` is loaded into memory and exposed to the model through mock tools. It is not edited as a real working tree.

See [case_guide.md](./docs/case_guide.md) for guidance on writing community cases.

See [case_format.md](./docs/case_format.md) for the full case schema.

See [check_reference.md](./docs/check_reference.md) for supported `check.type` values.

### Built-In Cases

`llmsnare init` copies several built-in cases to `~/.config/llmsnare/benchmarks/`.

Current built-ins:

- `read_write_ratio_sample`
- `style_required_reads`

## HTTP API

See [docs/api.md](./docs/api.md).

## Development

Run tests:

```bash
go test ./...
```
