# Timeline WAL 与 SQLite 投影视图

## 背景

当前 timeline 存储是按 profile 分文件的 JSONL 追加日志。

这个模型有两个优点：

- 写入简单，追加即可。
- 文件天然可读，排查问题直接。

它的问题也很明确：

- 按 `model`、`model_vendor`、`inference_provider`、`case_id` 这类字段筛选时，只能全量扫描。
- `limit` 目前也要先读完整个文件，再做截断。
- 数据量累计后，`serve` 的冷查询成本会线性增长。

引入 SQLite 的目的，不是替代这份追加日志，而是给 timeline API 提供可索引的读模型。

## 术语

本文中的 “WAL” 指 `llmsnare` 自己维护的 timeline JSONL 追加日志。

这里不指 SQLite 自带的 `journal_mode=WAL`。

## 核心决定

### 1. 保留 WAL

继续保留当前按 profile 分文件的 JSONL 追加日志。

WAL 是唯一 truth。

理由：

- 追加写最稳，失败边界清楚。
- 文件格式简单，便于人工检查。
- SQLite 损坏、丢失、schema 变化，都可以从 WAL 重建。

### 2. SQLite 只是投影视图

新增一个 SQLite 数据库，用来承接 timeline API 的读请求。

SQLite 的角色是：

- 提供按 `profile`、`model`、`model_vendor`、`inference_provider`、`case_id` 等字段的筛选能力
- 提供按时间倒序取最近 N 条的能力
- 为后续聚合、分页、排序留出空间

SQLite 不是 truth。

如果 SQLite 和 WAL 不一致，以 WAL 为准。

### 3. 双写顺序固定

`run --persist` 的写入顺序固定为：

1. 先写 WAL
2. 再写 SQLite

这样即使 SQLite 写失败，truth 仍然已经落盘。

SQLite 写失败后，系统状态可以描述为：

- benchmark 结果已经成功持久化到 WAL
- SQLite 投影视图落后，需要修复或重建

## 这个方案为什么合理

这个方案的重点不是“同时维护两套真相”，而是：

- 一套真相：WAL
- 一套可丢弃、可重建的索引化读模型：SQLite

这和很多事件日志系统的结构一致。写路径追求稳定，读路径追求查询效率，两者职责分开。

对 `llmsnare` 来说，这样做有四个直接好处：

- 不需要一次性把全部历史 JSONL 迁到 SQLite 才能上线
- SQLite schema 可以演进，旧数据仍然可从 WAL 回放
- 读性能问题和写可靠性问题被拆开处理
- 即使 SQLite 文件损坏，也不会丢 benchmark 历史

## 约束与前提

### 1. 必须有稳定的 timeline 记录 ID

如果要做双写和重建，单条 timeline 记录必须有稳定 ID。

这不是优化项，而是前置条件。

在 `run_id` 落地之前，不开始 SQLite 双写，也不开始重建命令。

推荐做法：

- 在写 WAL 前，为每条结果生成 `run_id`
- `run_id` 使用 UUID v7 字符串格式
- 这个 `run_id` 一起写进 WAL
- SQLite 用它做主键或唯一键

没有这个 ID，会出现两个问题：

- SQLite 重放时难以去重
- 双写失败重试时容易插入重复记录

不建议把 `timestamp + profile` 当唯一键。它不够稳。

### 1.1 UUID v7 在 SQLite 里的用法

SQLite 没有原生 UUID 类型，也不区分 UUID v4 和 UUID v7。

这不是问题。

第一版直接把 `run_id` 存成 `TEXT` 即可。

建议约束：

- 使用标准 UUID 字符串格式
- 统一使用小写
- 列定义为 `TEXT PRIMARY KEY` 或 `TEXT NOT NULL UNIQUE`

这样 SQLite 仍然可以很好地处理：

- 相等匹配
- 唯一性约束
- upsert

第一版不要依赖 `run_id` 的字典序来表达 timeline 顺序。

timeline 查询仍然统一按 `finished_at DESC` 排序。

这样做更稳，也更符合语义。`run_id` 负责身份，`finished_at` 负责时间顺序。

如果后面确认需要更紧凑的存储，再考虑把 UUID 编码成 16 字节 `BLOB`。第一版没有必要。

### 2. SQLite 必须被视为可重建缓存

只要接受 “SQLite 可以删掉再重建”，整个系统边界就会简单很多。

反过来，如果让 SQLite 承担 truth 角色，就会把迁移、备份、一致性、修复全都压到数据库上。这个项目没有必要走那条路。

### 3. 不在请求路径里做 WAL 全量回退

`serve` 不应该在正常请求路径里，一边查 SQLite，一边在失败时退回全量 JSONL 扫描。

原因：

- 请求延迟不可控
- 代码路径会分叉
- 缓存语义会变复杂

当前实现的做法是：

- SQLite 已完成重建且没有 dirty 标记时，API 读 SQLite
- SQLite 缺失、未完成首次重建或被标记为 dirty 时，API 回退到 WAL

这样做的原因是先保证结果正确，再逐步把默认读路径收敛到 SQLite。

## Go 侧 SQLite 库选择

第一版建议使用：

- `database/sql`
- `modernc.org/sqlite`

理由：

- 纯 Go，不依赖 CGO
- 保持现有 `go build`、`go test`、发布流程简单
- 对当前这种低写入、偏查询的场景已经够用

第一版不建议使用 `github.com/mattn/go-sqlite3`。

不是因为它不能用，而是因为它会把 CGO 和交叉编译负担一起带进来。对 `llmsnare` 现在这个体量，这个代价不值。

也不建议一开始就直接上更重的封装层。

第一版直接用 `database/sql`，把 schema、upsert、查询都写清楚，最容易控边界。

### 性能判断

按 `llmsnare` 当前的负载形态，这个选择没有明显性能问题。

原因：

- 写入频率很低，通常是每小时每个 profile 追加一条
- 查询模式简单，主要是按条件筛选、按时间倒序、取最近 N 条
- 这个项目的数据规模，远没有逼近 SQLite 的能力边界

需要明确的是：

- `modernc.org/sqlite` 通常不会比基于 CGO 的驱动更快
- 但在这个项目里，数据库驱动性能不是主矛盾
- 真正昂贵的部分仍然是模型调用，不是 timeline 索引查询

第一版里，真正会做全量线性扫描的是两个管理动作：

- `backfill-run-id`
- `rebuild-sqlite`

它们不在请求路径里，所以只要实现稳，线性扫描是可以接受的。

## SQLite 存什么

SQLite 只存 timeline API 需要的字段。

不存下面这些重字段：

- `tool_calls`
- `final_writes`
- `final_response`
- `error`
- `endpoint`

原因很简单：当前 timeline API 不返回它们，索引查询也不依赖它们。

建议把下面这些字段存成普通列：

- `run_id`
- `timestamp`
- `finished_at`
- `case_id`
- `profile`
- `provider`
- `model`
- `model_vendor`
- `inference_provider`
- `success`
- `total_score`
- `raw_score`
- `max_score`
- `normalized_score`

建议把下面这些字段存成 JSON 文本：

- `metrics_json`
- `deductions_json`
- `bonuses_json`

这样做的好处：

- schema 简单
- `metrics` 将来加字段时，不需要改表结构
- 列索引只落在真正用于筛选和排序的字段上

## SQLite 文件位置

第一版建议把 SQLite 文件放在现有 timeline 目录下面。

默认路径：

```text
<timeline_dir>/timeline.sqlite3
```

这样做的理由：

- 与 WAL 共用同一个存储根目录
- 权限、备份和迁移边界更清楚
- 不需要额外新增必填配置项
- 现有代码只扫描 `.jsonl` 文件，不会把 `.sqlite3` 误识别成 timeline WAL

第一版不急着新增单独的 SQLite 配置项。

如果后面确认有把 SQLite 放到单独磁盘或单独目录的需求，再新增可选配置，例如：

- `storage.sqlite_path`

## 索引建议

第一版先建最小索引集：

- `(profile, finished_at DESC)`
- `(model_vendor, finished_at DESC)`
- `(inference_provider, finished_at DESC)`
- `(model_vendor, inference_provider, profile, finished_at DESC)`
- `(model, profile, finished_at DESC)`
- `(case_id, profile, finished_at DESC)`

## 双写流程

建议流程如下：

1. 组装本次 benchmark 的 `Result`
2. 生成稳定的 `run_id`
3. 把完整结果追加写入 WAL
4. 把 timeline API 需要的字段写入 SQLite
5. 如果 SQLite 写失败，返回明确错误，并把投影视图标记为需要修复

这里的关键点是第 3 步先于第 4 步。

因为 WAL 是 truth，所以宁可出现 “WAL 已写入，SQLite 未跟上”，也不要出现反过来的情况。

## 重建方案

### 目标

提供一个显式方案，把当前 WAL 全量重建写入 SQLite。

这个能力至少用于三类场景：

- 第一次引入 SQLite，需要把历史数据导入
- SQLite 文件损坏或丢失
- SQLite schema 升级后，需要重建

### 重建原则

重建过程只读 WAL，不读旧 SQLite。

原因：

- 旧 SQLite 可能已经脏了
- truth 在 WAL，不在 SQLite

### 旧 WAL 没有 `run_id` 时怎么办

这是历史数据投影时必须先处理的问题。

因为早期 JSONL 记录里可能没有 `run_id`，所以第一版重建不能直接跳到 “扫 WAL 写 SQLite”。

应先做一个显式的 WAL 回填步骤：

1. 扫描所有 timeline JSONL 文件
2. 找出缺少 `run_id` 的记录
3. 为这些记录生成新的 UUID v7
4. 用临时文件重写原 JSONL
5. 校验通过后原子替换旧文件

这个回填步骤改的是 truth，所以必须谨慎：

- 逐文件处理
- 每个文件都走临时文件 + 原子替换
- 不允许直接原地修改

回填完成后，WAL 才具备作为 SQLite 重建输入的前提。

### 重建步骤

建议流程如下：

1. 先执行一次 WAL `run_id` 回填，确保所有记录都有 `run_id`
2. 创建一个新的临时 SQLite 文件
3. 在临时文件里创建最新 schema 和索引
4. 按 profile 文件名稳定排序，逐个扫描 WAL 文件
5. 按文件中的行顺序逐条解析 JSON
6. 从每条记录里取出 `run_id`
7. 将记录转换成 SQLite 行并写入
8. 全量写完后，校验记录数
9. 用原子替换方式，把临时文件切换成正式数据库文件

这个流程有两个重点：

- 重建期间不修改原数据库文件
- 只有校验通过后才替换

这样即使重建中途中断，也不会把现有 SQLite 破坏掉。

### 运维操作建议

建议把历史数据投影拆成两个显式命令：

1. `llmsnare timeline backfill-run-id --config ./config.yaml`
2. `llmsnare timeline rebuild-sqlite --config ./config.yaml`

第一个命令只做一件事：

- 把旧 WAL 里缺失的 `run_id` 补齐

第二个命令也只做一件事：

- 从已经带 `run_id` 的 WAL 全量重建 SQLite

这样拆开的好处：

- 出错边界清楚
- 日志容易理解
- 出问题时容易重试

不建议把 “补 `run_id`” 和 “重建 SQLite” 混成一个黑箱动作。

当前实现就是这两个显式命令：

1. `llmsnare timeline backfill-run-id --config ./config.yaml`
2. `llmsnare timeline rebuild-sqlite --config ./config.yaml`

另外补一个只读检查命令：

1. `llmsnare timeline status --config ./config.yaml`

它只回答当前状态，不改任何数据。至少会显示：

- 当前读路径是 `wal` 还是 `sqlite`
- WAL profile 数和总行数
- SQLite 文件是否存在
- SQLite 是否 ready
- SQLite 是否 dirty

### 去重策略

SQLite 写入要基于 `run_id` 做幂等 upsert。

这样有两个好处：

- 重建命令可以重复执行
- 双写失败后的补写不会制造重复行

### 校验建议

第一版至少做下面两项校验：

- WAL 解析出的总记录数
- SQLite 写入后的总记录数

如果两者不一致，重建失败，不切换正式文件。

后续如果需要更强校验，可以再加：

- 按 profile 统计条数对比
- 最新一条 `finished_at` 对比

## API 读路径

切到 SQLite 后，timeline API 的读路径建议这样收敛：

- `/v1/timelines/{profile}`：直接查 SQLite
- `/v1/timelines`：直接查 SQLite
- 过滤条件：直接映射为 SQL `WHERE`
- `limit`：直接映射为 SQL `LIMIT`

这样 `limit` 才是真正的限制结果数，而不是先全量读完再截断。

## 失败场景

### WAL 写失败

这属于真正的持久化失败。

结果不应视为已保存。

### WAL 写成功，SQLite 写失败

这不属于数据丢失。

它属于投影视图落后。

处理原则：

- truth 仍然存在
- API 结果可能不完整
- 需要执行修复或重建

### SQLite 重建失败

保留旧 SQLite，不切换正式文件。

这样系统仍然可以继续使用旧投影视图，只是缺少最新修复结果。

## 分阶段落地建议

建议按下面顺序推进：

### 第一阶段

- 保留现有 WAL
- 新增 `run_id`
- 新增 SQLite schema
- `run --persist` 改为先写 WAL，再写 SQLite

这里的重点是先把写路径和主键打稳。

如果这一阶段没有完成，不进入重建命令开发。

### 第二阶段

- 新增显式重建命令
- 支持从历史 WAL 全量导入 SQLite

也就是说，实施顺序固定为：

1. 先做 `run_id`
2. 再做 SQLite schema 和双写
3. 再做重建命令
4. 最后再把 timeline API 切到 SQLite

### 第三阶段

- timeline API 改为从 SQLite 读取
- 增加按 `model`、`model_vendor`、`inference_provider`、`case_id` 的筛选参数

## 结论

这个方案是合理的。

前提只有三个：

- WAL 是唯一 truth
- 每条 timeline 记录有稳定 `run_id`
- SQLite 被当成可重建的投影视图，而不是第二份 truth

只要守住这三个前提，保留 WAL 并同时写入 SQLite，不会把系统变复杂到失控。相反，它能把“可靠写入”和“高效查询”拆开处理。
