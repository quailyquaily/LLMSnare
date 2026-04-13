# LLMSnare

`llmsnare` is a Go CLI for running a context-fidelity benchmark against LLM profiles.

The benchmark checks whether a model actually reads code before editing, follows existing helpers, recovers from misleading paths, and writes output that matches repository conventions.

## What It Measures

- `llmsnare` is meant for repeated runs against the same case set. The normal workflow is to persist results and inspect timelines, not to treat one run as the whole story.
- It focuses on tool-driven code agent behavior: whether the model will `list_dir`, `read_file`, and only then `write_file`.
- It is useful for judging whether a model is likely to behave well in a tool-calling agent. Models that are weak at tool use usually score poorly here.
- It checks whether the model writes too early after seeing partial context. Low `read_write_ratio`, low `pre_write_read_coverage`, and writes before required reads are all penalized.
- It does not hide the reasons behind the score. Each run records tool logs, final writes, deductions, bonuses, and automatic metrics.
- Coverage is case-driven. The runner stays fixed, while `prompt`, `rootfs`, and scoring rules define what each case actually tests.

## What It Does Not Measure

- It does not rank general model intelligence or broad ability across arbitrary coding, math, or writing tasks.
- It cannot prove that a model is broadly "good at coding". It only shows how the model behaves inside the specific benchmark cases you run.
- A score here should be read as evidence about context fidelity and tool-use discipline, not as a complete judgment of agent quality.
- Conclusions are only as strong as the case set. A small smoke-test case supports only small conclusions.

## Commands

### Initialize

Generate a config file plus the built-in benchmark case:

```bash
llmsnare init
```

If `--config` is omitted, `init` writes to:

- `~/.config/llmsnare/config.yaml`
- `~/.config/llmsnare/benchmarks/read_write_ratio_sample/case.yaml`
- `~/.config/llmsnare/benchmarks/read_write_ratio_sample/rootfs/...`

Or write into a custom location:

```bash
llmsnare init --config ./config.yaml
```

This creates:

- `config.yaml`
- `benchmarks/read_write_ratio_sample/case.yaml`
- `benchmarks/read_write_ratio_sample/rootfs/...`

### List Cases

List all cases under the default cases directory:

```bash
llmsnare cases
```

### List Profiles

List all configured profiles:

```bash
llmsnare profiles --config ./config.yaml
```

### Run Once

Run one profile:

```bash
llmsnare run openai_gpt4o --config ./config.yaml
```

Run all profiles:

```bash
llmsnare run --config ./config.yaml
```

Run multiple profiles in parallel while avoiding same-prefix profiles at the same time:

```bash
llmsnare run --config ./config.yaml --parallel 4
```

Print JSON:

```bash
llmsnare run openai_gpt4o --config ./config.yaml --json
```

Persist the result to timeline storage:

```bash
llmsnare run openai_gpt4o --config ./config.yaml --case read_write_ratio_sample --persist
```

Run a case by case ID:

```bash
llmsnare run openai_gpt4o \
  --config ./config.yaml \
  --case read_write_ratio_sample
```

Run a case by case directory path:

```bash
llmsnare run openai_gpt4o \
  --config ./config.yaml \
  --case ./benchmarks/read_write_ratio_sample
```

### Serve

Expose timelines over HTTP:

```bash
llmsnare serve --config ./config.yaml
```

### Maintain Timeline Storage

Backfill missing `run_id` values in existing WAL files:

```bash
llmsnare timeline backfill-run-id --config ./config.yaml
```

Rebuild the SQLite projection from WAL:

```bash
llmsnare timeline rebuild-sqlite --config ./config.yaml
```

Inspect whether timeline reads currently use WAL or SQLite:

```bash
llmsnare timeline status --config ./config.yaml
```

## Release

Push a version tag to trigger the GitHub Actions release workflow:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs GoReleaser and publishes Linux `amd64` and `arm64`
binaries plus `checksums.txt` to the GitHub Release.

If the tag already exists and needs to be published again, run the
`release` workflow manually from GitHub Actions and pass the tag name in
the `tag` input.

## Config

Example:

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
- use Linux `cron` to schedule repeated runs; see [linux_cron_examples.md](./docs/linux_cron_examples.md)
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
0 */6 * * * /usr/local/bin/llmsnare run openai_gpt4o --config /etc/llmsnare/config.yaml --case read_write_ratio_sample --persist
```

More examples:

- [linux_cron_examples.md](./docs/linux_cron_examples.md)

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

See [case_format.md](./docs/case_format.md) for the full case schema.

See [check_reference.md](./docs/check_reference.md) for supported `check.type` values.

## Built-In Cases

Built-in cases live in the source tree and are embedded into the binary:

- [read_write_ratio_sample](./internal/benchcase/testdata/builtin/benchmarks/read_write_ratio_sample/case.yaml)

Case notes:

- [read_write_ratio_sample README](./internal/benchcase/testdata/builtin/benchmarks/read_write_ratio_sample/README.md)

`llmsnare init` copies this embedded case to disk so you can edit it freely.

You can keep richer local-only examples under `samples/`; that directory is ignored by git.

## HTTP API

When running `serve`, the daemon exposes:

- `GET /healthz`
- `GET /openapi.yaml`
- `GET /v1/timelines`
- `GET /v1/timelines/{profile}`

Response shapes:

- `GET /healthz` returns `{"status":"ok"}`
- `GET /v1/timelines` returns `{"profiles":{"<profile>":[BenchmarkResult,...]}}`
- `GET /v1/timelines/{profile}` returns `{"profile":"<profile>","entries":[BenchmarkResult,...]}`
- all endpoints include `Access-Control-Allow-Origin: *` for browser access
- timeline endpoints default to the latest 1024 entries and cap `limit` at 1024
- timeline endpoints support `model_vendor` and `inference_provider` query filters

Each `BenchmarkResult` includes:

- run metadata: `run_id`, `timestamp`, `finished_at`, `case_id`, `profile`, `provider`, `model`, `model_vendor`, `inference_provider`, `success`
- scores: `total_score`, `raw_score`, `max_score`, `normalized_score`
- automatic metrics: `read_file_calls`, `write_file_calls`, `list_dir_calls`, `read_write_ratio`, `pre_write_read_coverage`
- scoring details: `deductions`, `bonuses`

The API intentionally omits:

- `endpoint`
- `final_writes`
- `final_response`
- `bonuses[].description`
- `tool_calls`

Default listen address:

```text
127.0.0.1:8787
```

OpenAPI spec:

- [openapi.yaml](./internal/api/openapi.yaml)

## Development

Run tests:

```bash
go test ./...
```
