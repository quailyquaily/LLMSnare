# Linux Cron Examples

`llmsnare run` 适合由 Linux `cron` 定时触发。下面给几种常见写法。

## Before You Start

- 使用绝对路径，不要依赖 `cron` 的当前目录。
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
PATH=/usr/local/bin:/usr/bin:/bin

0 */6 * * * . /etc/llmsnare/env && /usr/local/bin/llmsnare run openai_gpt4o --config /etc/llmsnare/config.yaml --case read_write_ratio_sample --persist >> /var/log/llmsnare/run.log 2>&1
```

每天凌晨 2:15 跑全部 profile：

```cron
15 2 * * * . /etc/llmsnare/env && /usr/local/bin/llmsnare run --config /etc/llmsnare/config.yaml --case read_write_ratio_sample --persist >> /var/log/llmsnare/run.log 2>&1
```

每个小时的第 16 分跑一次单个 profile：

```cron
16 * * * * . /etc/llmsnare/env && /usr/local/bin/llmsnare run openai_gpt4o --config /etc/llmsnare/config.yaml --case read_write_ratio_sample --persist >> /var/log/llmsnare/run.log 2>&1
```

## Avoid Overlap

如果单次运行可能超过调度间隔，建议加锁：

```cron
*/30 * * * * . /etc/llmsnare/env && flock -n /var/run/llmsnare.lock /usr/local/bin/llmsnare run gemini_main --config /etc/llmsnare/config.yaml --case read_write_ratio_sample --persist >> /var/log/llmsnare/run.log 2>&1
```

`flock -n` 拿不到锁就直接退出，避免并发写 timeline。

## `/etc/cron.d` Example

如果你用系统级任务文件，可以放一份到 `/etc/cron.d/llmsnare`。注意这里比用户 crontab 多一个用户字段：

```cron
SHELL=/bin/sh
PATH=/usr/local/bin:/usr/bin:/bin
MAILTO=""

0 */6 * * * root . /etc/llmsnare/env && /usr/local/bin/llmsnare run openai_resp_gpt54 --config /etc/llmsnare/config.yaml --case read_write_ratio_sample --persist >> /var/log/llmsnare/run.log 2>&1
```

## Quick Checks

- 用 `llmsnare profiles --config /etc/llmsnare/config.yaml` 先确认 profile 名称。
- 先手动跑一次 `llmsnare run ...`，确认配置、环境变量和 case 都没问题。
- 如果没有日志，先检查 `cron` 是否真的加载了环境文件。
