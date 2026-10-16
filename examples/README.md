# deploytool 全类型示例：中文手工验收指南

`example-project` 是可以实际运行的离线演示项目，覆盖 `exe`、`python`、`java`、`nodejs`、`go`、`nginx` 六种服务类型，共七个服务。所有服务只监听 `127.0.0.1` 的 18100–18106 端口，全程不需要联网，也不需要下载依赖。

其中 Java 给了两种交付形态：`java-api` 直接跑编译出的 `.class`，`java-jar-api` 跑打好的 jar 包，见第 7 节。

> `nginx-proxy` 是自包含的 Go 反向代理演示程序，用来展示 `nginx` 类型的生命周期、依赖顺序和代理效果；生产环境请替换为已审批的离线 Nginx 发行包及真实 `nginx.conf`。

同目录的 `deploytool.yaml` 是更轻量的冒烟配置：它同样覆盖六种服务类型，但只安装文件、不启动任何进程，也不需要先跑 `prepare.ps1`，适合先验证安装与卸载这一步。

手册按「安装 → 运行 → 查看状态 → 停止 → 更新 → 卸载」的顺序编排，每一步都给出命令和预期输出，可以逐条照着勾选。下文命令均在仓库根目录的 PowerShell 中执行。

## 命令速查

| 目的 | 命令 |
| --- | --- |
| 查看帮助 | `dist\deploytool-windows-amd64.exe --help` |
| 安装并启动 | `install --config <cfg>` |
| 查看状态 | `status --config <cfg> [--json] [--only svc1,svc2]` |
| 停止 | `stop --config <cfg> <服务名\|all>` |
| 启动 | `start --config <cfg> <服务名\|all>` |
| 打包更新包 | `pack --config <cfg> --src <目录> --out <包.tar.gz>` |
| 增量更新 | `update --config <cfg> --package <包.tar.gz> [--only svc]` |
| 卸载 | `uninstall --config <cfg> [--purge-state]` |
| Web 界面 | `ui --config <cfg> [--port 8090]` |

除命令行外，项目还带一个浏览器控制台：运行 `& $exe ui --config $cfg` 后打开 http://127.0.0.1:8090。左侧列出所有服务及其状态，右侧查看单个服务的详情、日志和健康信息，页面上可直接安装 / 卸载 / 启停服务，也支持打包与增量更新。

## 1. 前置条件

本手册面向 Windows PowerShell。请带上整个项目目录；`dist` 需要先通过 `.\scripts\build.ps1` 构建（或从 GitHub Releases 下载预编译二进制）。首次准备示例要求本机有 Go、Python 3、Node.js 与 JDK：

```powershell
go version
python --version
node --version
java -version
javac -version
```

进入仓库根目录，并约定两个变量供后续命令复用：

```powershell
Set-Location D:\path\to\ProjectHelper
$exe = '.\dist\deploytool-windows-amd64.exe'
$cfg = '.\examples\example-project\deploytool.windows.yaml'
```

可选：把运行状态放到仓库外的专用目录。不设置时使用仓库根目录的 `runtime-live`；环境变量 `INSTALL_ROOT` 会覆盖配置里的 `install_root`，正斜杠和反斜杠路径都支持：

```powershell
$env:INSTALL_ROOT = 'D:\deploytool-demo-runtime'
```

确认演示端口空闲（没有任何输出即为空闲）：

```powershell
Get-NetTCPConnection -LocalPort (18100..18106) -ErrorAction SilentlyContinue
```

## 2. 准备离线示例制品

脚本会编译 Go 演示程序、编译 Java 类文件、打出 jar 包，并把 Python/Node.js 源码复制到 `build`，全部使用本仓库内容：

```powershell
.\examples\example-project\prepare.ps1
```

预期输出：

```
Prepared offline demo artifacts in ...\examples\example-project\build
```

产物包括 `bootstrap-exe.exe`、`nginx-proxy.exe`、`ExampleServer.class`、`JarServer.class`、`java-jar-api.jar`、`python-app.py`、`node-app.js`。

配置里的 `src` 源路径是相对**配置文件所在目录**解析的，所以把 `example-project`（含 `build`）整个拷到任意位置都能用。要做一个便携包，带上 `deploytool` 可执行文件、这份配置和 `build` 目录即可：双击 exe（或运行 `deploytool desktop`）会自动在浏览器打开控制台。

打包 jar 需要 `jar.exe`，它经常不在 PATH 上：JDK 的 `java`/`javac` 通常由 `C:\Program Files\Common Files\Oracle\Java\javapath` 的转发程序提供，而真正的 JDK 目录里才有 `jar.exe`。脚本会依次从 `JAVA_HOME`、PATH、`javac` 所在目录、常见 JDK 安装目录里查找，找不到会明确报错并提示安装 JDK 或设置 `JAVA_HOME`。

## 3. 安装并启动全部服务

```powershell
& $exe install --config $cfg
```

安装会逐项落地文件、启动服务，并等待 HTTP 健康检查通过。预期七个服务全部 `installed=true running=true healthy=true healthy`，命令以退出码 `0` 结束：

```
bootstrap-exe        installed=true  running=true  healthy=true  skipped=false healthy
java-api             installed=true  running=true  healthy=true  skipped=false healthy
java-jar-api         installed=true  running=true  healthy=true  skipped=false healthy
python-worker        installed=true  running=true  healthy=true  skipped=false healthy
go-gateway           installed=true  running=true  healthy=true  skipped=false healthy
nodejs-web           installed=true  running=true  healthy=true  skipped=false healthy
nginx-proxy          installed=true  running=true  healthy=true  skipped=false healthy
```

`install` 是幂等的：已安装的服务不会重复落地文件，但已停止的服务会被重新拉起。后台模式的服务日志写在 `<install_root>\.deploytool\logs\<服务名>.log`。

## 4. 访问服务与查看状态

依次请求七个健康接口：

```powershell
18100..18106 | ForEach-Object {
  "Port $_"
  Invoke-RestMethod "http://127.0.0.1:$_/healthz"
}
```

预期输出：

```
Port 18100 -> {"service":"bootstrap-exe","status":"ok"}
Port 18101 -> {"service":"python-worker","status":"ok"}
Port 18102 -> {"service":"java-api","status":"ok"}
Port 18103 -> {"service":"nodejs-web","status":"ok"}
Port 18104 -> {"service":"go-gateway","status":"ok"}
Port 18105 -> {"service":"nginx-proxy","status":"ok"}
Port 18106 -> {"service":"java-jar-api","status":"ok","packaging":"jar"}
```

18105 是 `nginx-proxy`，会反向代理到 Go 网关 18104；18106 是 jar 形态的服务，它比别的服务多返回一个 `"packaging":"jar"`，用来区分交付形态。

查看状态，`--json` 输出机器可读结果，`--only` 只看指定服务：

```powershell
& $exe status --config $cfg
& $exe status --config $cfg --json
& $exe status --config $cfg --only nodejs-web
```

查看日志（未设置 `INSTALL_ROOT` 时用 `.\runtime-live`）：

```powershell
Get-ChildItem .\runtime-live\.deploytool\logs
Get-ChildItem "$env:INSTALL_ROOT\.deploytool\logs"   # 设置过 INSTALL_ROOT 时
```

## 5. 单服务停止与重新启动

```powershell
& $exe stop --config $cfg nodejs-web
& $exe status --config $cfg --only nodejs-web
& $exe start --config $cfg nodejs-web
Invoke-RestMethod http://127.0.0.1:18103/healthz
```

预期：停止后状态为 `running=false ... stopped`；重新启动后恢复 `running=true healthy=true`，健康接口返回 `{"service":"nodejs-web","status":"ok"}`。

jar 服务同样支持单服务操作，把服务名换成 `java-jar-api` 即可：

```powershell
& $exe stop --config $cfg java-jar-api
& $exe start --config $cfg java-jar-api
Invoke-RestMethod http://127.0.0.1:18106/healthz
```

注意：`stop <服务名>` 只停止这一个服务，不会级联停止依赖它的服务，也不会带上它依赖的服务。需要整体停机时用 `all`：

```powershell
& $exe stop --config $cfg all
& $exe status --config $cfg
& $exe start --config $cfg all
```

`stop all` 按依赖倒序停止，`start all` 按依赖正序启动。全部停止后 `status` 仍以退出码 `0` 结束，每个服务显示 `running=false ... stopped`。

## 6. 前台控制台模式（可选排障）

默认 `start_mode: background`：命令在健康检查通过后返回，输出写入日志文件。需要盯着某个服务的实时输出时，把**单个**服务的配置改成：

```yaml
start_mode: console
```

然后在一个独立的 PowerShell 窗口里停止并重新启动该服务：

```powershell
& $exe stop --config $cfg python-worker
& $exe start --config $cfg python-worker
```

命令会占用当前终端并转发服务的标准输出与标准输入，按 Ctrl+C 结束（退出后状态记为已停止或失败）。控制台模式是交互式的，不要用于全量安装、多服务批量启动或 Web 界面。

## 7. Java 服务的两种交付形态

两个 Java 服务用同一个 JDK 运行时，区别只在交付物和启动命令：

| | `java-api`（class 形态） | `java-jar-api`（jar 形态） |
| --- | --- | --- |
| 安装产物 | `ExampleServer.class` | `java-jar-api.jar` |
| 打包方式 | `javac` 直接产出 | `javac` 编译后由 `jar` 打成带 `Main-Class` 的 jar |
| 落地方式 | `copy_file` 复制 class 文件 | `copy_file` 复制单个 jar |
| 启动命令 | `java -cp . ExampleServer 18102 java-api` | `java -jar java-jar-api.jar 18106 java-jar-api` |
| 健康接口 | `{"service":"java-api","status":"ok"}` | `{"service":"java-jar-api","status":"ok","packaging":"jar"}` |

对应的配置片段：

```yaml
  - name: java-jar-api
    type: java
    workdir: "{{ .Vars.install_root }}/java-jar-api"
    depends_on: [bootstrap-exe]
    install:
      steps:
        - { action: mkdir, path: "{{ .workdir }}" }
        - { action: copy_file, src: examples/example-project/build/java-jar-api.jar, dest: "{{ .workdir }}/java-jar-api.jar" }
    start: "java -jar java-jar-api.jar 18106 java-jar-api"
    healthcheck: { type: http, url: http://127.0.0.1:18106/healthz, retries: 15 }
```

`type: java` 只是标签，工具不关心包里是 class 还是 jar：它只负责把文件放到 `workdir`，然后执行你写的 `start`。这也是为什么换一种交付形态不需要改工具。

真实项目的 jar 发布通常不止一个文件，而是 jar 加依赖库和配置目录。这种情况把整个发布目录打成 `.tar.gz` 一次解包更省事：

```yaml
    install:
      steps:
        - { action: mkdir, path: "{{ .workdir }}" }
        - { action: extract, src: artifacts/app-dist.tar.gz, dest: "{{ .workdir }}" }
    start: "java -jar lib/app.jar --spring.config.location=conf/"
```

依赖不在 jar 内部时，可以用 `java -cp "lib/*;conf" com.example.Main` 显式指定 classpath——Windows 的分隔符是 `;`，Linux 是 `:`；用 `java -jar` 并依赖 manifest 里的 `Class-Path` 则不需要关心分隔符。更新 jar 包和更新其他服务一样：`pack` 打出新包，`update --only java-jar-api` 覆盖并重启即可。

## 8. 增量更新：将 Node.js 服务升级为 V2

`updates/nodejs-web-v2/app.js` 会让健康接口额外返回 `"version":"v2"`。先确认服务在运行，再打包并只更新 `nodejs-web`：

```powershell
New-Item -ItemType Directory -Force .\artifacts | Out-Null
& $exe pack --config $cfg --src .\examples\example-project\updates\nodejs-web-v2 --out .\artifacts\nodejs-web-v2.tar.gz
& $exe update --config $cfg --package .\artifacts\nodejs-web-v2.tar.gz --only nodejs-web
Invoke-RestMethod http://127.0.0.1:18103/healthz
```

升级前健康接口返回 `{"service":"nodejs-web","status":"ok"}`，升级后返回 `{"service":"nodejs-web","status":"ok","version":"v2"}`。

`pack` 会自动生成 `manifest.json`；`update` 会校验 SHA-256、停止服务、把被替换的文件备份到 `<install_root>\.deploytool\backups\<服务名>\<时间戳>\`、替换文件、重新启动并做健康检查。失败且 `rollback_on_failure: true` 时会用备份回滚。备份目录在卸载时会被一并清理。

## 9. 卸载与现场清理

```powershell
& $exe uninstall --config $cfg --purge-state
```

预期七个服务都显示 `not installed`，退出码 `0`；服务进程全部退出，安装时落地的文件、pid 文件、更新备份目录和已清空的服务目录都会被删除，`--purge-state` 还会删除状态文件 `<install_root>\.deploytool\state.json`。

验证端口已释放（无输出即为干净；`TimeWait` 是已关闭连接的正常残留，不是监听）：

```powershell
Get-NetTCPConnection -LocalPort (18100..18106) -ErrorAction SilentlyContinue
```

运行日志不会自动删除，方便事后排查；确认不再需要时可手工清理：

```powershell
Remove-Item -Recurse -Force .\runtime-live, .\artifacts
```

`prepare.ps1` 生成的 `examples/example-project/build` 与 `artifacts` 里的更新包都是可复用的测试制品，删除后重新执行第 2 步即可重建。

## 10. 退出码与常见问题

| 退出码 | 含义 |
| --- | --- |
| `0` | 本次操作成功；对 `status` 而言表示所有选中服务都已安装，并且要么已停止、要么健康 |
| `2` | 用法错误、配置无法读取或校验失败、缺少必需参数 |
| `4` | 操作未全部成功，例如服务未安装、启动后健康检查失败、指定了不存在的服务 |

常见情况：

- 报 `unknown command "..."`：第一个参数必须是子命令，配置文件要用 `--config` 传入，例如 `& $exe install --config $cfg`。
- `prepare.ps1` 报找不到 `jar.exe`：安装 JDK 或设置 `JAVA_HOME` 指向 JDK 根目录（不是 JRE）。
- 端口被占用：用 `Get-NetTCPConnection -LocalPort 18100 -ErrorAction SilentlyContinue` 找出占用进程，或改配置里的端口。
- 只想操作部分服务：生命周期命令都支持 `--only svc1,svc2`，用法如 `& $exe stop --config $cfg --only nodejs-web`；`--only` 只处理指定服务，不会自动带上它的依赖。
- 状态显示 `process is not running`：服务进程意外退出，查看 `<install_root>\.deploytool\logs\<服务名>.log`。
- 忘记配置里有哪些服务：`& $exe status --config $cfg` 会列出全部服务及其状态。
