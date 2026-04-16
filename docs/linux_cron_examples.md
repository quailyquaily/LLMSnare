# Linux Cron Examples

`llmsnare run` 适合由 Linux `cron` 定时触发。下面给几种常见写法。

## Before You Start

- 使用你自己的明确路径或占位变量，不要依赖 `cron` 的当前目录。
- `run` 需要显式传 `--case`。
- 如果要保留时间线，带上 `--persist`。
- `cron` 环境很小。API key 通常放到单独的环境文件里，再在任务里加载。

示例环境文件：

```sh
OPENAI_API_KEY=...
GEMINI_API_KEY=...
CLOUDFLARE_ACCOUNT_ID=...
CLOUDFLARE_API_TOKEN=...
```

## User Crontab

先执行：

```bash
crontab -e
```

每 6 小时跑一次单个 profile：

```cron
MAILTO=""
PATH=<bin_dir>:/usr/bin:/bin

0 */6 * * * . <env_file> && <llmsnare_bin> run openai_gpt4o --config <config_file> --case read_write_ratio_sample --persist >> <log_file> 2>&1
```

每天凌晨 2:15 跑全部 profile：

```cron
15 2 * * * . <env_file> && <llmsnare_bin> run --config <config_file> --case read_write_ratio_sample --persist >> <log_file> 2>&1
```

每个小时的第 16 分跑一次单个 profile：

```cron
16 * * * * . <env_file> && <llmsnare_bin> run openai_gpt4o --config <config_file> --case read_write_ratio_sample --persist >> <log_file> 2>&1
```

## Avoid Overlap

如果单次运行可能超过调度间隔，建议加锁：

```cron
*/30 * * * * . <env_file> && flock -n <lock_file> <llmsnare_bin> run gemini_main --config <config_file> --case read_write_ratio_sample --persist >> <log_file> 2>&1
```

`flock -n` 拿不到锁就直接退出，避免并发写 timeline。

## System `cron.d` Example

如果你用系统级任务文件，可以放一份到你自己的 `cron.d` 目录。注意这里比用户 crontab 多一个用户字段：

```cron
SHELL=/bin/sh
PATH=<bin_dir>:/usr/bin:/bin
MAILTO=""

0 */6 * * * root . <env_file> && <llmsnare_bin> run openai_resp_gpt54 --config <config_file> --case read_write_ratio_sample --persist >> <log_file> 2>&1
```

## Quick Checks

- 用 `llmsnare profiles --config <config_file>` 先确认 profile 名称。
- 先手动跑一次 `llmsnare run ...`，确认配置、环境变量和 case 都没问题。
- 如果没有日志，先检查 `cron` 是否真的加载了环境文件。
