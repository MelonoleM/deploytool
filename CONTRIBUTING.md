# Contributing

感谢你考虑为 deploytool 贡献代码。本项目的依赖已 vendoring，构建和测试全程无需联网。

## 开发环境

- Go 1.22+（`go.mod` 中声明的版本）。
- Windows 下可安装 WebView2 运行时（Win10/11 通常已内置）用于测试原生窗口。

## 构建与测试

```powershell
go test ./...
go build ./cmd/deploytool
```

发布构建：

```powershell
# Windows
.\scripts\build.ps1

# Linux / macOS
sh scripts/build.sh
```

## 代码约定

- 使用 `gofmt` 格式化 Go 代码。
- 逻辑改动请补充或更新对应测试（`internal/**/*_test.go`）。
- 前端资源位于 `internal/webui/static/`，随二进制 `//go:embed` 内嵌。

## 提交

- 提交信息使用清晰的中文或英文描述改动内容。
- 功能与修复分提交，避免大而全的一次性提交。

## 协议

提交即表示你同意在项目协议（MIT）下贡献你的代码。