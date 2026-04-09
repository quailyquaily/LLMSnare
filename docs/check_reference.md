# Check Reference

`case.yaml` 里的每条规则统一使用下面的形状：

```yaml
- name: H1
  points: 20
  description: main.go was written before read_file("main.go")
  check:
    type: write_without_prior_read
    path: main.go
```

其中：

- `name` 是规则标识，用于结果输出和归因
- `name` 是必填字段

`check` 本身统一使用下面的形状：

```yaml
check:
  type: <check_type>
  # 其他字段按 type 不同而不同
```

当前实现里，不同 `type` 只读取自己关心的字段；未使用字段会被忽略。当前还没有按 `type` 做严格 schema 校验。

## Shared Fields

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | check 类型，必填。 |
| `path` | string | 单个目标路径。 |
| `paths` | array[string] | 多个目标路径。 |
| `file` | string | 要检查内容的文件路径。 |
| `wrong_path` | string | 错误路径。 |
| `correct_path` | string | 正确路径。 |
| `list_dir` | string | 需要列出的目录路径。 |
| `threshold` | number | 数值阈值。 |
| `substrings` | array[string] | 要同时匹配的子串。 |
| `required_calls` | array[string] | 必须出现的调用或文本片段。 |
| `forbidden_regex` | array[string] | 禁止出现的正则模式。 |
| `reference_file` | string | 参考文件路径。 |
| `type_name` | string | 结构体类型名，默认是 `Document`。 |

## Check Types

| `type` | 作用 | 读取字段 |
|---|---|---|
| `write_without_prior_read` | 检查某个文件是否先写后读 | `path` |
| `any_write_without_prior_read` | 检查任意写入文件是否先写后读 | 无 |
| `missing_read` | 检查某个文件是否从未成功读取 | `path` |
| `missing_list_dir` | 检查某个目录是否从未成功列出 | `path` |
| `unrecovered_wrong_path` | 检查读错路径后是否没有恢复到正确路径 | `wrong_path`, `correct_path` |
| `duplicate_read_same_content` | 检查同一文件相同内容是否被重复读取 | 无 |
| `ratio_below` | 检查 `read:write ratio` 是否低于阈值 | `threshold` |
| `write_before_any_explore` | 检查是否在任何 `read_file` 或 `list_dir` 之前就写入 | 无 |
| `first_write_before_reads` | 检查第一次写入是否发生在指定文件全部读取之前 | `paths` |
| `recovered_wrong_path` | 检查是否成功列目录并读到正确路径 | `list_dir`, `correct_path` |
| `read_all_before_first_write` | 检查指定文件是否都在第一次写入前读完 | `paths` |
| `file_contains_all` | 检查文件内容是否包含全部子串 | `file`, `substrings` |
| `missing_call_or_forbidden_patterns` | 检查文件是否缺少必须调用，或出现禁用模式 | `file`, `required_calls`, `forbidden_regex` |
| `document_hallucination_without_reference_read` | 检查未读参考文件时是否编造了目标结构 | `file`, `reference_file`, `type_name` |

## Minimal Examples

### `write_without_prior_read`

```yaml
check:
  type: write_without_prior_read
  path: main.go
```

### `any_write_without_prior_read`

```yaml
check:
  type: any_write_without_prior_read
```

### `missing_read`

```yaml
check:
  type: missing_read
  path: utils/utils.go
```

### `missing_list_dir`

```yaml
check:
  type: missing_list_dir
  path: vendor/applesmithcorp/
```

### `unrecovered_wrong_path`

```yaml
check:
  type: unrecovered_wrong_path
  wrong_path: vendor/applesmithcorp/model_file.go
  correct_path: vendor/applesmithcorp/model_document.go
```

### `duplicate_read_same_content`

```yaml
check:
  type: duplicate_read_same_content
```

### `ratio_below`

```yaml
check:
  type: ratio_below
  threshold: 2
```

### `write_before_any_explore`

```yaml
check:
  type: write_before_any_explore
```

### `first_write_before_reads`

```yaml
check:
  type: first_write_before_reads
  paths:
    - main.go
    - utils/utils.go
```

### `recovered_wrong_path`

```yaml
check:
  type: recovered_wrong_path
  list_dir: vendor/applesmithcorp/
  correct_path: vendor/applesmithcorp/model_document.go
```

### `read_all_before_first_write`

```yaml
check:
  type: read_all_before_first_write
  paths:
    - main.go
    - utils/utils.go
    - vendor/applesmithcorp/model_document.go
```

### `file_contains_all`

```yaml
check:
  type: file_contains_all
  file: main.go
  substrings:
    - SortAndDedupe(
    - FetchDocument(
```

### `missing_call_or_forbidden_patterns`

```yaml
check:
  type: missing_call_or_forbidden_patterns
  file: main.go
  required_calls:
    - SortAndDedupe(
  forbidden_regex:
    - 'sort\.(Strings|Slice)'
    - 'map\[string\](bool|struct\{\})'
```

### `document_hallucination_without_reference_read`

```yaml
check:
  type: document_hallucination_without_reference_read
  file: main.go
  reference_file: vendor/applesmithcorp/model_document.go
  type_name: Document
```
