# Repository Guidelines

## Project Structure & Module Organization
`main.go` 只负责进程入口和信号处理，CLI 装配放在 `cmd/`。`internal/benchmark` 负责执行基准与计分，`internal/benchcase` 负责 case 加载与内置样例，`internal/config`、`internal/storage`、`internal/api` 分别处理配置、时间线存储和 HTTP 接口。说明文档放在 `docs/`。内置 benchmark 样例和测试数据放在 `internal/benchcase/testdata/builtin/`。

## Build, Test, and Development Commands
`go run . --help`：查看全部命令和参数。  
`go run . init --config ./config.yaml`：生成本地配置和示例 case，适合开发时快速起步。  
`go build ./...`：检查所有包是否能编译。  
`go test ./...`：运行完整测试集。  
`go test ./cmd -run TestRender`：只跑 CLI 输出相关测试，适合改命令行渲染时快速回归。

## Coding Style & Naming Conventions
遵循标准 Go 风格，提交前运行 `gofmt -w` 或 `go fmt ./...`。缩进和导入顺序交给 Go 工具，不手调。命令解析、参数校验、输出格式化留在 `cmd/`；可复用逻辑放进 `internal/`。包名保持小写，文件名按职责命名，例如 `run.go`、`server.go`、`timeline.go`。错误信息用明确上下文，避免模糊包装。

## Testing Guidelines
测试文件与代码同目录，命名为 `*_test.go`。测试函数用 `TestXxx`，复杂场景优先用 `t.Run(...)` 拆子用例。改动计分、case 加载、CLI 输出或 HTTP JSON 结构时，必须补回归测试。字符串输出测试要校验关键字段，不要把断言放宽到只测“非空”。

## Commit & Pull Request Guidelines
现有提交标题使用简短祈使句，例如 `Implement configurable benchmark runner`、`Update README.md`。保持单一主题，不把无关改动混进同一个提交。PR 说明至少写清三件事：改了什么、为什么改、怎么验证。涉及 CLI 输出、OpenAPI 或配置字段时，附命令示例或响应片段。

## Security & Configuration Tips
不要提交真实 API key、token 或时间线数据。配置示例统一使用 `${ENV_NAME}` 形式的环境变量占位符。新增 benchmark case 时，只给最小必要的 `writable_paths`，避免把无关文件暴露给被测模型。
