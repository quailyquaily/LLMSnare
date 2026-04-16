# Config

`llmsnare init` writes this template:

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
    model_vendor: "openai"
    inference_provider: "openai"
    api_key: "${OPENAI_API_KEY}"
    timeout: 300s
    max_output_tokens: 4096
```

Notes:

- benchmark cases live under `benchmarks/` relative to the config file directory
- `--case` accepts either a case ID or a case directory path
- `run` requires `--case`
- if `benchmarks/` is empty, run `llmsnare init`
- if cases already exist, run `llmsnare cases` and then pass `--case`
- `run --persist` appends JSONL entries under `storage.timeline_dir`
- use Linux `cron` to schedule repeated runs; see [linux_cron_examples.md](./linux_cron_examples.md)
- supported providers are `openai`, `openai_resp`, `anthropic`, `gemini`, and `cloudflare`
- `provider` is the API integration type; `model_vendor` and `inference_provider` are optional metadata
- `model_vendor` names the organization that publishes the model
- `inference_provider` names the service that actually hosts and serves the model
- `api_key` supports `${ENV_NAME}` expansion
- `endpoint` is optional; if omitted, a provider-specific default is used
- `openai_resp` uses the native OpenAI Responses API and currently requires the default OpenAI base URL
- `cloudflare` profiles use `account_id` plus `api_token` instead of `api_key`
- Anthropic endpoint overrides are currently rejected because the configured `uniai` provider does not expose a custom base URL

Example Linux `cron` entry:

```cron
0 */6 * * * <llmsnare_bin> run openai_gpt4o --config <config_file> --case read_write_ratio_sample --persist
```

More examples:

- [linux_cron_examples.md](./linux_cron_examples.md)

Cloudflare example:

```yaml
profiles:
  cf_llama:
    provider: cloudflare
    model: "@cf/meta/llama-3.1-8b-instruct"
    model_vendor: "meta"
    inference_provider: "cloudflare"
    account_id: "${CLOUDFLARE_ACCOUNT_ID}"
    api_token: "${CLOUDFLARE_API_TOKEN}"
    timeout: 300s
    max_output_tokens: 4096
```
