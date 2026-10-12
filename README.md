# deploytool

> An offline-first, declarative service deployment and lifecycle manager. Each artifact is a single self-contained executable with no runtime dependencies, and the tool itself makes no outbound network requests. It supports Windows and Linux (amd64 / arm64 / loong64).

[中文说明](README.zh-CN.md) · [Examples & walkthrough](examples/README.md) · [Configuration reference](docs/config-schema.md) · [Changelog](CHANGELOG.md)

deploytool describes an entire project in a single `deploytool.yaml`: which services exist, how each one is installed, how it starts, how its health is verified, and in what order services depend on each other. You then install, start, stop, update, uninstall, and package services through the command line or the built-in web console.

## Features

- **Declarative configuration**: services and install steps are described in YAML, with `mkdir`, `extract`, `copy_file`, `render_template`, `write_env`, `set_permission`, and `run_script` actions.
- **Dependency ordering**: services are topologically ordered by `depends_on`; install runs in dependency order, and stop/uninstall run in reverse order.
- **Health checks**: HTTP, TCP, process, and script probes; a failed startup is reported as unhealthy.
- **Offline operation**: web assets are embedded in the binary (no CDN), and Go dependencies are vendored for reproducible offline builds.
- **Flexible delivery shapes**: `exe`, `python`, `java`, `nodejs`, `go`, and `nginx` service types; Java supports both loose class files and packaged jars.
- **Web console**: first-run welcome flow, form-based service configuration, installed/uninstalled sections, one-click install/start/stop/uninstall, install progress and logs, and an "open portal" action.
- **Native desktop window**: a WebView2-based native window on Windows (`deploytool-desktop.exe`).
- **Template updates**: `config_version` detects template upgrades, shows a diff, and merges changes into the local override with a three-way merge (the official template wins on conflict).
- **Incremental updates**: `pack` produces a `tar.gz` with a `manifest.json` and SHA-256 digests; `update` verifies, replaces, and restarts services, with rollback on failure.

## Quick start

Building requires Go 1.22+ (dependencies are vendored, so no network access is needed).

```powershell
# Windows: produces all platform artifacts under dist/
.\scripts\build.ps1

# Linux / macOS
sh scripts/build.sh
```

Primary artifacts:

| File | Description |
| --- | --- |
| `deploytool-windows-amd64.exe` | Windows CLI; double-clicking opens the browser console |
| `deploytool-desktop.exe` | Windows native-window build (double-click to run) |
| `deploytool-linux-amd64` | Linux amd64 |
| `deploytool-linux-arm64` | Linux arm64 |
| `deploytool-linux-loong64` | Linux loong64 |

Run the lightweight smoke example (installs files only, starts no processes):

```powershell
go build -o deploytool.exe ./cmd/deploytool
.\deploytool.exe install   --config examples/example-project/deploytool.yaml
.\deploytool.exe status    --config examples/example-project/deploytool.yaml
.\deploytool.exe uninstall --config examples/example-project/deploytool.yaml
```

A full runnable demo (seven services, health checks, incremental update, and uninstall) is documented in [examples/README.md](examples/README.md), currently in Chinese.

## Commands

| Command | Description |
| --- | --- |
| `install` | Install and start services in dependency order; `--no-start` installs without starting |
| `uninstall` | Uninstall in reverse dependency order; `--purge-state` also removes the state file |
| `start` | Start a named service or `all` |
| `stop` | Stop a named service or `all` |
| `status` | Show status; `--json` emits machine-readable output |
| `update` | Apply an incremental update from a `--package` tar.gz |
| `pack` | Build an update package (`--src` + `--out`) |
| `ui` | Start the web console (default `127.0.0.1:8090`; does not open a browser automatically) |
| `desktop` | Start the console and open the system default browser |
| `window` | Start the console and open a native WebView2 window (Windows only) |

Common flags: `--config <path>` (default `deploytool.yaml`), `--only svc1,svc2`, `--host`, and `--port`. Run `deploytool --help` for the full usage.

## Configuration

Configuration lives in `deploytool.yaml`; the complete field reference is in [docs/config-schema.md](docs/config-schema.md):

```yaml
apiVersion: v1
config_version: "1.0.0"   # template version, used to detect upgrades
project:
  name: demo
  version: "1.0.0"
  portal: "http://127.0.0.1:8080/"   # optional: where "open portal" navigates
vars:
  install_root: ./runtime            # install root directory
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

Notes:

- The base `deploytool.yaml` is a read-only template; CLI and console edits are written to `<install_root>/.deploytool/override.yaml` and can be reset to the initial configuration.
- Service fields and `env` are edited through form controls in the console; complex structures such as install steps remain YAML-edited.
- After a `config_version` bump, a deployed instance prompts for an update on open; applying it three-way merges the template changes into the override (the official template wins on conflict) while preserving your install directory.

## Directory layout

```
.
├── cmd/                    # command-line entry points (CLI + Windows desktop)
├── internal/               # core implementation (config / engine / process / webui / ...)
├── examples/               # example projects and walkthrough
├── docs/                   # configuration notes
├── scripts/                # build scripts
├── vendor/                 # vendored Go dependencies (offline builds)
├── .github/workflows/      # CI: tests + tag-triggered builds and releases
└── README.md
```

## Platform support

| Platform | Form | Notes |
| --- | --- | --- |
| Windows 10/11 amd64 | CLI + native window | The native window requires the WebView2 runtime (usually preinstalled) |
| Linux amd64 | CLI | — |
| Linux arm64 | CLI | — |
| Linux loong64 | CLI | Adapted for domestic platforms |

## Releases

Pushes to `main` run the test suite; pushing a `v*` tag builds all platform artifacts and creates a GitHub Release automatically.

## License

[MIT](LICENSE) © 2026 MelonoleM