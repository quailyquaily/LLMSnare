# Commands

## Initialize

Generate a config file plus the built-in benchmark case:

```bash
llmsnare init
```

If `--config` is omitted, `init` writes to:

- `<default_config_dir>/config.yaml`
- `<default_config_dir>/benchmarks/read_write_ratio_sample/case.yaml`
- `<default_config_dir>/benchmarks/read_write_ratio_sample/rootfs/...`

Or write into a custom location:

```bash
llmsnare init --config ./config.yaml
```

This creates:

- `config.yaml`
- `benchmarks/read_write_ratio_sample/case.yaml`
- `benchmarks/read_write_ratio_sample/rootfs/...`

## List Cases

List all cases under the default cases directory:

```bash
llmsnare cases
```

## List Profiles

List all configured profiles:

```bash
llmsnare profiles --config ./config.yaml
```

## Run Once

Run one profile:

```bash
llmsnare run openai_gpt4o --config ./config.yaml --case read_write_ratio_sample
```

Run all profiles:

```bash
llmsnare run --config ./config.yaml --case read_write_ratio_sample
```

Run multiple profiles in parallel while avoiding same-prefix profiles at the same time:

```bash
llmsnare run --config ./config.yaml --case read_write_ratio_sample --parallel 4
```

Print JSON:

```bash
llmsnare run openai_gpt4o --config ./config.yaml --case read_write_ratio_sample --json
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

## Serve

Expose timelines over HTTP:

```bash
llmsnare serve --config ./config.yaml
```

## Maintain Timeline Storage

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
