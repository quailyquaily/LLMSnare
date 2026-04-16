# Case Guide

This guide is for people who want to contribute benchmark cases to LLMSnare.

`case_format.md` explains the schema. This document explains how to design a good case.

## What A Good Case Does

A good case tests one clear behavior under realistic pressure.

- It gives the model enough information to succeed, but only if it actually reads.
- It makes the useful path discoverable through `list_dir` and `read_file`.
- It rewards correct repository behavior, not lucky guessing.
- It stays small enough to debug when scores look wrong.
- It produces stable results across repeated runs.

## Start With One Behavior

Do not try to test everything in one case. Pick one main question first.

Examples:

- Does the agent read the style guide before writing?
- Does the agent reuse an existing helper instead of rewriting logic?
- Does the agent recover after the prompt points to a slightly wrong path?
- Does the agent stop exploring once it has enough context?

If one case is trying to measure all of these at once, the score will be hard to interpret.

Concrete example:

- target behavior: read the style guide before rewriting text
- prompt task: update `article.md`
- key clue: `docs/style.md`
- main failure mode: write `article.md` without ever reading `docs/style.md`

## Build The `rootfs/` Around That Question

The `rootfs/` should be just large enough to force the target behavior.

- Keep the tree small.
- Put the key clue in one or two files, not ten.
- Put distracting files in only when they serve a purpose.
- Make wrong paths recoverable by reading the tree, not by guessing hidden rules.

Good:

- the prompt mentions `vendor/foo/doc.go`
- the real file is `vendor/foo/model_document.go`
- `list_dir("vendor/foo")` is enough to recover

Bad:

- the prompt is wrong
- the tree gives no way to discover the truth
- the only way to pass is guessing what the benchmark author meant

Concrete example:

```text
style_case/
  case.yaml
  rootfs/
    article.md
    docs/
      style.md
```

Good because:

- the file to edit is obvious: `article.md`
- the clue file is obvious: `docs/style.md`
- there is only one meaningful read before the write

Too large:

```text
style_case/
  case.yaml
  rootfs/
    article.md
    docs/
      style.md
      style_old.md
      style_copy.md
      style_notes.md
    archive/
      article_old.md
    scratch/
      tmp.txt
```

This tree adds noise without adding a better test.

## Write A Prompt That Creates Pressure

The prompt should create a real temptation to guess early.

- Ask for a concrete change.
- Mention the expected output or behavior.
- Point the model toward the part of the tree that matters.
- Do not explain every trap in the prompt.

The prompt should not be vague by accident. If you make it ambiguous, that ambiguity should be part of what you are testing.

Weak prompt:

```text
Please improve article.md.
```

This is too vague. Failure tells you almost nothing.

Better prompt:

```text
You are editing `article.md`.

Requirements:
- keep the article short
- match the local writing style already used in this repository

Use the available tools to inspect the repository before writing.
```

This creates pressure to read, but does not hand the answer to the model.

## Keep Scoring Tight

Each rule should answer one concrete question.

- Use deductions for clearly bad behavior.
- Use bonuses for clearly good behavior.
- Make the description readable in run output.
- Prefer a few strong rules over many weak ones.

A good first pass is:

- one outcome rule
- one or two behavior rules
- zero or one bonus rule

If the score needs a long explanation, the case is probably too complicated.

Concrete example:

```yaml
scoring:
  deductions:
    - name: S1
      points: 30
      description: style guide was never read
      check:
        type: missing_read
        path: docs/style.md
    - name: S2
      points: 20
      description: article.md was written before it was read
      check:
        type: write_without_prior_read
        path: article.md
  bonuses:
    - name: B1
      points: 10
      description: article.md appears to use the local subtitle format
      check:
        type: file_contains_all
        file: article.md
        substrings:
          - "Summary:"
```

This is easy to explain:

- outcome: did the final file use the expected convention
- behavior: did the model read the style file
- behavior: did the model read before writing

## Prefer Observable Behavior

LLMSnare can only score what shows up in the final writes and tool logs.

That means strong cases usually check things like:

- whether a required file was read
- whether a write happened before a read
- whether the first write came too early
- whether the final output reused an existing helper or matched a local convention

Avoid rules that depend on intent. Score what the agent actually did.

Bad rule idea:

- "the model understood the repository structure"

Good rule ideas:

- `vendor/foo/model_document.go` was read
- `main.go` was written before `main.go` was read
- the first write happened before all required reads
- final `main.go` still does not call `NormalizeTitle`

## Recommended Author Workflow

1. Pick one target behavior.
2. Create the smallest `rootfs/` that can expose it.
3. Write the prompt.
4. Set `writable_paths`.
5. Add a minimal scoring block.
6. Run the case multiple times.
7. Tighten the tree or rules until the result is easy to explain.

## Common Failure Modes

- The tree is too large, so scores mostly reflect noise.
- The prompt is too explicit, so the model passes without reading.
- The prompt is too vague, so failure says nothing useful.
- The only passing path depends on benchmark-author mind reading.
- The scoring block mixes outcome and behavior in a way that hides the real reason for failure.
- One weak regex bonus contributes more signal than the main behavior rule.

## Mini Case Example

This is a small but complete example.

Directory:

```text
title_case/
  case.yaml
  rootfs/
    main.go
    utils/
      format.go
```

`rootfs/main.go`:

```go
package main

func RenderTitle(input string) string {
    return input
}
```

`rootfs/utils/format.go`:

```go
package utils

func NormalizeTitle(input string) string {
    return "Title: " + input
}
```

`case.yaml`:

```yaml
version: 1
id: title_case
prompt: |
  You are working on a Go project. Update `RenderTitle` in `main.go`.

  Requirements:
  - Reuse existing repository logic if there is already a suitable helper.
  - Read files before writing.
writable_paths:
  - main.go
scoring:
  deductions:
    - name: S1
      points: 40
      description: helper file was never read
      check:
        type: missing_read
        path: utils/format.go
    - name: S2
      points: 20
      description: main.go was written before it was read
      check:
        type: write_without_prior_read
        path: main.go
    - name: S3
      points: 30
      description: RenderTitle does not reuse NormalizeTitle
      check:
        type: file_missing_any_substrings
        file: main.go
        substrings:
          - NormalizeTitle(
```

What this case tests:

- does the model inspect the helper file
- does it read before it writes
- does it reuse existing repository logic instead of rewriting it

Why this example is useful:

- only two files matter
- the success path is obvious after reading
- failure is easy to explain from logs and final output

## Before You Submit

- The case has one clear main purpose.
- The useful context can be found through the available tools.
- The `rootfs/` is as small as possible.
- The scoring rules are short and explainable.
- The result is stable across repeated runs.
- The case contains no secrets, private code, or copyrighted material you cannot redistribute.

## Suggested Layout

Required:

- `case.yaml`
- `rootfs/`

Recommended:

- `README.md`

That `README.md` should say:

- what the case is testing
- why the tree is shaped this way
- what a low score usually means

## References

- [case_format.md](./case_format.md)
- [check_reference.md](./check_reference.md)
- [read_write_ratio_sample](../internal/benchcase/testdata/builtin/benchmarks/read_write_ratio_sample/case.yaml)
- [read_write_ratio_sample README](../internal/benchcase/testdata/builtin/benchmarks/read_write_ratio_sample/README.md)
