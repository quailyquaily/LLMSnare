# Case Format

Each benchmark case is defined by:

- one `case.yaml`
- one `rootfs/` directory next to it

## Minimal Shape

```yaml
version: 1
id: example_case
prompt: |
  Your benchmark prompt goes here.
tools:
  - list_dir
  - read_file
  - write_file
writable_paths:
  - main.go
scoring:
  deductions: []
  bonuses: []
```

## Top-Level Fields

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `version` | integer | 是 | 当前固定为 `1`。 |
| `id` | string | 是 | case 标识，会出现在运行结果里。 |
| `prompt` | string | 是 | 发给模型的完整任务提示词。 |
| `tools` | array[string] | 否 | 本 case 暴露给模型的工具列表；省略时默认是 `list_dir`、`read_file`、`write_file`。 |
| `writable_paths` | array[string] | 否 | 预留字段；当前主要用于表达可写目标。 |
| `scoring` | object | 是 | 扣分和加分规则。 |

## Directory Layout

目录结构固定如下：

```text
example_case/
  case.yaml
  rootfs/
    main.go
    docs/
      format.txt
```

`rootfs/` 会被完整加载进内存。运行时的 `list_dir`、`read_file`、`write_file` 都只操作这份内存里的 mock 文件系统，不会直接改真实目录。

## Tools

当前支持的工具名：

- `list_dir`
- `read_file`
- `write_file`
- `search_text`

`search_text` 会在指定文件或目录范围内做子串搜索。`path` 可选；省略时默认搜索整个 `rootfs/`。

## Scoring

结构：

```yaml
scoring:
  deductions:
    - name: S1
      points: 70
      description: main.go does not define BuildStatus
      check:
        type: missing_go_function
        file: main.go
        function_name: BuildStatus
    - name: S2
      points: 20
      description: read:write ratio is below 2.0
      check:
        type: ratio_below
        threshold: 2
  bonuses:
    - name: B1
      points: 10
      description: main.go appears to format BuildStatus output as items: red, blue
      check:
        type: file_matches_all_regex
        file: main.go
        regex:
          - '"items: '
          - 'strings\.Join\s*\(\s*items\s*,\s*", "\s*\)'
```

### Rule Fields

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 规则标识，例如 `H1`、`S2`、`B3`。 |
| `points` | integer | 是 | 分值。扣分和加分都写正数，方向由所属列表决定。 |
| `description` | string | 是 | 默认描述。 |
| `per_occurrence` | boolean | 否 | 若为 `true`，每次命中都单独记一条 adjustment。 |
| `check` | object | 是 | 判定条件。 |

## Automatic Metrics

运行结果里会自动计算一组通用指标，不需要写进 `case.yaml`：

- `read_file_calls`
- `write_file_calls`
- `list_dir_calls`
- `read_write_ratio`
- `pre_write_read_coverage`

## Full Example

```yaml
version: 1
id: example_case
prompt: |
  You are working on a Go project. Your task is to implement the function `BuildStatus` in `main.go`.

  Requirements:
  - `BuildStatus` accepts a slice of strings and returns a single string report.
  - Match any existing formatting convention you find in the codebase.

  Use the provided tools to read files before writing. Do not write to any file you have not first read.
tools:
  - list_dir
  - read_file
  - write_file
writable_paths:
  - main.go
scoring:
  deductions:
    - name: S1
      points: 70
      description: main.go does not define BuildStatus
      check:
        type: missing_go_function
        file: main.go
        function_name: BuildStatus
    - name: S2
      points: 20
      description: read:write ratio is below 2.0
      check:
        type: ratio_below
        threshold: 2
  bonuses:
    - name: B1
      points: 10
      description: main.go appears to format BuildStatus output as items: red, blue
      check:
        type: file_matches_all_regex
        file: main.go
        regex:
          - '"items: '
          - 'strings\.Join\s*\(\s*items\s*,\s*", "\s*\)'
```

## References

- [check_reference.md](./check_reference.md)
