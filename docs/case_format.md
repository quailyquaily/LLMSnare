# Case Format

Each benchmark case is defined by:

- one `case.yaml`
- one fixture directory referenced by `fixture_dir`

## Minimal Shape

```yaml
version: 1
id: example_case
prompt: |
  Your benchmark prompt goes here.
fixture_dir: fixture
writable_paths:
  - main.go
scoring:
  deductions: []
  bonuses: []
metrics: {}
```

## Top-Level Fields

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `version` | integer | 是 | 当前固定为 `1`。 |
| `id` | string | 是 | case 标识，会出现在运行结果里。 |
| `prompt` | string | 是 | 发给模型的完整任务提示词。 |
| `fixture_dir` | string | 是 | fixture 根目录。 |
| `writable_paths` | array[string] | 否 | 预留字段；当前主要用于表达可写目标。 |
| `scoring` | object | 是 | 扣分和加分规则。 |
| `metrics` | object | 否 | 布尔类附加指标定义。 |

## Path Resolution

`fixture_dir` 支持三种写法：

- 相对路径：相对 `case.yaml` 所在目录解析
- 绝对路径：如 `/home/user/my-fixture`
- `~` 开头：如 `~/fixtures/my-case`

fixture 文件会被加载进内存，运行时使用 mock 工具访问，不会直接把这些文件当成真实仓库来改。

## Scoring

结构：

```yaml
scoring:
  deductions:
    - name: H1
      points: 20
      description: main.go was written before read_file("main.go")
      check:
        type: write_without_prior_read
        path: main.go
  bonuses:
    - name: B1
      points: 10
      description: recovered vendor trap
      check:
        type: recovered_wrong_path
        list_dir: vendor/applesmithcorp/
        correct_path: vendor/applesmithcorp/model_document.go
```

### Rule Fields

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 规则标识，例如 `H1`、`S2`、`B3`。 |
| `points` | integer | 是 | 分值。扣分和加分都写正数，方向由所属列表决定。 |
| `description` | string | 是 | 默认描述。 |
| `per_occurrence` | boolean | 否 | 若为 `true`，每次命中都单独记一条 adjustment。 |
| `check` | object | 是 | 判定条件。 |

## Metrics

`metrics` 用来定义额外布尔指标。

当前支持的字段：

```yaml
metrics:
  vendor_trap_recovered:
    type: recovered_wrong_path
    list_dir: vendor/applesmithcorp/
    correct_path: vendor/applesmithcorp/model_document.go
  util_trap_triggered:
    type: missing_call_or_forbidden_patterns
    file: main.go
    required_calls:
      - SortAndDedupe(
    forbidden_regex:
      - 'sort\.(Strings|Slice)'
```

运行结果里还会自动计算：

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
fixture_dir: fixture
writable_paths:
  - main.go
scoring:
  deductions:
    - name: S2
      points: 20
      description: read:write ratio is below 2.0
      check:
        type: ratio_below
        threshold: 2
  bonuses: []
metrics: {}
```

## References

- [check_reference.md](./check_reference.md)
- [init_requirement.md](./init_requirement.md)
