# Changelog

本项目的版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增

- 配置模板更新检测与一键合并：`config_version`、`baseline.yaml`、三路合并、更新详情、稍后更新。
- 「关于DeployTool」设置面板与更新红点提醒。
- 设置页改为左侧标签 + 右侧面板布局。

## [1.1.x] - 2026-10

- 完整 Web 控制台：首次安装欢迎流程、服务表单化配置、已安装/未安装分区管理、一键安装/启动/停止/卸载、进度与日志查看。
- 「进入主页」与「一键启动后自动进入主页」。
- 前台控制台独立窗口（`start_mode: console`）。
- 安装目录运行时数据随 `install_root` 迁移；默认目录不产生 `.install_root` 侧车文件。
- 服务级环境变量 `env`、安装后不自动启动选项。

## [1.0.x] - 2026-09

- 首个可用版本：声明式配置引擎、六种服务类型适配器。
- 生命周期命令：`install` / `uninstall` / `update` / `start` / `stop` / `status` / `pack`。
- 离线打包与 SHA-256 校验、依赖拓扑排序、HTTP/TCP/进程/脚本健康检查。
- 浏览器控制台（`ui` / `desktop`）与 Windows 原生 WebView2 窗口（`window` / `deploytool-desktop.exe`）。