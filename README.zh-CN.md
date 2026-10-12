# deploytool · 部署工具

> 一个离线优先、声明式的服务部署与生命周期管理工具。每个产物都是单一可执行文件，无需运行时依赖，也不发起任何外网请求。支持 Windows 与 Linux（amd64 / arm64 / loong64）。

[English](README.md) · [示例与验收手册](examples/README.md) · [配置参考](docs/config-schema.md) · [更新日志](CHANGELOG.md)

deploytool 用一份 `deploytool.yaml` 描述整个项目：包含哪些服务、每个服务如何安装、如何启动、健康检查怎么做、依赖顺序是什么。然后通过命令行或内置 Web 控制台完成「安装 / 启动 / 停止 / 更新 / 卸载 / 打包」。

## 特性

- **声明式配置**：用 YAML 描述服务与安装步骤，支持 `mkdir` / `extract` / `copy_file` / `render_template` / `write_env` / `set_permission` / `run_script`。
- **依赖排序**：按 `depends_on` 拓扑排序，安装按依赖正序、停止与卸载按倒序。
- **健康检查**：HTTP / TCP / 进程 / 脚本四类探测，启动失败自动标记为异常。
- **离线运行**：前端资源内嵌进二进制，不依赖 CDN；Go 依赖已 vendoring，离线可复现构建。
- **多形态交付**：`exe` / `python` / `java` / `nodejs` / `go` / `nginx` 六种类型标签；Java 同时支持 class 与 jar。
- **Web 控制台**：首次安装欢迎流程、服务表单化配置、已安装/未安装分区、一键安装/启动/停止/卸载、安装进度与日志、进入主页。
- **原生桌面窗口**：Windows 下通过 WebView2 提供原生窗口（`deploytool-desktop.exe`）。
- **配置模板更新**：通过 `config_version` 检测模板升级，自动生成差异并按三路合并应用到本地配置，冲突以官方模板为准。
- **增量更新**：`pack` 生成带 `manifest.json` 与 SHA-256 校验的 tar.gz，`update` 校验后替换并重启，失败可回滚。

## 快速开始

构建需要 Go 1.22+（依赖已 vendoring，无需联网）。

```powershell
# Windows：产出全部平台产物到 dist/
.\scripts\build.ps1

# Linux / macOS
sh scripts/build.sh
```

主要产物：

| 文件 | 说明 |
| --- | --- |
| `deploytool-windows-amd64.exe` | Windows 命令行版；双击可直接打开浏览器控制台 |
| `deploytool-desktop.exe` | Windows 原生窗口版（双击即用） |
| `deploytool-linux-amd64` | Linux amd64 |
| `deploytool-linux-arm64` | Linux arm64 |
| `deploytool-linux-loong64` | Linux loong64 |

跑通轻量冒烟示例（只安装文件、不启动进程）：

```powershell
go build -o deploytool.exe ./cmd/deploytool
.\deploytool.exe install   --config examples/example-project/deploytool.yaml
.\deploytool.exe status    --config examples/example-project/deploytool.yaml
.\deploytool.exe uninstall --config examples/example-project/deploytool.yaml
```

完整的可运行演示（七个服务、健康检查、增量更新、卸载）见 [examples/README.md](examples/README.md)。

## 命令

| 命令 | 说明 |
| --- | --- |
| `install` | 按依赖顺序安装并启动服务；`--no-start` 仅安装不启动 |
| `uninstall` | 按依赖倒序卸载；`--purge-state` 同时删除状态文件 |
| `start` | 启动指定服务或 `all` |
| `stop` | 停止指定服务或 `all` |
| `status` | 查看状态；`--json` 输出机器可读结果 |
| `update` | 用 `--package` 指定的 tar.gz 增量更新 |
| `pack` | 打包更新包（`--src` + `--out`） |
| `ui` | 启动 Web 控制台（默认 `127.0.0.1:8090`，不自动开浏览器） |
| `desktop` | 启动控制台并打开系统默认浏览器 |
| `window` | 启动控制台并打开原生 WebView2 窗口（仅 Windows） |

通用参数：`--config <path>`（默认 `deploytool.yaml`）、`--only svc1,svc2`、`--host`、`--port`。运行 `deploytool --help` 查看完整用法。

## 配置

配置文件是 `deploytool.yaml`，结构如下（完整字段见 [docs/config-schema.md](docs/config-schema.md)）：

```yaml
apiVersion: v1
config_version: "1.0.0"   # 配置模板版本，用于检测升级
project:
  name: demo
  version: "1.0.0"
  portal: "http://127.0.0.1:8080/"   # 可选：控制台“进入主页”的跳转地址
vars:
  install_root: ./runtime            # 安装根目录
services:
  - name: web
    type: nodejs
    workdir: web
    install:
      steps:
        - { action: mkdir, path: "{{ .workdir }}" }
        - { action: copy_file, src: build/app.js, dest: "{{ .workdir }}/app.js" }
    start: "node app.js"
    start_mode: background            # background | console
    healthcheck: { type: http, url: "http://127.0.0.1:8080/healthz" }
    env:
      NODE_ENV: production
```

要点：

- 基础 `deploytool.yaml` 是只读模板；命令行与界面修改写入 `<install_root>/.deploytool/override.yaml`，可随时“重置为初始配置”。
- 服务字段与 `env` 可在控制台表单化编辑；安装步骤这类复杂结构保留 YAML 编辑。
- 升级 `config_version` 后，已部署实例打开会提示更新，点击更新会三路合并到 override（冲突以官方模板为准），并保留你的安装目录。

## 目录结构

```
.
├── cmd/                    # 命令行入口（CLI + Windows 桌面）
├── internal/               # 核心实现（config / engine / process / webui / ...）
├── examples/               # 示例项目与中文验收手册
├── docs/                   # 配置说明
├── scripts/                # 构建脚本
├── vendor/                 # Go 依赖（离线构建）
├── .github/workflows/      # CI：测试 + 打 tag 自动构建发布
└── README.md
```

## 平台支持

| 平台 | 形态 | 说明 |
| --- | --- | --- |
| Windows 10/11 amd64 | CLI + 原生窗口 | 原生窗口依赖 WebView2 运行时（系统通常已内置） |
| Linux amd64 | CLI | — |
| Linux arm64 | CLI | — |
| Linux loong64 | CLI | 适配国产化平台 |

## 发布

推送到 `main` 会运行测试；推送 `v*` 标签会自动构建全部平台产物并创建 GitHub Release。

## 协议

[MIT](LICENSE) © 2026 MelonoleM