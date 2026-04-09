# LLM Snare Implementation Tracking

## Goal

Build the initial `llmsnare` Go application described in [init_requirement.md](./init_requirement.md).

## Confirmed Decisions

- CLI framework: `cobra`
- LLM integration library: `github.com/quailyquaily/uniai`
- HTTP contract: define and serve an OpenAPI spec
- Failed benchmark runs may still be persisted to timeline storage
- Benchmark case definitions should live outside source code
- Rootfs files should come from a real directory on disk, not inline YAML blobs
- Embedded built-ins should stay minimal; richer local examples can live under ignored `samples/`

## Scope for This Pass

- Implement `llmsnare init`
- Implement `llmsnare run [profile_name]`
- Implement `llmsnare serve`
- Load and validate v1 config
- Run the single benchmark rootfs from the requirement
- Load the benchmark definition from a case YAML file
- Load rootfs files from the fixed `rootfs/` directory in each case
- Persist serve-mode timeline entries as JSONL
- Expose timeline data over HTTP
- Produce and serve an OpenAPI document

## Implementation Tasks

- [x] Initialize Go module and dependencies
- [x] Scaffold Cobra root command and subcommands
- [x] Implement config parsing, validation, and path expansion
- [x] Implement environment variable expansion for `api_key`
- [x] Implement the in-memory rootfs file system
- [x] Refactor benchmark data into an external case definition
- [x] Load rootfs files from case directories
- [x] Implement mock tools and structured tool-call logging
- [x] Implement the `uniai`-based benchmark loop
- [x] Implement scoring and post-run metrics
- [x] Implement JSONL timeline persistence
- [x] Implement HTTP handlers for timeline APIs
- [x] Add OpenAPI spec and serve it from the daemon
- [x] Add tests for config, scoring, and storage
- [x] Run formatting and tests

## Notes

- `uniai` supports custom base URLs for `openai` and `gemini`.
- `uniai`'s `anthropic` provider does not expose a custom endpoint, so config validation rejects non-default Anthropic endpoints.
- H4 should be judged from the written `main.go` content plus helper-read behavior, not from model prose.
- H7 should be judged from the written `main.go` content plus whether the vendor file was actually read.
- Checker field reference lives in [check_reference.md](./check_reference.md).
- Case file format reference lives in [case_format.md](./case_format.md).
