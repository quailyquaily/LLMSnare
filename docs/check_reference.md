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
| `function_name` | string | 要检查的函数名。 |
| `regex` | array[string] | 正则模式列表，具体语义由 `type` 决定。 |
| `threshold` | number | 数值阈值。 |
| `substrings` | array[string] | 要同时匹配的子串。 |

## Check Types

| `type` | 作用 | 读取字段 |
|---|---|---|
| `write_without_prior_read` | 检查某个文件是否先写后读 | `path` |
| `any_write_without_prior_read` | 检查任意写入文件是否先写后读 | 无 |
| `missing_read` | 检查某个文件是否从未成功读取 | `path` |
| `missing_list_dir` | 检查某个目录是否从未成功列出 | `path` |
| `duplicate_read_same_content` | 检查同一文件相同内容是否被重复读取 | 无 |
| `ratio_below` | 检查 `read:write ratio` 是否低于阈值 | `threshold` |
| `write_before_any_explore` | 检查是否在任何 `read_file` 或 `list_dir` 之前就写入 | 无 |
| `first_write_before_reads` | 检查第一次写入是否发生在指定文件全部读取之前 | `paths` |
| `read_all_before_first_write` | 检查指定文件是否都在第一次写入前读完 | `paths` |
| `file_contains_all` | 检查文件内容是否包含全部子串 | `file`, `substrings` |
| `file_missing_any_substrings` | 检查文件是否缺少任意一个必需子串 | `file`, `substrings` |
| `file_matches_all_regex` | 检查文件内容是否匹配全部正则模式 | `file`, `regex` |
| `file_matches_any_regex` | 检查文件内容是否匹配任意一个正则模式 | `file`, `regex` |
| `missing_go_function` | 检查 Go 文件里是否真的定义了指定顶层函数 | `file`, `function_name` |

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

### `file_missing_any_substrings`

```yaml
check:
  type: file_missing_any_substrings
  file: main.go
  substrings:
    - SortAndDedupe(
    - FetchDocument(
```

### `file_matches_all_regex`

```yaml
check:
  type: file_matches_all_regex
  file: main.go
  regex:
    - '"items: '
    - 'strings\.Join\s*\(\s*items\s*,\s*", "\s*\)'
```

### `file_matches_any_regex`

```yaml
check:
  type: file_matches_any_regex
  file: main.go
  regex:
    - 'sort\.(Strings|Slice)'
    - 'map\[string\](bool|struct\{\})'
```

### `missing_go_function`

```yaml
check:
  type: missing_go_function
  file: main.go
  function_name: BuildStatus
```
