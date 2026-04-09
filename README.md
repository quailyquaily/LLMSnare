# llmsnare

`llmsnare` is a Go CLI for running a context-fidelity benchmark against LLM profiles.

The benchmark checks whether a model actually reads code before editing, follows existing helpers, recovers from misleading paths, and writes output that matches repository conventions.

## Commands

### Initialize

Generate a config file plus the built-in benchmark case:

```bash
llmsnare init
```

Or write into a custom location:

```bash
llmsnare init --config /tmp/llmsnare/config.yaml
```

This creates:

- `config.yaml`
- `benchmarks/read_write_ratio_smoke_v1/case.yaml`
- a matching fixture directory for that case

### Run Once

Run one profile:

```bash
llmsnare run openai_gpt4o --config /tmp/llmsnare/config.yaml
```

Run all profiles:

```bash
llmsnare run --config /tmp/llmsnare/config.yaml
```

Print JSON:

```bash
llmsnare run openai_gpt4o --config /tmp/llmsnare/config.yaml --json
```

Override the case file and fixture directory without editing config:

```bash
llmsnare run openai_gpt4o \
  --config /tmp/llmsnare/config.yaml \
  --case /tmp/llmsnare/benchmarks/read_write_ratio_smoke_v1/case.yaml \
  --fixture-dir /home/user/my-fixtures/read-write-ratio
```

### Serve

Run on a schedule and expose timelines over HTTP:

```bash
llmsnare serve --config /tmp/llmsnare/config.yaml
```

The same `--case` and `--fixture-dir` overrides are available in `serve` mode.

## Config

Example:

```yaml
version: 1

benchmark:
  case_file: "benchmarks/read_write_ratio_smoke_v1/case.yaml"

serve:
  interval: 6h
  listen: "127.0.0.1:8787"

storage:
  timeline_dir: "~/.local/state/llmsnare/timeline"

profiles:
  openai_gpt4o:
    driver: openai
    model: "gpt-4o"
    endpoint: "https://api.openai.com/v1"
    api_key: "${OPENAI_API_KEY}"
    timeout: 90s
    temperature: 0
    max_output_tokens: 4096
```

Notes:

- `benchmark.case_file` points to a `case.yaml`
- relative paths are resolved relative to the config file directory
- `api_key` supports `${ENV_NAME}` expansion
- `endpoint` is required for every profile
- Anthropic endpoint overrides are currently rejected because the configured `uniai` provider does not expose a custom base URL

## Case Files

Each benchmark case is defined by a `case.yaml` plus a fixture directory.

Example layout:

```text
benchmarks/
  read_write_ratio_smoke_v1/
    case.yaml
    fixture/
      main.go
      docs/format.txt
```

`case.yaml` points to the fixture tree with:

```yaml
fixture_dir: fixture
```

`fixture_dir` may be:

- relative to the case file directory
- an absolute path
- a `~`-prefixed home path

See [case_format.md](./docs/case_format.md) for the full case schema.

See [check_reference.md](./docs/check_reference.md) for supported `check.type` values.

## Built-In Cases

Built-in cases live in the source tree and are embedded into the binary:

- [read_write_ratio_smoke_v1](./internal/benchcase/testdata/builtin/benchmarks/read_write_ratio_smoke_v1/case.yaml)

`llmsnare init` copies this embedded case to disk so you can edit it freely.

You can keep richer local-only examples under `samples/`; that directory is ignored by git.

## HTTP API

When running `serve`, the daemon exposes:

- `GET /healthz`
- `GET /openapi.yaml`
- `GET /api/v1/timelines`
- `GET /api/v1/timelines/{profile}`

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
